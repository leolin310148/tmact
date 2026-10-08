package prompt

import (
	"strings"
	"testing"
	"time"
)

func TestDetectDirectoryAccessPrompt(t *testing.T) {
	raw := `
╭───────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────╮
│ Allow directory access                                                                                                    │
│ ───────────────────────────────────────────────────────────────────────────────────────────────────────────────────────── │
│ This action may read or write the following paths outside your allowed directory list.                                    │
│                                                                                                                           │
│ ╭───────────────────────────────────────────────────────────────────────────────────────────────────────────────────────╮ │
│ │ ../../sample-project/packages/cli/src/cli.ts, /status                                                                  │ │
│ ╰───────────────────────────────────────────────────────────────────────────────────────────────────────────────────────╯ │
│                                                                                                                           │
│ Do you want to allow this?                                                                                                │
│                                                                                                                           │
│   1. Yes                                                                                                                  │
│ ❯ 2. Yes, and add these directories to the allowed list                                                                   │
│   3. No (Esc)                                                                                                             │
│                                                                                                                           │
│ ↑↓ to navigate · Enter to select · Esc to cancel                                                                          │
╰───────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────╯
`

	detected := DetectDirectoryAccess(raw)
	if detected == nil {
		t.Fatal("expected prompt")
	}
	if detected.Title != "Allow directory access" {
		t.Fatalf("title = %q", detected.Title)
	}
	if detected.Path != "../../sample-project/packages/cli/src/cli.ts" {
		t.Fatalf("path = %q", detected.Path)
	}
	if len(detected.Paths) != 2 {
		t.Fatalf("paths len = %d", len(detected.Paths))
	}
	if detected.Paths[1] != "/status" {
		t.Fatalf("second path = %q", detected.Paths[1])
	}
	if detected.SelectedOption == nil {
		t.Fatal("expected selected option")
	}
	if detected.SelectedOption.Number != 2 {
		t.Fatalf("selected number = %d", detected.SelectedOption.Number)
	}
	if len(detected.Options) != 3 {
		t.Fatalf("options len = %d", len(detected.Options))
	}
}

func TestDetectDirectoryAccessIgnoresScrolledPrompt(t *testing.T) {
	raw := `
Allow directory access
This action may read or write the following paths outside your allowed directory list.
/tmp/project
Do you want to allow this?
  1. Yes
❯ 2. Yes, and add these directories to the allowed list
  3. No (Esc)
running tests...
compiling package one
compiling package two
compiling package three
done in 4.2s
project $
`

	if detected := DetectDirectoryAccess(raw); detected != nil {
		t.Fatalf("expected no prompt, got %#v", detected)
	}
}

func TestDirectoryAccessPromptConversionsCloneMutableFields(t *testing.T) {
	access := &DirectoryAccess{
		Title: "Allow directory access",
		Path:  "/tmp/project",
		Paths: []string{"/tmp/project"},
		SelectedOption: &Option{
			Number:   1,
			Label:    "Yes",
			Selected: true,
		},
		Options: []Option{{Number: 1, Label: "Yes", Selected: true}},
	}

	detected := PromptFromDirectoryAccess(access)
	access.Paths[0] = "/tmp/changed"
	access.SelectedOption.Label = "Changed"
	access.Options[0].Label = "Changed"

	if detected.Paths[0] != "/tmp/project" {
		t.Fatalf("prompt paths shared backing array: %#v", detected.Paths)
	}
	if detected.SelectedOption.Label != "Yes" {
		t.Fatalf("prompt selected option shared pointer: %#v", detected.SelectedOption)
	}
	if detected.Options[0].Label != "Yes" {
		t.Fatalf("prompt options shared backing array: %#v", detected.Options)
	}

	roundTripped := DirectoryAccessFromPrompt(detected)
	detected.Paths[0] = "/tmp/changed-again"
	detected.SelectedOption.Label = "Changed again"
	detected.Options[0].Label = "Changed again"

	if roundTripped.Paths[0] != "/tmp/project" {
		t.Fatalf("directory paths shared backing array: %#v", roundTripped.Paths)
	}
	if roundTripped.SelectedOption.Label != "Yes" {
		t.Fatalf("directory selected option shared pointer: %#v", roundTripped.SelectedOption)
	}
	if roundTripped.Options[0].Label != "Yes" {
		t.Fatalf("directory options shared backing array: %#v", roundTripped.Options)
	}
}

