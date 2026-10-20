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

func TestDailyCheckedReportsDiscoveryFailure(t *testing.T) {
	config := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(config, []byte("file"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CLAUDE_CONFIG_DIRS", "")
	t.Setenv("CLAUDE_CONFIG_DIR", config)
	if _, err := DailyChecked(time.Now(), 30); err == nil {
		t.Fatal("failed discovery must not look like zero cost")
	}
}

func TestDailyTaiwanMidnightBoundaries(t *testing.T) {
	root := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIRS", "")
	t.Setenv("CLAUDE_CONFIG_DIR", filepath.Join(root, "claude"))
	t.Setenv("CODEX_HOME", filepath.Join(root, "codex"))
	const row = `{"type":"assistant","timestamp":"%s","message":{"id":"%s","model":"claude-opus-4-8","usage":{"input_tokens":1000}}}`
	writeFixture(t, filepath.Join(root, "claude", "projects", "test", "s.jsonl"), []string{
		fmt.Sprintf(row, "2026-10-19T15:59:59Z", "before"),
		fmt.Sprintf(row, "2026-10-19T16:00:00Z", "midnight"),
		fmt.Sprintf(row, "2026-10-20T00:30:00Z", "after-utc-midnight"),
	})
	now := time.Date(2026, 10, 20, 1, 0, 0, 0, time.UTC)
	rows, err := DailyChecked(now, 2)
	if err != nil {
		t.Fatal(err)
	}
	totals := map[string]float64{}
	for _, r := range rows {
		if r.Provider == "claude" {
			totals[r.Date] = r.USD
		}
	}
	approx(t, "before Taiwan midnight", totals["2026-10-19"], .005)
	approx(t, "after Taiwan midnight", totals["2026-10-20"], .010)
	if got := TrendDayStart(now).UTC().Format(time.RFC3339); got != "2026-10-19T16:00:00Z" {
		t.Fatalf("day start = %s", got)
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

	// Codex: one token_count, uncached input 800 + output 500 + cacheRead 200.
	// reasoning_output_tokens is a breakdown of output_tokens, not additional
	// usage. At gpt-5.3-codex rates this is $0.008435, dated in-week.
	codexRows := []string{
		`{"type":"session_meta","timestamp":"2026-05-29T09:00:00Z","payload":{"session_id":"sess1","model":"gpt-5.3-codex"}}`,
		`{"type":"event_msg","timestamp":"2026-05-29T09:01:00Z","payload":{"type":"token_count","info":{"last_token_usage":{"input_tokens":1000,"cached_input_tokens":200,"output_tokens":500,"reasoning_output_tokens":100,"total_tokens":1600},"total_token_usage":{"total_tokens":1600}}}}`,
	}
	writeFixture(t, filepath.Join(codexHome, "sessions", "2026", "05", "29", "rollout-sess1.jsonl"), codexRows)
	daily := Daily(time.Date(2026, 5, 30, 12, 0, 0, 0, time.UTC), 30)
	var claudeMay29, claudeMay10 float64
	for _, d := range daily {
		if d.Provider == "claude" && d.Date == "2026-05-29" {
			claudeMay29 = d.USD
		}
		if d.Provider == "claude" && d.Date == "2026-05-10" {
			claudeMay10 = d.USD
		}
	}
	approx(t, "daily dedup", claudeMay29, .005)
	approx(t, "daily historical", claudeMay10, .005)

	now := time.Date(2026, 5, 30, 12, 0, 0, 0, time.Local)
	res := compute(now, newFileCache())

	approx(t, "claude week", res["claude"].WeekUSD, 0.005)   // only msgA
	approx(t, "claude month", res["claude"].MonthUSD, 0.010) // msgA + msgB
	approx(t, "codex week", res["codex"].WeekUSD, 0.008435)
	approx(t, "codex month", res["codex"].MonthUSD, 0.008435)
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

func TestClaudeSpendStillRequiresModelOnUsageRecord(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session.jsonl")
	writeFixture(t, path, []string{
		`{"type":"assistant","timestamp":"2026-05-29T10:00:00Z","message":{"id":"m1","role":"assistant","model":"claude-opus-4-8","usage":{"input_tokens":1000}}}`,
		`{"type":"assistant","timestamp":"2026-05-29T10:00:01Z","message":{"id":"m2","role":"assistant","usage":{"input_tokens":1000}}}`,
	})

	rows := (claudeScanner{}).parseFile(path)
	if len(rows) != 1 || rows[0].dedup != "m1" {
		t.Fatalf("rows = %#v", rows)
	}
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
