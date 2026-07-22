package sessionlog

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestDiscoverClaudeHonorsConfigDirsAndDeduplicates(t *testing.T) {
	root := t.TempDir()
	first := filepath.Join(root, "first")
	second := filepath.Join(root, "second")
	firstLog := touchLog(t, filepath.Join(first, "projects", "project-a", "a.jsonl"))
	secondLog := touchLog(t, filepath.Join(second, "projects", "project-b", "b.jsonl"))
	// Nested subagent logs were not part of agentspend discovery and remain out
	// of scope so extracting this reader cannot change spend totals.
	touchLog(t, filepath.Join(first, "projects", "project-a", "subagents", "ignored.jsonl"))
	t.Setenv("CLAUDE_CONFIG_DIRS", strings.Join([]string{second, first, first}, string(os.PathListSeparator)))
	t.Setenv("CLAUDE_CONFIG_DIR", filepath.Join(root, "ignored-single"))

	got, err := Discover(ProviderClaude)
	if err != nil {
		t.Fatal(err)
	}
	want := []Source{{Provider: ProviderClaude, Path: secondLog}, {Provider: ProviderClaude, Path: firstLog}}
	if !reflect.DeepEqual(got.Sources, want) || len(got.Errors) != 0 {
		t.Fatalf("discovery = %#v, want %#v", got, want)
	}
}

func TestDiscoverCodexWalksSessions(t *testing.T) {
	root := t.TempDir()
	t.Setenv("CODEX_HOME", root)
	wantPath := touchLog(t, filepath.Join(root, "sessions", "2026", "07", "rollout-redacted.jsonl"))
	touchLog(t, filepath.Join(root, "sessions", "2026", "07", "ignored.txt"))

	got, err := Discover(ProviderCodex)
	if err != nil {
		t.Fatal(err)
	}
	want := []Source{{Provider: ProviderCodex, Path: wantPath}}
	if !reflect.DeepEqual(got.Sources, want) || len(got.Errors) != 0 {
		t.Fatalf("discovery = %#v, want %#v", got, want)
	}
}

func TestDiscoverMissingRootIsEmpty(t *testing.T) {
	root := filepath.Join(t.TempDir(), "missing")
	t.Setenv("CODEX_HOME", root)
	got, err := Discover(ProviderCodex)
	if err != nil || len(got.Sources) != 0 || len(got.Errors) != 0 {
		t.Fatalf("discovery=%#v err=%v", got, err)
	}
}

func TestDiscoverRejectsUnknownProvider(t *testing.T) {
	if _, err := Discover("other"); err == nil || !strings.Contains(err.Error(), "unsupported") {
		t.Fatalf("err = %v", err)
	}
}

func touchLog(t *testing.T, path string) string {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		t.Fatal(err)
	}
	return abs
}
