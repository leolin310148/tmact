package dispatch

import (
	"strings"
	"testing"
	"time"
)

func TestSubmitPromptStyledInputDoesNotResubmitSuggestionOrPartialDraft(t *testing.T) {
	for _, tc := range []struct {
		name, raw, ansi string
		wantError       bool
	}{
		{
			name: "suggestion repeats submitted prompt",
			raw:  "❯ do the thing\n⏺ done\n❯ do the thing\nCost: $0.01",
			ansi: "❯ do the thing\n⏺ done\n❯ \x1b[2mdo the thing\x1b[22m\nCost: $0.01",
		},
		{
			name:      "partial paste",
			raw:       "❯ only the tail\nCost: $0.01",
			ansi:      "❯ \x1b[0monly the tail\nCost: $0.01",
			wantError: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			pastes := 0
			deps := Deps{
				PasteText:       func(string, string, bool) error { pastes++; return nil },
				CapturePane:     func(string, int) (string, error) { return tc.raw, nil },
				CapturePaneANSI: func(string, int) (string, error) { return tc.ansi, nil },
				Sleep:           func(time.Duration) {},
				SendKeys:        func(string, []string) error { t.Fatal("unexpected Enter retry"); return nil },
			}
			_, err := submitPrompt(Options{Agent: "claude", Prompt: "do the thing"}, deps, "%7")
			if (err != nil) != tc.wantError || (err != nil && !strings.Contains(err.Error(), "partial paste")) || pastes != 1 {
				t.Fatalf("pastes=%d err=%v", pastes, err)
			}
		})
	}
}
