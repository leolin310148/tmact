package agentspend

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestWindowBounds(t *testing.T) {
	// Saturday 2026-05-30 12:00 local. Week (Mon) → 2026-05-25; month → 2026-05-01.
	now := time.Date(2026, 5, 30, 12, 0, 0, 0, time.Local)
	week, month := windowBounds(now)
	if !week.Equal(time.Date(2026, 5, 25, 0, 0, 0, 0, time.Local)) {
		t.Errorf("week start = %v, want Mon 2026-05-25", week)
	}
	if !month.Equal(time.Date(2026, 5, 1, 0, 0, 0, 0, time.Local)) {
		t.Errorf("month start = %v, want 2026-05-01", month)
	}
}

// End-to-end over fixture files: one in-week row, one earlier-in-month row, one
// last-month row (excluded), and a duplicate message id (counted once).
func TestComputeWindowsAndDedup(t *testing.T) {
	home := t.TempDir()
	claudeDir := filepath.Join(home, ".claude")
	codexHome := filepath.Join(home, ".codex-home")
	t.Setenv("CLAUDE_CONFIG_DIR", claudeDir)
	t.Setenv("CLAUDE_CONFIG_DIRS", "")
	t.Setenv("CODEX_HOME", codexHome)

	// Claude: input 1000 @ opus-4-8 (=opus-4-6, 5e-6) → $0.005 per row.
	const c = `{"type":"assistant","timestamp":"%s","message":{"id":"%s","model":"claude-opus-4-8","usage":{"input_tokens":1000,"output_tokens":0,"cache_creation_input_tokens":0,"cache_read_input_tokens":0}}}`
	rows := []string{
		fmt.Sprintf(c, "2026-05-29T10:00:00Z", "msgA"), // in week
		fmt.Sprintf(c, "2026-05-10T10:00:00Z", "msgB"), // in month, not week
		fmt.Sprintf(c, "2026-04-20T10:00:00Z", "msgC"), // last month, excluded
		fmt.Sprintf(c, "2026-05-29T10:00:01Z", "msgA"), // dup of msgA → ignored
	}
	writeFixture(t, filepath.Join(claudeDir, "projects", "proj", "s.jsonl"), rows)

	// Codex: one token_count, uncached input 800 + output 600 + cacheRead 200
	// @ gpt-5.3-codex → $0.009835, dated in-week.
	codexRows := []string{
		`{"type":"session_meta","timestamp":"2026-05-29T09:00:00Z","payload":{"session_id":"sess1","model":"gpt-5.3-codex"}}`,
		`{"type":"event_msg","timestamp":"2026-05-29T09:01:00Z","payload":{"type":"token_count","info":{"last_token_usage":{"input_tokens":1000,"cached_input_tokens":200,"output_tokens":500,"reasoning_output_tokens":100,"total_tokens":1600},"total_token_usage":{"total_tokens":1600}}}}`,
	}
	writeFixture(t, filepath.Join(codexHome, "sessions", "2026", "05", "29", "rollout-sess1.jsonl"), codexRows)

	now := time.Date(2026, 5, 30, 12, 0, 0, 0, time.Local)
	res := compute(now, newFileCache())

	approx(t, "claude week", res["claude"].WeekUSD, 0.005)   // only msgA
	approx(t, "claude month", res["claude"].MonthUSD, 0.010) // msgA + msgB
	approx(t, "codex week", res["codex"].WeekUSD, 0.009835)
	approx(t, "codex month", res["codex"].MonthUSD, 0.009835)
}

func TestComputeCacheReusesParse(t *testing.T) {
	home := t.TempDir()
	claudeDir := filepath.Join(home, ".claude")
	t.Setenv("CLAUDE_CONFIG_DIR", claudeDir)
	t.Setenv("CLAUDE_CONFIG_DIRS", "")
	t.Setenv("CODEX_HOME", filepath.Join(home, "empty"))

	const c = `{"type":"assistant","timestamp":"2026-05-29T10:00:00Z","message":{"id":"m1","model":"claude-opus-4-8","usage":{"input_tokens":1000,"output_tokens":0,"cache_creation_input_tokens":0,"cache_read_input_tokens":0}}}`
	writeFixture(t, filepath.Join(claudeDir, "projects", "proj", "s.jsonl"), []string{c})

	fc := newFileCache()
	now := time.Date(2026, 5, 30, 12, 0, 0, 0, time.Local)
	a := compute(now, fc)
	b := compute(now, fc) // second pass hits cache; must match
	approx(t, "cache stable", a["claude"].WeekUSD, b["claude"].WeekUSD)
	approx(t, "cache value", b["claude"].WeekUSD, 0.005)
}

// helpers

func writeFixture(t *testing.T, path string, lines []string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	content := ""
	for _, l := range lines {
		content += l + "\n"
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
