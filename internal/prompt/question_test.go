package prompt

import "testing"

func TestDetectQuestionFromApprovalPrompt(t *testing.T) {
	q := DetectQuestion("Allow this command?\n  1. Yes\n❯ 2. No\n")
	if q == nil {
		t.Fatal("expected a question")
	}
	if q.Prompt == "" {
		t.Fatal("expected a prompt line")
	}
	if len(q.Choices) != 2 {
		t.Fatalf("choices = %d, want 2", len(q.Choices))
	}
	if q.Choices[0].Number != 1 || q.Choices[1].Number != 2 {
		t.Fatalf("choice numbers = %#v", q.Choices)
	}
}

func TestDetectQuestionFromTrailingMenu(t *testing.T) {
	raw := `
╭──────────────────────────────────────────╮
│ Which approach do you prefer?              │
│                                            │
│ ❯ 1. Use a library                         │
│   2. Hand-roll the protocol                │
│   3. Skip it for now                       │
╰──────────────────────────────────────────╯
`
	q := DetectQuestion(raw)
	if q == nil {
		t.Fatal("expected a question")
	}
	if q.Prompt != "Which approach do you prefer?" {
		t.Fatalf("prompt = %q", q.Prompt)
	}
	if len(q.Choices) != 3 {
		t.Fatalf("choices = %d, want 3", len(q.Choices))
	}
	if q.Choices[1].Label != "Hand-roll the protocol" {
		t.Fatalf("choice 2 label = %q", q.Choices[1].Label)
	}
}

func TestDetectQuestionFromCodexCursorMenu(t *testing.T) {
	// Codex marks the current row with "›" instead of Claude's "❯".
	raw := `Do you trust the contents of this directory?
› 1. Yes, continue
  2. No, quit
`
	q := DetectQuestion(raw)
	if q == nil {
		t.Fatal("expected a question")
	}
	if len(q.Choices) != 2 {
		t.Fatalf("choices = %d, want 2", len(q.Choices))
	}
	if q.Choices[0].Label != "Yes, continue" {
		t.Fatalf("choice 1 label = %q", q.Choices[0].Label)
	}
}

func TestDetectQuestionFromCodexStructuredQuestion(t *testing.T) {
	// Codex's structured-question UI prints multi-line wrapped option labels
	// and a hint footer below the menu. The detector must look past those and
	// still recognise the menu as the active question.
	raw := `Some preamble explaining the trade-offs.

  Question 1/3 (3 unanswered)
  你希望監控資訊第一版放在哪裡？

  › 1. 新增 Monitor tab (Recommended)  Control 保持操作
                                       導向，監控集中在獨
                                       立頁面。
    2. 嵌入 Control                    在現有主畫面直接顯
                                       示關鍵指標。
    3. Peer modal 詳細                 只在點開單一機器時
                                       顯示詳細監控。
    4. None of the above               Optionally, add
                                       details in notes
                                       (tab).

  tab to add notes | enter to submit answer
  ←/→ to navigate questions | esc to interrupt
`
	q := DetectQuestion(raw)
	if q == nil {
		t.Fatal("expected a question")
	}
	if len(q.Choices) != 4 {
		t.Fatalf("choices = %d, want 4", len(q.Choices))
	}
	if q.Choices[0].Number != 1 || q.Choices[3].Number != 4 {
		t.Fatalf("choice numbers = %#v", q.Choices)
	}
	if q.Choices[0].Label != "新增 Monitor tab (Recommended)  Control 保持操作 導向，監控集中在獨 立頁面。" {
		t.Fatalf("choice 1 label = %q", q.Choices[0].Label)
	}
	if q.Choices[3].Label != "None of the above               Optionally, add details in notes (tab)." {
		t.Fatalf("choice 4 label = %q", q.Choices[3].Label)
	}
}

func TestDetectQuestionIgnoresProseNumberedList(t *testing.T) {
	// A numbered list with no selection cursor is the agent talking, not a
	// menu — it must not register as a question.
	raw := "Here is the plan:\n1. Read the file\n2. Edit it\n3. Run the tests\n"
	if q := DetectQuestion(raw); q != nil {
		t.Fatalf("expected no question, got %#v", q)
	}
}

func TestDetectQuestionIgnoresScrolledMenu(t *testing.T) {
	// A menu followed by fresh output is no longer the active question.
	raw := `❯ 1. Yes
  2. No
running the build...
compiling package one
compiling package two
compiling package three
done in 4.2s
`
	if q := DetectQuestion(raw); q != nil {
		t.Fatalf("expected no question, got %#v", q)
	}
}

func TestDetectQuestionNoMenuReturnsNil(t *testing.T) {
	if q := DetectQuestion("just some plain output\nnothing to pick here\n"); q != nil {
		t.Fatalf("expected no question, got %#v", q)
	}
}