func TestDetectGenericCommandApprovalPrompt(t *testing.T) {
	raw := `
Allow this command?
  1. Yes
❯ 2. No
`

	detected := Detect(raw)
	if detected == nil {
		t.Fatal("expected prompt")
	}
	if detected.Type != TypeCommandApproval {
		t.Fatalf("type = %q", detected.Type)
	}
	if detected.SelectedOption == nil || detected.SelectedOption.Number != 2 {
		t.Fatalf("selected option = %#v", detected.SelectedOption)
	}
}

func TestDetectGenericCommandApprovalPromptWithCodexCursor(t *testing.T) {
	raw := `
Allow this command?
  1. Yes
› 2. No
`

	detected := Detect(raw)
	if detected == nil {
		t.Fatal("expected prompt")
	}
	if detected.Type != TypeCommandApproval {
		t.Fatalf("type = %q", detected.Type)
	}
	if detected.SelectedOption == nil || detected.SelectedOption.Number != 2 {
		t.Fatalf("selected option = %#v", detected.SelectedOption)
	}
}

func TestDetectGenericPromptIgnoresOSCTitleSequence(t *testing.T) {
	raw := "\x1b]0;tmact\aAllow this command?\n  1. Yes\n❯ 2. No\n"

	detected := Detect(raw)
	if detected == nil {
		t.Fatal("expected prompt")
	}
	if detected.Type != TypeCommandApproval {
		t.Fatalf("type = %q", detected.Type)
	}
}

func TestDetectGenericPromptIgnoresOSCTitleSequenceWithSTTerminator(t *testing.T) {
	raw := "\x1b]0;tmact\x1b\\Allow this command?\n  1. Yes\n❯ 2. No\n"

	detected := Detect(raw)
	if detected == nil {
		t.Fatal("expected prompt")
	}
	if detected.Type != TypeCommandApproval {
		t.Fatalf("type = %q", detected.Type)
	}
}

func TestDetectGenericCommandApprovalIgnoresScrolledPrompt(t *testing.T) {
	raw := `
Allow this command?
  1. Yes
❯ 2. No
running the build...
compiling package one
compiling package two
compiling package three
done in 4.2s
project $
`

	if detected := Detect(raw); detected != nil {
		t.Fatalf("expected no prompt, got %#v", detected)
	}
}

func TestDetectGenericPromptDoesNotClaimNewTrailingMenu(t *testing.T) {
	raw := `
Allow this command?
  1. Yes
❯ 2. No
running the build...
done in 4.2s
Choose next action
❯ 1. Run tests
  2. Commit changes
`

	detected := Detect(raw)
	if detected == nil {
		t.Fatal("expected prompt")
	}
	if detected.Type != TypeChoicePrompt {
		t.Fatalf("type = %q", detected.Type)
	}
	if detected.Question != "Choose next action" {
		t.Fatalf("question = %q", detected.Question)
	}
	if len(detected.Options) != 2 {
		t.Fatalf("options len = %d", len(detected.Options))
	}
}

func TestDetectTrustFolderPrompt(t *testing.T) {
	detected := Detect("Do you trust the files in this folder?\n1. Trust folder\n3. Don't trust\n")
	if detected == nil {
		t.Fatal("expected prompt")
	}
	if detected.Type != TypeTrustFolder {
		t.Fatalf("type = %q", detected.Type)
	}
}

func TestDetectCodexTrustDirectoryPrompt(t *testing.T) {
	detected := Detect("Do you trust the contents of this directory?\n› 1. Yes, continue\n  2. No, quit\n")
	if detected == nil || detected.Type != TypeTrustFolder {
		t.Fatalf("detected = %#v", detected)
	}
	if detected.SelectedOption == nil || detected.SelectedOption.Number != 1 {
		t.Fatalf("selected option = %#v", detected.SelectedOption)
	}
}

// TestDetectCodexFolderAccessTrustPrompt pins the screen Codex 0.161 renders
// for an untrusted folder: the question shares its line with the explanation
// and the decline row returns to the command center instead of quitting.
func TestDetectCodexFolderAccessTrustPrompt(t *testing.T) {
	raw := `
  Folder access
  /private/tmp/tmact-trust-probe

  Trust this folder? Codex can read, edit, and run files here, subject to your permission settings. Folder settings can run code
  automatically, even without a model request. Continue only if you trust these files. Your trust decision will be saved.

› 1. Trust and continue
  2. Back to Agent Command Center

  enter continue · esc back
`
	detected := Detect(raw)
	if detected == nil || detected.Type != TypeTrustFolder {
		t.Fatalf("detected = %#v", detected)
	}
	if len(detected.Options) != 2 || detected.SelectedOption == nil || detected.SelectedOption.Label != "Trust and continue" {
		t.Fatalf("options = %#v selected = %#v", detected.Options, detected.SelectedOption)
	}
}

