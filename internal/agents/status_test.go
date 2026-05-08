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

func TestClassifyPaneDetectsIdlePrompt(t *testing.T) {
	state, _ := ClassifyPane("ready\nproject $")
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
