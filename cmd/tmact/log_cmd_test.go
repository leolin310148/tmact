package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/leolin310148/tmact/internal/logsearch"
	"github.com/leolin310148/tmact/internal/logstats"
)

func TestLogSearchJSONUsesPrivateDefaultsAndAcceptsFlagsAfterQuery(t *testing.T) {
	setupCLILogFixtures(t)
	out, err := captureRun(t, "log", "search", "topsecret", "--provider", "claude", "--since", "48h", "--kind", "tool_call", "--limit", "5", "--json")
	if err != nil {
		t.Fatal(err)
	}
	var report logsearch.Report
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatal(err)
	}
	if len(report.Matches) != 1 || report.Matches[0].CommandVerb != "git" || report.Matches[0].CommandSubcommand != "status" {
		t.Fatalf("matches = %#v", report.Matches)
	}
	if len(report.Coverage) != 1 || report.Coverage[0].Provider != "claude" {
		t.Fatalf("coverage = %#v", report.Coverage)
	}
	if strings.Contains(out, "topsecret") || strings.Contains(out, "private argument") {
		t.Fatalf("default output leaked private command content: %s", out)
	}
}

func TestLogSearchShowContentRequiresOptIn(t *testing.T) {
	setupCLILogFixtures(t)
	defaultOut, err := captureRun(t, "log", "search", "needle")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(defaultOut, "private prompt") || !strings.Contains(defaultOut, "Coverage:") {
		t.Fatalf("unexpected default output: %s", defaultOut)
	}
	privateOut, err := captureRun(t, "log", "search", "needle", "--show-content")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(privateOut, "private prompt") {
		t.Fatalf("show-content output = %s", privateOut)
	}
}

func TestLogSearchValidationAndSinceParsing(t *testing.T) {
	now := time.Date(2026, 7, 22, 12, 0, 0, 0, time.UTC)
	if got, err := parseLogSince("24h", now); err != nil || !got.Equal(now.Add(-24*time.Hour)) {
		t.Fatalf("duration since = %v, %v", got, err)
	}
	if got, err := parseLogSince("2026-07-20T10:00:00Z", now); err != nil || got.Format(time.RFC3339) != "2026-07-20T10:00:00Z" {
		t.Fatalf("timestamp since = %v, %v", got, err)
	}
	for _, args := range [][]string{
		{"log", "search"},
		{"log", "search", "query", "extra"},
		{"log", "search", "query", "--limit", "0"},
		{"log", "search", "query", "--provider", "other"},
		{"log", "search", "query", "--since", "later"},
		{"log", "search", "query", "--kind", "other"},
	} {
		if _, err := captureRun(t, args...); err == nil {
			t.Fatalf("expected error for %v", args)
		}
	}
}

func TestLogSearchCanSearchFlagLikeQueryAfterDelimiter(t *testing.T) {
	setupCLILogFixtures(t)
	out, err := captureRun(t, "log", "search", "--json", "--", "--short")
	if err != nil {
		t.Fatal(err)
	}
	var report logsearch.Report
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatal(err)
	}
	if len(report.Matches) != 1 || report.Matches[0].CommandVerb != "git" {
		t.Fatalf("matches = %#v", report.Matches)
	}
}

func TestLogStatsJSONAggregatesSafeFieldsAndUsesIndex(t *testing.T) {
	setupCLILogFixtures(t)
	out, err := captureRun(t, "log", "stats", "--since", "48h", "--json")
	if err != nil {
		t.Fatal(err)
	}
	var report logstats.Report
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatal(err)
	}
	if report.Records != 2 || report.Index.Status != "rebuilt_missing" {
		t.Fatalf("report = %#v", report)
	}
	if len(report.Providers) != 2 || report.Commands[0].Name != "git" || report.Commands[0].Count != 1 {
		t.Fatalf("aggregates = %#v", report)
	}
	for _, private := range []string{"topsecret", "private argument", "needle private prompt"} {
		if strings.Contains(out, private) {
			t.Fatalf("stats leaked %q: %s", private, out)
		}
	}

	second, err := captureRun(t, "log", "stats", "--json")
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(second), &report); err != nil {
		t.Fatal(err)
	}
	if report.Index.Status != "healthy" || report.Index.Hits != 2 {
		t.Fatalf("cached report = %#v", report.Index)
	}
}

func TestLogDoctorRebuildsCorruptCacheAndReportsHealth(t *testing.T) {
	setupCLILogFixtures(t)
	cachePath := filepath.Join(os.Getenv("HOME"), ".tmact", logstats.DefaultCacheName)
	if err := os.MkdirAll(filepath.Dir(cachePath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cachePath, []byte("not-json"), 0o600); err != nil {
		t.Fatal(err)
	}
	out, err := captureRun(t, "log", "doctor", "--json")
	if err != nil {
		t.Fatal(err)
	}
	var report logstats.DoctorReport
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatal(err)
	}
	if !report.Cache.Healthy || report.Cache.Status != "rebuilt_corrupt" || report.Files.Discovered != 2 || report.Records.Known != 2 {
		t.Fatalf("doctor = %#v", report)
	}
}

func TestLogHelpDocumentsPrivacyCoverageAndIndex(t *testing.T) {
	for _, args := range [][]string{{"log", "--help"}, {"log", "search", "--help"}, {"log", "stats", "--help"}, {"log", "doctor", "--help"}} {
		out, err := captureRun(t, args...)
		if err != nil {
			t.Fatal(err)
		}
		for _, want := range []string{"privacy", "coverage"} {
			if !strings.Contains(strings.ToLower(out), want) {
				t.Fatalf("help %v missing %q: %s", args, want, out)
			}
		}
	}
	for _, topic := range []string{"log search", "log stats", "log doctor"} {
		if _, ok := commandHelpFor(topic); !ok {
			t.Fatalf("command catalog missing %s", topic)
		}
	}
}

func setupCLILogFixtures(t *testing.T) {
	t.Helper()
	reset := stubCLIHooks(t)
	t.Cleanup(reset)
	tmactNow = func() time.Time { return time.Date(2026, 7, 22, 12, 0, 0, 0, time.UTC) }
	root := t.TempDir()
	t.Setenv("HOME", root)
	claudeRoot := filepath.Join(root, "claude")
	codexRoot := filepath.Join(root, "codex")
	writeCLILog(t, filepath.Join(claudeRoot, "projects", "fixture", "session.jsonl"), strings.Join([]string{
		`{"type":"assistant","timestamp":"2026-07-21T09:00:00Z","sessionId":"claude-cli","cwd":"/workspace/cli","message":{"role":"assistant","content":[{"type":"text","text":"needle private prompt"},{"type":"tool_use","name":"Bash","input":{"command":"TOKEN=topsecret git status --short private argument"}}]}}`,
	}, "\n")+"\n")
	writeCLILog(t, filepath.Join(codexRoot, "sessions", "2026", "07", "session.jsonl"),
		`{"type":"session_meta","timestamp":"2026-07-21T10:00:00Z","payload":{"id":"codex-cli","cwd":"/workspace/cli"}}`+"\n")
	t.Setenv("CLAUDE_CONFIG_DIRS", claudeRoot)
	t.Setenv("CLAUDE_CONFIG_DIR", "")
	t.Setenv("CODEX_HOME", codexRoot)
}

func writeCLILog(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}
