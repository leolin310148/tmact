package agentspend

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// claudeScanner reads Claude Code session logs from the configured config
// dir(s). Layout: <configDir>/projects/<slug>/<sessionId>.jsonl, one JSON
// object per line. Assistant turns carry message.usage and message.model.
//
// Resolution mirrors codeburn's getClaudeConfigDirs: CLAUDE_CONFIG_DIRS
// (os.PathListSeparator list) → CLAUDE_CONFIG_DIR → ~/.claude. Claude Desktop
// "cowork" sessions are intentionally out of scope for this first cut (noted as
// a deviation in the package docs); the CLI config dirs cover normal usage.
type claudeScanner struct{}

func (claudeScanner) provider() string { return "claude" }

func claudeConfigDirs() []string {
	expand := func(p string) string {
		p = strings.TrimSpace(p)
		if p == "~" {
			if h, err := os.UserHomeDir(); err == nil {
				return h
			}
		}
		if strings.HasPrefix(p, "~/") {
			if h, err := os.UserHomeDir(); err == nil {
				return filepath.Join(h, p[2:])
			}
		}
		return p
	}

	if multi := os.Getenv("CLAUDE_CONFIG_DIRS"); multi != "" {
		var dirs []string
		seen := map[string]bool{}
		for _, d := range strings.Split(multi, string(os.PathListSeparator)) {
			d = expand(d)
			if d == "" {
				continue
			}
			abs, err := filepath.Abs(d)
			if err != nil {
				abs = d
			}
			if !seen[abs] {
				seen[abs] = true
				dirs = append(dirs, abs)
			}
		}
		if len(dirs) > 0 {
			return dirs
		}
	}
	if single := os.Getenv("CLAUDE_CONFIG_DIR"); single != "" {
		return []string{expand(single)}
	}
	if h, err := os.UserHomeDir(); err == nil {
		return []string{filepath.Join(h, ".claude")}
	}
	return nil
}

func (claudeScanner) discover() []string {
	var files []string
	seen := map[string]bool{}
	for _, dir := range claudeConfigDirs() {
		projects := filepath.Join(dir, "projects")
		entries, err := os.ReadDir(projects)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			matches, _ := filepath.Glob(filepath.Join(projects, e.Name(), "*.jsonl"))
			for _, m := range matches {
				if abs, err := filepath.Abs(m); err == nil {
					m = abs
				}
				if !seen[m] {
					seen[m] = true
					files = append(files, m)
				}
			}
		}
	}
	return files
}

// claudeLine is the subset of a Claude JSONL record we price.
type claudeLine struct {
	Type      string `json:"type"`
	Timestamp string `json:"timestamp"`
	Message   struct {
		ID    string `json:"id"`
		Model string `json:"model"`
		Usage *struct {
			InputTokens         int `json:"input_tokens"`
			OutputTokens        int `json:"output_tokens"`
			CacheCreationTokens int `json:"cache_creation_input_tokens"`
			CacheReadTokens     int `json:"cache_read_input_tokens"`
			ServerToolUse       struct {
				WebSearchRequests int `json:"web_search_requests"`
			} `json:"server_tool_use"`
			CacheCreation struct {
				Ephemeral1hInputTokens int `json:"ephemeral_1h_input_tokens"`
			} `json:"cache_creation"`
			Speed string `json:"speed"`
		} `json:"usage"`
	} `json:"message"`
}

func (claudeScanner) parseFile(path string) []pricedRow {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()

	var rows []pricedRow
	sc := bufio.NewScanner(f)
	// Assistant turns with large tool payloads can exceed bufio's default 64K
	// line cap; raise it generously.
	sc.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
	for sc.Scan() {
		b := sc.Bytes()
		if len(b) == 0 || !strings.Contains(string(b), `"usage"`) {
			continue
		}
		var ln claudeLine
		if err := json.Unmarshal(b, &ln); err != nil {
			continue
		}
		if ln.Type != "assistant" || ln.Message.Usage == nil {
			continue
		}
		model := ln.Message.Model
		if model == "" || model == "<synthetic>" {
			continue
		}
		ts, err := time.Parse(time.RFC3339, ln.Timestamp)
		if err != nil {
			continue
		}
		u := ln.Message.Usage
		cost, ok := calculateCost(
			model,
			u.InputTokens,
			u.OutputTokens,
			u.CacheCreationTokens,
			u.CacheReadTokens,
			u.ServerToolUse.WebSearchRequests,
			u.Speed,
			u.CacheCreation.Ephemeral1hInputTokens,
		)
		if !ok {
			continue
		}
		dedup := ln.Message.ID
		if dedup == "" {
			dedup = "claude:" + ln.Timestamp
		}
		rows = append(rows, pricedRow{ts: ts, cost: cost, dedup: dedup})
	}
	return rows
}
