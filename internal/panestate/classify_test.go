package panestate

import (
	"strings"
	"testing"
	"unicode/utf8"
)

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

func TestClassifyDetectsTrailingChoicePrompt(t *testing.T) {
	result := Classify(`
Skill 位置

4 個 skill 要放哪?這影響是否進版控、team 是否看得到、以及是否馬上在 worktree 可用。

❯ 1. 專案 .claude/skills/ (推薦)
  2. 個人 ~/.claude/skills/
  3. Type something.

Enter to select · ↑/↓ to navigate · Esc to cancel
`)

	if result.State != StateWaitingPermission {
		t.Fatalf("state = %q", result.State)
	}
	if !result.Asking {
		t.Fatal("result should be asking")
	}
	if result.InteractivePrompt == nil {
		t.Fatal("expected interactive prompt")
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

func TestClassifyPrefersClaudePromptAboveIdleFooterOverStaleWorkingScrollback(t *testing.T) {
	cases := []struct {
		name      string
		statusBar string
	}{
		{name: "model status", statusBar: "example-web | Opus 4.8 (1M context) | high | ctx:13% | master"},
		{name: "cwd status", statusBar: "/work/repos/example-web | main | ctx:13%"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result := Classify(`
I am working on the synthesis now.
用戶目前無待辦。
❯
` + tc.statusBar + `
⏵⏵ auto mode on (shift+tab to cycle) · ← for agents
`)

			if result.State != StateWaitingInput {
				t.Fatalf("state = %q", result.State)
			}
			if result.Asking {
				t.Fatal("idle Claude prompt should not be asking")
			}
		})
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

func TestClassifyDetectsExactCodexUsageLimit(t *testing.T) {
	result := Classify(`
› Do the work
■ You've hit your usage limit. Visit https://chatgpt.com/codex/settings/usage to purchase more credits or try again at Aug 18th, 2026 9:34
AM.
› Summarize recent commits
/work/repo · Context 0% used
`)
	if result.State != StateWaitingQuota || result.UsageLimit == nil || result.UsageLimit.Provider != "codex" {
		t.Fatalf("result=%#v", result)
	}
}

func TestClassifyBlocksMalformedCurrentCodexUsageLimit(t *testing.T) {
	result := Classify(`
■ You've hit your usage limit. Visit https://chatgpt.com/codex/settings/usage to purchase more credits or try again sometime.
› Summarize recent commits
/work/repo · Context 0% used
`)
	if result.State != StateBlocked || len(result.Signals) != 1 || result.Signals[0] != "usage_limit_unrecognized" {
		t.Fatalf("result=%#v", result)
	}
}

func TestClassifyTreatsActiveSteeringInputAsWorking(t *testing.T) {
	result := Classify(`
• Working (1s • esc to interrupt)

› Implement {feature}

/private/tmp/repo · Context 0% used
`)

	if result.State != StateWorking {
		t.Fatalf("state = %q, signals = %#v", result.State, result.Signals)
	}
}

func TestClassifyTreatsClaudeSpinnerWithoutInterruptHintAsWorking(t *testing.T) {
	result := Classify(`
✻ Frosting… (2m 51s)

❯

⏵⏵ auto mode on (shift+tab to cycle) · ← for agents
`)

	if result.State != StateWorking {
		t.Fatalf("state = %q, signals = %#v", result.State, result.Signals)
	}
}

func TestClassifyDoesNotReviveStaleClaudeSpinnerAfterCompletion(t *testing.T) {
	result := Classify(`
✻ Frosting… (2m 51s)

Done. All tests pass.

❯

⏵⏵ auto mode on (shift+tab to cycle) · ← for agents
`)

	if result.State != StateWaitingInput {
		t.Fatalf("state = %q, signals = %#v", result.State, result.Signals)
	}
}

func TestClassifyDoesNotReviveStaleInterruptIndicatorAfterCompletion(t *testing.T) {
	result := Classify(`
• Working (46s • esc to interrupt)

• TRUST_REGRESSION_READY

› Implement {feature}

/private/tmp/repo · Context 2% used
`)

	if result.State != StateWaitingInput {
		t.Fatalf("state = %q, signals = %#v", result.State, result.Signals)
	}
}

func TestLastMeaningfulLineTruncatesWithoutSplittingUTF8(t *testing.T) {
	prefix := strings.Repeat("a", 179)
	got := LastMeaningfulLine(prefix + "界")

	if !utf8.ValidString(got) {
		t.Fatalf("last line is invalid UTF-8: %q", got)
	}
	if got != prefix+"..." {
		t.Fatalf("last line = %q, want %q", got, prefix+"...")
	}
}