func TestDetectClaudeQuickSafetyCheckTrustPrompt(t *testing.T) {
	raw := `
Accessing workspace:

/private/tmp/tmact-e2e-session-history

Quick safety check: Is this a project you created or one you trust?

Security guide

❯ 1. Yes, I trust this folder
  2. No, exit

Enter to confirm · Esc to cancel
`
	detected := Detect(raw)
	if detected == nil || detected.Type != TypeTrustFolder {
		t.Fatalf("detected=%#v", detected)
	}
	if detected.SelectedOption == nil || detected.SelectedOption.Number != 1 {
		t.Fatalf("selected option=%#v", detected.SelectedOption)
	}
}

// TestDetectClaudeCursorOnlyTrustPrompt pins the screen Claude 2.1.278 renders:
// the options lost their digits, leaving a cursor menu whose first row is the
// negative one. An undetected screen here reads as a ready agent, so the
// dispatched prompt confirms "No, exit" and lands in the shell instead.
func TestDetectClaudeCursorOnlyTrustPrompt(t *testing.T) {
	raw := `
────────────────────────────────────────────

 Accessing workspace:

 /private/tmp/tmact-trust-probe

 Quick safety check: Is this a project you created or one you trust? (Like your own code, a well-known open source project, or work from
 your team). If not, take a moment to review what's in this folder first.

 Claude Code'll be able to read, edit, and execute files here.

 Security guide

 ❯ No, exit
   Yes, I trust this folder

 Enter to confirm · Esc to cancel
`
	detected := Detect(raw)
	if detected == nil || detected.Type != TypeTrustFolder {
		t.Fatalf("detected=%#v", detected)
	}
	if detected.Path != "/private/tmp/tmact-trust-probe" {
		t.Fatalf("path=%q", detected.Path)
	}
	if len(detected.Options) != 2 ||
		detected.Options[0].Label != "No, exit" || !detected.Options[0].Selected ||
		detected.Options[1].Label != "Yes, I trust this folder" || detected.Options[1].Selected {
		t.Fatalf("options=%#v", detected.Options)
	}
	// A row with no digit must not advertise one: relaying "0" would confirm
	// whatever the cursor happens to sit on.
	for _, option := range detected.Options {
		if option.Number != 0 {
			t.Fatalf("option %q carries number %d", option.Label, option.Number)
		}
	}
	if question := DetectQuestion(raw); question != nil {
		t.Fatalf("cursor-only menu offered tappable choices: %#v", question)
	}
}

// TestDetectIgnoresWorkspaceTrustScreenWithDriftedLabels keeps detection exact:
// an unrecognized variant must fail closed rather than be auto-answered.
func TestDetectIgnoresWorkspaceTrustScreenWithDriftedLabels(t *testing.T) {
	raw := `
 Accessing workspace:

 /tmp/example

 Quick safety check: Is this a project you created or one you trust?

 ❯ No, take me out
   Sure, trust it

 Enter to confirm · Esc to cancel
`
	if detected := Detect(raw); detected != nil {
		t.Fatalf("detected=%#v", detected)
	}
}

func TestDetectGenericConfirmationPrompt(t *testing.T) {
	raw := `
Do you want to proceed?
  1. Yes
❯ 2. No
`

	detected := Detect(raw)
	if detected == nil {
		t.Fatal("expected prompt")
	}
	if detected.Type != TypeGenericConfirmation {
		t.Fatalf("type = %q", detected.Type)
	}
	if detected.SelectedOption == nil || detected.SelectedOption.Number != 2 {
		t.Fatalf("selected option = %#v", detected.SelectedOption)
	}
}

func TestDetectPatchApprovalPrompt(t *testing.T) {
	raw := `
Apply this patch?
  1. Yes
❯ 2. No
`

	detected := Detect(raw)
	if detected == nil {
		t.Fatal("expected prompt")
	}
	if detected.Type != TypePatchApproval {
		t.Fatalf("type = %q", detected.Type)
	}
	if detected.Title != "Apply this patch?" {
		t.Fatalf("title = %q", detected.Title)
	}
	if detected.SelectedOption == nil || detected.SelectedOption.Number != 2 {
		t.Fatalf("selected option = %#v", detected.SelectedOption)
	}
	if len(detected.Options) != 2 {
		t.Fatalf("options len = %d", len(detected.Options))
	}
}

func TestDetectWaitingApprovalPromptWithoutOptions(t *testing.T) {
	detected := Detect("Waiting for approval\n")
	if detected == nil {
		t.Fatal("expected prompt")
	}
	if detected.Type != TypeWaitingApproval {
		t.Fatalf("type = %q", detected.Type)
	}
	if detected.Title != "Waiting for approval" {
		t.Fatalf("title = %q", detected.Title)
	}
	if len(detected.Options) != 0 {
		t.Fatalf("options = %#v", detected.Options)
	}
}

