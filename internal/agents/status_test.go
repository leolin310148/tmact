package agents

import "testing"

func TestClassifyPaneDetectsDirectoryAccessPrompt(t *testing.T) {
	raw := `
╭─ Allow directory access ─╮
│ /tmp/sample              │
│ Do you want to allow this? │
│ 1. No                    │
│ ❯ 2. Yes, and add these directories to the allowed list │
╰──────────────────────────╯
`

	state, detected := ClassifyPane(raw)
	if state != StateWaitingPermission {
		t.Fatalf("state = %q", state)
	}
	if detected == nil {
		t.Fatal("expected prompt")
	}
}

func TestClassifyPaneDetectsWorking(t *testing.T) {
	state, _ := ClassifyPane("Codex\nWorking\nEsc to interrupt")
	if state != StateWorking {
		t.Fatalf("state = %q", state)
	}
}

func TestClassifyPanePrefersCurrentShellPromptOverStaleWorkingScrollback(t *testing.T) {
	state, _ := ClassifyPane(`
I am working on the synthesis now.
tokens used
project $
`)
	if state != StateIdle {
		t.Fatalf("state = %q", state)
	}
}

func TestClassifyPaneDetectsIdlePrompt(t *testing.T) {
	state, _ := ClassifyPane("ready\nproject $")
	if state != StateIdle {
		t.Fatalf("state = %q", state)
	}
}

func TestClassifyPaneIgnoresAgentPromptPlaceholder(t *testing.T) {
	state, _ := ClassifyPane(`
⏺ feedback review skipped
✻ Cooked for 23s
❯ continue
30k/1M  11%/3:30AM  T3  none  O(H)  Cost: $1.48
`)
	if state != StateIdle {
		t.Fatalf("state = %q", state)
	}
}

func TestClassifyPaneIgnoresClaudeWelcomeChrome(t *testing.T) {
	state, _ := ClassifyPane(`
╭─── Claude Code v2.1.133 ─────────────────────────────────────────────────────╮
│                  Welcome back Leo!                 │ Run /init to create a … │
│                                                    │ What's new              │
│               ~/workspace/example                │ /release-notes for more │
╰──────────────────────────────────────────────────────────────────────────────╯

❯ /clear
  ⎿  (no content)
❯
~  10%/3:30AM  T2  none  O(H)  Cost: $1.26
`)
	if state != StateIdle {
		t.Fatalf("state = %q", state)
	}
}

func TestLastMeaningfulLineCleansBlankLines(t *testing.T) {
	got := LastMeaningfulLine("\nfirst\n\nsecond\n")
	if got != "second" {
		t.Fatalf("last line = %q", got)
	}
}
