package logsearch

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/leolin310148/tmact/internal/sessionlog"
)

func TestSearchBothProvidersIsPrivateByDefaultAndReportsCoverage(t *testing.T) {
	installFixtures(t)
	report, err := Search(Options{Query: "needle", Limit: DefaultLimit})
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Matches) != 2 || report.Matches[0].Provider != sessionlog.ProviderCodex || report.Matches[1].Provider != sessionlog.ProviderClaude {
		t.Fatalf("matches = %#v", report.Matches)
	}
	if len(report.Coverage) != 2 || report.Coverage[0].Provider != sessionlog.ProviderClaude || report.Coverage[1].Provider != sessionlog.ProviderCodex {
		t.Fatalf("coverage = %#v", report.Coverage)
	}
	if report.Coverage[0].Malformed != 1 || report.Coverage[0].Unknown != 1 || report.Coverage[0].Records != 3 {
		t.Fatalf("claude coverage = %#v", report.Coverage[0])
	}
	encoded, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	for _, private := range []string{"private Claude", "private Codex", "topsecret", "full-argument"} {
		if strings.Contains(string(encoded), private) {
			t.Fatalf("default JSON leaked %q: %s", private, encoded)
		}
	}
}

func TestSearchShowContentIsExplicitAndBounded(t *testing.T) {
	installFixtures(t)
	report, err := Search(Options{Query: "needle", Limit: DefaultLimit, ShowContent: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Matches) != 2 || !strings.Contains(report.Matches[0].Content, "private Codex prompt") || !strings.Contains(report.Matches[1].Content, "private Claude response") {
		t.Fatalf("matches = %#v", report.Matches)
	}
	content := strings.Repeat("界", MaxContentBytes)
	got, truncated := truncateUTF8(content, MaxContentBytes)
	if !truncated || len(got) > MaxContentBytes || !strings.HasSuffix(got, "界") {
		t.Fatalf("truncated=%t bytes=%d suffix-valid=%t", truncated, len(got), strings.HasSuffix(got, "界"))
	}
}

func TestSearchCommandOutputUsesOnlySafeVerbAndSubcommand(t *testing.T) {
	installFixtures(t)
	report, err := Search(Options{Query: "topsecret", Limit: DefaultLimit})
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Matches) != 1 {
		t.Fatalf("matches = %#v", report.Matches)
	}
	match := report.Matches[0]
	if match.CommandVerb != "git" || match.CommandSubcommand != "status" || match.Content != "" {
		t.Fatalf("match = %#v", match)
	}
	for _, test := range []struct {
		command string
		verb    string
		sub     string
	}{
		{"SECRET=value echo private", "echo", ""},
		{"rtk git status --short", "git", "status"},
		{"rtk proxy go test ./...", "go", "test"},
		{"echo private", "echo", ""},
		{"git -C /private status", "git", ""},
	} {
		verb, sub := commandSummary(test.command)
		if verb != test.verb || sub != test.sub {
			t.Fatalf("commandSummary(%q) = %q %q, want %q %q", test.command, verb, sub, test.verb, test.sub)
		}
	}
}

func TestSearchFiltersSortsAndLimits(t *testing.T) {
	installFixtures(t)
	since := time.Date(2026, 7, 21, 10, 0, 0, 0, time.UTC)
	report, err := Search(Options{
		Query:     "private",
		Providers: []sessionlog.Provider{sessionlog.ProviderCodex},
		Since:     since,
		CWD:       "/workspace/search-redacted",
		Kind:      sessionlog.KindToolResult,
		Limit:     1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Matches) != 1 || report.Matches[0].Timestamp != "2026-07-21T10:00:03Z" || report.Matches[0].Kind != sessionlog.KindToolResult {
		t.Fatalf("matches = %#v", report.Matches)
	}
	if len(report.Coverage) != 1 || report.Coverage[0].Provider != sessionlog.ProviderCodex {
		t.Fatalf("coverage = %#v", report.Coverage)
	}
}

func TestSearchReportsDiscoveryAndStreamErrors(t *testing.T) {
	discoveryErr := errors.New("cannot inspect")
	streamErr := errors.New("cannot read")
	report, err := search(Options{Query: "anything", Providers: []sessionlog.Provider{sessionlog.ProviderClaude}, Limit: 10},
		func(provider sessionlog.Provider) (sessionlog.Discovery, error) {
			return sessionlog.Discovery{
				Sources: []sessionlog.Source{{Provider: provider, Path: "/redacted/source.jsonl"}},
				Errors:  []sessionlog.DiscoveryError{{Path: "/redacted/root", Err: discoveryErr}},
			}, nil
		},
		func(sessionlog.Source, func(sessionlog.Record) error) (sessionlog.Stats, error) {
			return sessionlog.Stats{Lines: 2, Malformed: 1}, streamErr
		})
	if err != nil {
		t.Fatal(err)
	}
	coverage := report.Coverage[0]
	if coverage.Sources != 1 || coverage.Lines != 2 || coverage.Malformed != 1 || len(coverage.Errors) != 2 {
		t.Fatalf("coverage = %#v", coverage)
	}
}

func TestSearchValidation(t *testing.T) {
	for _, options := range []Options{
		{Query: "", Limit: 1},
		{Query: "x", Limit: 0},
		{Query: "x", Limit: 1, Providers: []sessionlog.Provider{"other"}},
		{Query: "x", Limit: 1, Kind: "other"},
	} {
		if _, err := Search(options); err == nil {
			t.Fatalf("expected error for %#v", options)
		}
	}
}

func installFixtures(t *testing.T) {
	t.Helper()
	root := t.TempDir()
	claudeRoot := filepath.Join(root, "claude")
	codexRoot := filepath.Join(root, "codex")
	copyFixture(t, "claude.jsonl", filepath.Join(claudeRoot, "projects", "search-project", "session.jsonl"))
	copyFixture(t, "codex.jsonl", filepath.Join(codexRoot, "sessions", "2026", "07", "session.jsonl"))
	t.Setenv("CLAUDE_CONFIG_DIRS", claudeRoot)
	t.Setenv("CLAUDE_CONFIG_DIR", "")
	t.Setenv("CODEX_HOME", codexRoot)
}

func copyFixture(t *testing.T, name, target string) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, data, 0o600); err != nil {
		t.Fatal(err)
	}
}
