package dispatch

import (
	"fmt"
	"strings"
	"unicode"

	"github.com/leolin310148/tmact/internal/panestate"
)

// submitPrompt pastes the prompt and confirms the agent actually accepted it.
// Three cold-start failures are recovered here: a transient startup
// notification (e.g. an MCP warning) can swallow the trailing Enter, leaving
// the prompt sitting in the input box; the agent UI can still be painting
// when the paste lands and drop the text entirely; or a fast prompt can
// finish between polls so "working" is never observed. After pasting we poll
// the pane and decide from where the prompt text ended up: gone from the
// input box means it was submitted, still in the box means re-send Enter, and
// nowhere at all means re-paste.
func submitPrompt(opts Options, deps Deps, target string) error {
	if err := deps.PasteText(target, opts.Prompt, true); err != nil {
		return err
	}
	for retry := 0; ; retry++ {
		deps.Sleep(submitSettleDelay)
		raw, err := deps.CapturePane(target, captureLines)
		if err != nil {
			return fmt.Errorf("confirm prompt submitted: %w", err)
		}
		if promptSubmitted(panestate.Classify(raw)) {
			return nil
		}
		inBox := promptInInputBox(raw, opts.Prompt)
		// The prompt left the input box and is somewhere on screen: it was
		// submitted, even if a fast task already finished and the agent is
		// idle again.
		if !inBox && promptVisible(raw, opts.Prompt) {
			return nil
		}
		if retry >= submitRetries {
			return fmt.Errorf("prompt was pasted but %s did not accept it after %d attempts", opts.Agent, submitRetries+1)
		}
		if inBox {
			// A cold start swallowed the trailing Enter.
			if err := deps.SendKeys(target, []string{"Enter"}); err != nil {
				return fmt.Errorf("re-send enter: %w", err)
			}
			continue
		}
		// The paste never landed — the agent UI dropped it while painting.
		if err := deps.PasteText(target, opts.Prompt, true); err != nil {
			return fmt.Errorf("re-paste prompt: %w", err)
		}
	}
}

// promptVisible reports whether the pasted prompt is shown anywhere in the
// pane. It compares the prompt and the capture reduced to letters and digits
// only, so line wrapping, input-box borders, and prompt markers cannot hide a
// prompt that did land.
func promptVisible(raw, prompt string) bool {
	needle := alnumOnly(prompt)
	if needle == "" {
		return true
	}
	if len(needle) > 60 {
		needle = needle[:60]
	}
	return strings.Contains(alnumOnly(raw), needle)
}

// promptInInputBox reports whether the prompt is still sitting in the agent's
// live input box rather than having been submitted. The live input box is the
// tail of the capture after the last prompt marker (❯ or ›); a submitted
// prompt has moved up into the transcript and is no longer in that tail.
func promptInInputBox(raw, prompt string) bool {
	idx := strings.LastIndexAny(raw, "❯›")
	if idx < 0 {
		return false
	}
	return promptVisible(raw[idx:], prompt)
}

func alnumOnly(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// promptSubmitted reports whether a pasted prompt left the input box. A pane
// that is working, blocked, or waiting on its own prompt has accepted the
// submission; anything still input-ready means the trailing Enter was
// swallowed. Re-sending Enter is never done while Asking, so an allowlisted
// permission prompt is not auto-confirmed.
func promptSubmitted(classified panestate.Result) bool {
	if classified.Asking {
		return true
	}
	switch classified.State {
	case panestate.StateWorking, panestate.StateBlocked:
		return true
	default:
		return false
	}
}

func promptDetail(prompt string) string {
	prompt = strings.Join(strings.Fields(prompt), " ")
	if len(prompt) > 60 {
		return prompt[:57] + "..."
	}
	return prompt
}
