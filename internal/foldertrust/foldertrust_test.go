package foldertrust

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/leolin310148/tmact/internal/panestatus"
	"github.com/leolin310148/tmact/internal/prompt"
	"github.com/leolin310148/tmact/internal/tmux"
)

func TestRunWaitsForPromptToClearAfterAcceptance(t *testing.T) {
	dir := t.TempDir()
	now := time.Unix(0, 0)
	accepted := false
	deps := Deps{
		ListPanes: func(string) ([]tmux.Pane, error) {
			return []tmux.Pane{{PanePID: 7, CurrentPath: dir, CurrentCommand: "codex"}}, nil
		},
		CapturePane: func(string, int) (string, error) {
			if !accepted {
				return "OpenAI Codex\nDo you trust the contents of this directory?\n› 1. Yes, continue\n  2. No, quit\n", nil
			}
			return "OpenAI Codex\n› ", nil
		},
		SendKeys: func(string, []string) error { accepted = true; return nil },
		ProcessRuntime: func(int) panestatus.RuntimeDetection {
			return panestatus.RuntimeDetection{Runtime: panestatus.RuntimeUnknown}
		},
		StartCommand: func(string) (string, error) { return "codex", nil },
		Now:          func() time.Time { return now },
		Sleep:        func(d time.Duration) { now = now.Add(d) },
	}
	result, err := RunWithDeps(context.Background(), Options{Target: "%7", Dir: dir, Agent: "codex", Timeout: 5 * time.Second}, deps)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Accepted || !accepted {
		t.Fatalf("result=%#v accepted=%t", result, accepted)
	}
}

