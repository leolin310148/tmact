package panestate

import "testing"

func TestClassifyDetectsDirectoryAccessPrompt(t *testing.T) {
	raw := `
╭─ Allow directory access ─╮
│ /tmp/sample              │
│ Do you want to allow this? │
│ 1. No                    │
│ ❯ 2. Yes, and add these directories to the allowed list │
╰──────────────────────────╯
`

	result := Classify(raw)
	if result.State != StateWaitingPermission {
		t.Fatalf("state = %q", result.State)
	}
	if !result.Asking {
		t.Fatal("result should be asking")
	}
	if result.Prompt == nil {
		t.Fatal("expected directory access prompt")
	}
}

func TestClassifyDetectsGenericApprovalQuestion(t *testing.T) {
	result := Classify("Waiting for approval\nsome status footer\n›\n")

	if result.State != StateWaitingPermission {
		t.Fatalf("state = %q", result.State)
	}
	if !result.Asking {
		t.Fatal("result should be asking")
	}
	if len(result.Signals) == 0 || result.Signals[0] != "asking_prompt" {
		t.Fatalf("signals = %#v", result.Signals)
	}
}

func TestClassifyDoesNotTreatNarrativeApprovalTextAsPrompt(t *testing.T) {
	result := Classify("舊 ClassifyPane 對 Waiting for approval 比較可能回 blocked\n›\n")

	if result.State != StateWaitingInput {
		t.Fatalf("state = %q", result.State)
	}
	if result.Asking {
		t.Fatal("narrative approval text should not be asking")
	}
}

func TestClassifyDetectsTrustPrompt(t *testing.T) {
	result := Classify("Do you trust the files in this folder?\n1. Trust folder\n3. Don't trust\n")

	if result.State != StateWaitingPermission {
		t.Fatalf("state = %q", result.State)
	}
	if !result.Asking {
		t.Fatal("result should be asking")
	}
	if len(result.Signals) == 0 || result.Signals[0] != "trust_prompt" {
		t.Fatalf("signals = %#v", result.Signals)
	}
}

func TestClassifyPrefersCurrentPromptOverStaleWorkingScrollback(t *testing.T) {
	result := Classify(`
I am working on the synthesis now.
tokens used
project $
`)

	if result.State != StateWaitingInput {
		t.Fatalf("state = %q", result.State)
	}
	if result.Asking {
		t.Fatal("idle prompt should not be asking")
	}
}

func TestClassifyDetectsWorkingText(t *testing.T) {
	result := Classify("Codex\nWorking\nEsc to interrupt")

	if result.State != StateWorking {
		t.Fatalf("state = %q", result.State)
	}
	if result.Asking {
		t.Fatal("working text should not be asking")
	}
}
