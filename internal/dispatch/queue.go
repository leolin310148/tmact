package dispatch

import (
	"fmt"

	"github.com/leolin310148/tmact/internal/panestate"
)

// dispatchQueue types the prompt into a same-agent pane that is busy working.
// Claude queues a message submitted mid-turn and Codex holds it for its next
// tool call or steers it into the running turn, so no idle window is needed.
func dispatchQueue(opts Options, deps Deps, report Report, target string) (Report, error) {
	steps := []Step{{Name: "queue-prompt", Detail: promptDetail(opts.Prompt), Status: StatusPlanned}}
	if !opts.Execute {
		report.Steps = steps
		return report, nil
	}
	submission, err := submitQueued(opts, deps, target)
	if err != nil {
		steps[0].Status = StatusFailed
		report.Steps = steps
		return report, fmt.Errorf("queue prompt: %w", err)
	}
	steps[0].Status = StatusOK
	report.Steps = steps
	return finishDispatch(opts, deps, report, target, submission)
}

// submitQueued pastes the prompt without Enter, confirms it landed in the
// input box of a pane that is not asking, and only then presses Enter. A
// working pane can raise a permission prompt at any moment; splitting the
// paste from the Enter keeps a trailing Enter from confirming one.
//
// The working state cannot prove acceptance here, and agents collapse long
// pastes into a "[Pasted text]" marker, so the input box's non-dim draft is
// the evidence: present after the paste, gone after Enter. Matching the prompt
// text would be fooled by a dim suggestion echoing it, and Enter on such a
// suggestion submits it. The prompt is re-pasted only while the box holds no
// draft.
func submitQueued(opts Options, deps Deps, target string) (submissionEvidence, error) {
	if deps.CapturePaneANSI == nil {
		return submissionEvidence{}, fmt.Errorf("styled capture is required to check the input box")
	}
	_, draft, err := captureInput(deps, target)
	if err != nil {
		return submissionEvidence{}, fmt.Errorf("check input draft: %w", err)
	}
	if draft {
		return submissionEvidence{}, fmt.Errorf("%s input contains an unsent draft; refusing to append to it", opts.Agent)
	}

	if err := deps.PasteText(target, opts.Prompt, false); err != nil {
		return submissionEvidence{}, err
	}
	for retry := 0; ; retry++ {
		deps.Sleep(submitSettleDelay)
		classified, draft, err := captureInput(deps, target)
		if err != nil {
			return submissionEvidence{}, fmt.Errorf("confirm prompt pasted: %w", err)
		}
		if classified.Asking {
			return submissionEvidence{}, fmt.Errorf("%s started waiting on a prompt (%s) before Enter; the pasted text was left unsubmitted", opts.Agent, promptKind(classified))
		}
		if draft {
			break
		}
		if retry >= submitRetries {
			return submissionEvidence{}, fmt.Errorf("prompt was pasted but never appeared in the %s input after %d attempts", opts.Agent, submitRetries+1)
		}
		// The paste never landed — the agent UI dropped it while painting.
		if err := deps.PasteText(target, opts.Prompt, false); err != nil {
			return submissionEvidence{}, fmt.Errorf("re-paste prompt: %w", err)
		}
	}

	if err := deps.SendKeys(target, []string{"Enter"}); err != nil {
		return submissionEvidence{}, fmt.Errorf("send enter: %w", err)
	}
	for retry := 0; ; retry++ {
		deps.Sleep(submitSettleDelay)
		classified, draft, err := captureInput(deps, target)
		if err != nil {
			return submissionEvidence{}, fmt.Errorf("confirm prompt queued: %w", err)
		}
		if !draft {
			return newSubmissionEvidence(classified, "prompt_queued"), nil
		}
		// Never re-send Enter while asking: it would confirm the prompt.
		if classified.Asking {
			return submissionEvidence{}, fmt.Errorf("%s started waiting on a prompt (%s) before the queued prompt was confirmed", opts.Agent, promptKind(classified))
		}
		if retry >= submitRetries {
			return submissionEvidence{}, fmt.Errorf("prompt is still in the %s input after %d Enter attempts", opts.Agent, submitRetries+1)
		}
		if err := deps.SendKeys(target, []string{"Enter"}); err != nil {
			return submissionEvidence{}, fmt.Errorf("re-send enter: %w", err)
		}
	}
}

// captureInput captures the pane once in plain and styled form and reports
// whether its live input line holds operator (non-dim) text.
func captureInput(deps Deps, target string) (panestate.Result, bool, error) {
	raw, err := deps.CapturePane(target, captureLines)
	if err != nil {
		return panestate.Result{}, false, err
	}
	ansi, err := deps.CapturePaneANSI(target, captureLines)
	if err != nil {
		return panestate.Result{}, false, fmt.Errorf("capture styled pane: %w", err)
	}
	return panestate.ClassifyANSI(raw, ansi), panestate.HasInputDraft(ansi), nil
}