func TestAcceptPromptAcceptsSelectedCodexTrustForExactDirectory(t *testing.T) {
	dir := t.TempDir()
	var gotTarget string
	var gotKeys []string
	result, err := AcceptPrompt(Options{
		Target: "%7",
		Dir:    dir,
		Agent:  panestatus.RuntimeCodex,
	}, tmux.Pane{CurrentPath: dir}, "Do you trust the contents of this directory?\n› 1. Yes, continue\n  2. No, quit\n", panestatus.RuntimeCodex, func(target string, keys []string) error {
		gotTarget = target
		gotKeys = append([]string(nil), keys...)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Accepted || result.OptionNumber != 1 {
		t.Fatalf("result = %#v", result)
	}
	if gotTarget != "%7" || len(gotKeys) != 1 || gotKeys[0] != "Enter" {
		t.Fatalf("target=%q keys=%#v", gotTarget, gotKeys)
	}
}

func TestAcceptPromptSelectsUnselectedClaudeTrustOptionByNumber(t *testing.T) {
	dir := t.TempDir()
	var gotKeys []string
	result, err := AcceptPrompt(Options{Target: "%8", Dir: dir, Agent: panestatus.RuntimeClaude},
		tmux.Pane{CurrentPath: dir},
		"Do you trust the files in this folder?\n1. Trust folder and continue\n2. No, exit\n",
		panestatus.RuntimeClaude,
		func(_ string, keys []string) error {
			gotKeys = append([]string(nil), keys...)
			return nil
		})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Accepted || len(gotKeys) != 1 || gotKeys[0] != "1" {
		t.Fatalf("result=%#v keys=%#v", result, gotKeys)
	}
}

// TestAcceptPromptArrowsToUnnumberedClaudeTrustOption covers Claude 2.1.278's
// cursor-only trust menu: with no digit to send, the affirmative row is reached
// by moving the cursor down from the default "No, exit".
func TestAcceptPromptArrowsToUnnumberedClaudeTrustOption(t *testing.T) {
	dir := t.TempDir()
	var gotKeys []string
	raw := " Accessing workspace:\n\n " + dir + "\n\n Quick safety check: Is this a project you created or one you trust?\n\n \u276f No, exit\n   Yes, I trust this folder\n\n Enter to confirm \u00b7 Esc to cancel\n"
	result, err := AcceptPrompt(Options{Target: "%8", Dir: dir, Agent: panestatus.RuntimeClaude},
		tmux.Pane{CurrentPath: dir},
		raw,
		panestatus.RuntimeClaude,
		func(_ string, keys []string) error {
			gotKeys = append([]string(nil), keys...)
			return nil
		})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Accepted || result.OptionLabel != "Yes, I trust this folder" {
		t.Fatalf("result=%#v", result)
	}
	if len(gotKeys) != 2 || gotKeys[0] != "Down" || gotKeys[1] != "Enter" {
		t.Fatalf("keys=%#v, want Down then Enter", gotKeys)
	}
}

// TestSelectionKeysRefusesUnnumberedMenuWithoutCursor makes the cursor-only
// menu fail closed when no row is marked: there is no digit to fall back to,
// and pressing Enter would confirm whatever is currently highlighted.
func TestSelectionKeysRefusesUnnumberedMenuWithoutCursor(t *testing.T) {
	detected := &prompt.Prompt{
		Type: prompt.TypeTrustFolder,
		Options: []prompt.Option{
			{Label: "No, exit"},
			{Label: "Yes, I trust this folder"},
		},
	}
	option, index, err := affirmativeOption(detected)
	if err != nil {
		t.Fatal(err)
	}
	keys, err := selectionKeys(detected, index, option.Number)
	if err == nil {
		t.Fatalf("expected a cursorless unnumbered menu to be refused, got keys=%#v", keys)
	}
}

func TestAcceptPromptRefusesDifferentPaneDirectory(t *testing.T) {
	dir := t.TempDir()
	other := t.TempDir()
	called := false
	_, err := AcceptPrompt(Options{Target: "%9", Dir: dir, Agent: panestatus.RuntimeCodex},
		tmux.Pane{CurrentPath: other},
		"Do you trust the contents of this directory?\n› 1. Yes, continue\n  2. No, quit\n",
		panestatus.RuntimeCodex,
		func(string, []string) error { called = true; return nil })
	if err == nil || !strings.Contains(err.Error(), "does not exactly match") {
		t.Fatalf("err = %v", err)
	}
	if called {
		t.Fatal("mismatched cwd must not send keys")
	}
}

func TestAcceptPromptComparesCanonicalSymlinkPaths(t *testing.T) {
	realDir := t.TempDir()
	link := filepath.Join(t.TempDir(), "repo-link")
	if err := os.Symlink(realDir, link); err != nil {
		t.Fatal(err)
	}
	result, err := AcceptPrompt(Options{Target: "%9", Dir: link, Agent: panestatus.RuntimeCodex},
		tmux.Pane{CurrentPath: realDir},
		"Do you trust the contents of this directory?\n› 1. Yes, continue\n  2. No, quit\n",
		panestatus.RuntimeCodex,
		func(string, []string) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	canonical, err := filepath.EvalSymlinks(realDir)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Accepted || result.Dir != canonical {
		t.Fatalf("result = %#v", result)
	}
}

func TestAcceptPromptRefusesWrongRuntime(t *testing.T) {
	dir := t.TempDir()
	_, err := AcceptPrompt(Options{Target: "%9", Dir: dir, Agent: panestatus.RuntimeClaude},
		tmux.Pane{CurrentPath: dir},
		"Do you trust the files in this folder?\n❯ 1. Yes, proceed\n  2. No, exit\n",
		panestatus.RuntimeCodex,
		func(string, []string) error { return nil })
	if err == nil || !strings.Contains(err.Error(), "expected \"claude\"") {
		t.Fatalf("err = %v", err)
	}
}

func TestAcceptPromptDryRunReportsWithoutSending(t *testing.T) {
	dir := t.TempDir()
	result, err := AcceptPrompt(Options{Target: "%10", Dir: dir, Agent: panestatus.RuntimeCodex, DryRun: true},
		tmux.Pane{CurrentPath: dir},
		"Do you trust the contents of this directory?\n› 1. Yes, continue\n  2. No, quit\n",
		panestatus.RuntimeCodex,
		func(string, []string) error { t.Fatal("dry-run sent keys"); return nil })
	if err != nil {
		t.Fatal(err)
	}
	if !result.PromptFound || result.Accepted || !result.DryRun {
		t.Fatalf("result = %#v", result)
	}
}
