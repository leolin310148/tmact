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
	var prevInput, prevCached, prevOutput, prevReasoning int

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

		var input, cached, output, reasoning int
		if last := info.LastTokenUsage; last != nil {
			input, cached, output, reasoning = last.InputTokens, last.CachedInputTokens, last.OutputTokens, last.ReasoningOutputTokens
		} else if cumulative > 0 {
			t := info.TotalTokenUsage
			input = t.InputTokens - prevInput
			cached = t.CachedInputTokens - prevCached
			output = t.OutputTokens - prevOutput
			reasoning = t.ReasoningOutputTokens - prevReasoning
		}

		// Advance cumulative baseline regardless of which branch produced the
		// delta (codeburn note: otherwise mixed last/no-last sessions
		// double-count).
		t := info.TotalTokenUsage
		prevInput, prevCached, prevOutput, prevReasoning = t.InputTokens, t.CachedInputTokens, t.OutputTokens, t.ReasoningOutputTokens
		havePrevCumulative = true
		prevCumulative = cumulative

		if input+cached+output+reasoning == 0 {
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

		// OpenAI folds cached tokens into input_tokens; normalize to
		// Anthropic semantics (input = non-cached) before pricing.
		uncachedInput := input - cached
		if uncachedInput < 0 {
			uncachedInput = 0
		}
		cost, ok := calculateCost(model, uncachedInput, output+reasoning, 0, cached, 0, "standard", 0)
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
