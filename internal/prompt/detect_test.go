package prompt

import "testing"

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

func TestDetectTrustFolderPrompt(t *testing.T) {
	detected := Detect("Do you trust the files in this folder?\n1. Trust folder\n3. Don't trust\n")
	if detected == nil {
		t.Fatal("expected prompt")
	}
	if detected.Type != TypeTrustFolder {
		t.Fatalf("type = %q", detected.Type)
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
