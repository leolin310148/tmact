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
