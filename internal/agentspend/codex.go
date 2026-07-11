package agentspend

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// codexScanner reads Codex CLI rollout logs from ~/.codex/sessions/**.jsonl.
// Token usage arrives as event_msg/token_count records carrying a
// last_token_usage delta and a cumulative total_token_usage; the cumulative is
// used as the per-session dedup key (codeburn's codex.ts approach). The
// char-estimate fallback codeburn uses when a turn has no token info is
// intentionally omitted here — real sessions report info.
type codexScanner struct{}

func (codexScanner) provider() string { return "codex" }

func codexSessionsDir() string {
	if d := os.Getenv("CODEX_HOME"); d != "" {
		return filepath.Join(d, "sessions")
	}
	if h, err := os.UserHomeDir(); err == nil {
		return filepath.Join(h, ".codex", "sessions")
	}
	return ""
}

func (codexScanner) discover() []string {
	base := codexSessionsDir()
	if base == "" {
		return nil
	}
	var files []string
	_ = filepath.WalkDir(base, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if !d.IsDir() && strings.HasSuffix(path, ".jsonl") {
			if abs, e := filepath.Abs(path); e == nil {
				path = abs
			}
			files = append(files, path)
		}
		return nil
	})
	return files
}

type codexTokenUsage struct {
	InputTokens           int `json:"input_tokens"`
	CachedInputTokens     int `json:"cached_input_tokens"`
	OutputTokens          int `json:"output_tokens"`
	ReasoningOutputTokens int `json:"reasoning_output_tokens"`
	TotalTokens           int `json:"total_tokens"`
}

type codexLine struct {
	Type      string `json:"type"`
	Timestamp string `json:"timestamp"`
	Payload   struct {
		Type      string `json:"type"`
		Model     string `json:"model"`
		SessionID string `json:"session_id"`
		Info      *struct {
			Model           string           `json:"model"`
			ModelName       string           `json:"model_name"`
			LastTokenUsage  *codexTokenUsage `json:"last_token_usage"`
			TotalTokenUsage *codexTokenUsage `json:"total_token_usage"`
		} `json:"info"`
	} `json:"payload"`
}

func (codexScanner) parseFile(path string) []pricedRow {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()

	sessionID := strings.TrimSuffix(filepath.Base(path), ".jsonl")
	sessionModel := ""
	havePrevCumulative := false
	prevCumulative := 0
	var prevInput, prevCached, prevOutput int

	var rows []pricedRow
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
	for sc.Scan() {
		b := sc.Bytes()
		if len(b) == 0 {
			continue
		}
		var ln codexLine
		if err := json.Unmarshal(b, &ln); err != nil {
			continue
		}

		switch {
		case ln.Type == "session_meta":
			if ln.Payload.SessionID != "" {
				sessionID = ln.Payload.SessionID
			}
			if ln.Payload.Model != "" {
				sessionModel = ln.Payload.Model
			}
			continue
		case ln.Type == "turn_context" && ln.Payload.Model != "":
			sessionModel = ln.Payload.Model
			continue
		case ln.Type == "event_msg" && ln.Payload.Type == "token_count":
			// handled below
		default:
			continue
		}

		info := ln.Payload.Info
		if info == nil || info.TotalTokenUsage == nil {
			continue
		}
		cumulative := info.TotalTokenUsage.TotalTokens
		if havePrevCumulative && cumulative == prevCumulative {
			continue // duplicate / replayed event
		}

		var input, cached, output int
		if last := info.LastTokenUsage; last != nil {
			input, cached, output = last.InputTokens, last.CachedInputTokens, last.OutputTokens
		} else if cumulative > 0 {
			t := info.TotalTokenUsage
			input = t.InputTokens - prevInput
			cached = t.CachedInputTokens - prevCached
			output = t.OutputTokens - prevOutput
		}

		// Advance cumulative baseline regardless of which branch produced the
		// delta (codeburn note: otherwise mixed last/no-last sessions
		// double-count).
		t := info.TotalTokenUsage
		prevInput, prevCached, prevOutput = t.InputTokens, t.CachedInputTokens, t.OutputTokens
		havePrevCumulative = true
		prevCumulative = cumulative

		if input+cached+output == 0 {
			continue
		}

		model := sessionModel
		if info.Model != "" {
			model = info.Model
		} else if info.ModelName != "" {
			model = info.ModelName
		}
		if model == "" {
			model = "gpt-5"
		}

		ts, err := time.Parse(time.RFC3339, ln.Timestamp)
		if err != nil {
			continue
		}

		cost, ok := calculateCodexTokenCost(model, input, cached, output)
		if !ok {
			continue
		}
		rows = append(rows, pricedRow{
			ts:    ts,
			cost:  cost,
			dedup: "codex:" + sessionID + ":" + itoa(cumulative),
		})
	}
	return rows
}

const codexLongContextThreshold = 272_000

// calculateCodexTokenCost normalizes the Responses API usage shape before
// pricing. Cached tokens are included in input_tokens, while reasoning tokens
// are already included in output_tokens and must not be added a second time.
// Current 1.05M-context GPT families charge 2x input and 1.5x output when the
// request input exceeds 272K tokens.
func calculateCodexTokenCost(model string, input, cached, output int) (float64, bool) {
	uncachedInput := input - cached
	if uncachedInput < 0 {
		uncachedInput = 0
	}
	inputCost, ok := calculateCost(model, uncachedInput, 0, 0, cached, 0, "standard", 0)
	if !ok {
		return 0, false
	}
	outputCost, _ := calculateCost(model, 0, output, 0, 0, 0, "standard", 0)
	if input > codexLongContextThreshold && codexHasLongContextSurcharge(model) {
		return 2*inputCost + 1.5*outputCost, true
	}
	return inputCost + outputCost, true
}

func codexHasLongContextSurcharge(model string) bool {
	switch resolveAlias(canonicalName(model)) {
	case "gpt-5.4", "gpt-5.4-pro", "gpt-5.5", "gpt-5.5-pro",
		"gpt-5.6", "gpt-5.6-sol", "gpt-5.6-terra", "gpt-5.6-luna":
		return true
	default:
		return false
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