func TestDetectTrailingChoicePrompt(t *testing.T) {
	raw := `
Skill 位置

4 個 skill 要放哪?這影響是否進版控、team 是否看得到、以及是否馬上在 worktree 可用。

❯ 1. 專案 .claude/skills/ (推薦)
  2. 個人 ~/.claude/skills/
  3. Type something.

Enter to select · ↑/↓ to navigate · Esc to cancel
`

	detected := Detect(raw)
	if detected == nil {
		t.Fatal("expected prompt")
	}
	if detected.Type != TypeChoicePrompt {
		t.Fatalf("type = %q", detected.Type)
	}
	if detected.Question != "4 個 skill 要放哪?這影響是否進版控、team 是否看得到、以及是否馬上在 worktree 可用。" {
		t.Fatalf("question = %q", detected.Question)
	}
	if detected.SelectedOption == nil || detected.SelectedOption.Number != 1 {
		t.Fatalf("selected option = %#v", detected.SelectedOption)
	}
	if len(detected.Options) != 3 {
		t.Fatalf("options len = %d", len(detected.Options))
	}
}

func TestIsCodexModelCapacityRetry(t *testing.T) {
	detected := &Prompt{
		Type:     TypeChoicePrompt,
		Question: "response, though it may be less capable of handling complex requests.",
		SelectedOption: &Option{
			Number: 1, Label: "Retry with a faster model", Selected: true,
		},
		Options: []Option{
			{Number: 1, Label: "Retry with a faster model", Selected: true},
			{Number: 2, Label: "Keep waiting"},
			{Number: 3, Label: "Learn more Press enter to confirm or esc to go back"},
		},
	}

	if !IsCodexModelCapacityRetry(detected) {
		t.Fatalf("prompt was not allowlisted: %#v", detected)
	}
}

func TestIsCodexModelCapacityRetryRejectsPermissionAndUnknownChoices(t *testing.T) {
	tests := []*Prompt{
		{
			Type: TypeCommandApproval, Title: "Allow this command?",
			SelectedOption: &Option{Number: 1, Label: "Yes", Selected: true},
			Options:        []Option{{Number: 1, Label: "Yes", Selected: true}, {Number: 2, Label: "No"}},
		},
		{
			Type:     TypeChoicePrompt,
			Question: "Choose how to continue",
			SelectedOption: &Option{
				Number: 1, Label: "Retry with a faster model", Selected: true,
			},
			Options: []Option{
				{Number: 1, Label: "Retry with a faster model", Selected: true},
				{Number: 2, Label: "Keep waiting"},
				{Number: 3, Label: "Approve filesystem access"},
			},
		},
	}

	for _, detected := range tests {
		if IsCodexModelCapacityRetry(detected) {
			t.Fatalf("unsafe prompt was allowlisted: %#v", detected)
		}
	}
}

func TestClaudeSessionLimitWaitAndResetAt(t *testing.T) {
	raw := `
You've hit your session limit · resets 2am (Asia/Taipei)
What do you want to do?
❯ 1. Stop and wait for limit to reset
  2. Upgrade your plan
`
	detected := Detect(raw)
	if !IsClaudeSessionLimitWait(raw, detected) {
		t.Fatalf("prompt was not allowlisted: %#v", detected)
	}
	location, err := time.LoadLocation("Asia/Taipei")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 8, 3, 23, 0, 0, 0, location)
	resetAt, ok := ClaudeSessionLimitResetAt(raw, detected, now)
	want := time.Date(2026, 8, 4, 2, 0, 0, 0, location)
	if !ok || !resetAt.Equal(want) {
		t.Fatalf("resetAt=%s ok=%t want=%s", resetAt, ok, want)
	}
}

func TestClaudeSessionLimitWaitFailsClosed(t *testing.T) {
	base := `
You've hit your session limit · resets 2am (Asia/Taipei)
What do you want to do?
❯ 1. Stop and wait for limit to reset
  2. Upgrade your plan
`
	tests := []string{
		"Allow this command?\n❯ 1. Stop and wait for limit to reset\n  2. Upgrade your plan\n",
		strings.Replace(base, "Upgrade your plan", "Approve filesystem access", 1),
		strings.Replace(base, "❯ 1.", "  1.", 1),
	}
	for _, raw := range tests {
		if detected := Detect(raw); IsClaudeSessionLimitWait(raw, detected) {
			t.Fatalf("unsafe prompt was allowlisted: %#v", detected)
		}
	}
}
