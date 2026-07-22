package dispatch

import (
	"context"
	"errors"
	"fmt"

	"github.com/leolin310148/tmact/internal/panewait"
)

func finishDispatch(opts Options, deps Deps, report Report, target string, submission submissionEvidence) (Report, error) {
	if !opts.Wait {
		return report, nil
	}
	report.Wait.Baseline = &submission.baseline
	if !submission.baseline.Accepted {
		setWaitStepStatus(&report, StatusFailed)
		report.Wait.Status = StatusFailed
		return report, fmt.Errorf("dispatch wait requires evidence that the prompt was accepted")
	}
	if deps.WaitPane == nil {
		setWaitStepStatus(&report, StatusFailed)
		report.Wait.Status = StatusFailed
		return report, fmt.Errorf("dispatch wait dependency is not configured")
	}
	if deps.CapturePaneContext == nil {
		setWaitStepStatus(&report, StatusFailed)
		report.Wait.Status = StatusFailed
		return report, fmt.Errorf("dispatch wait capture dependency is not configured")
	}

	waitCtx, cancel := context.WithTimeout(opts.Context, opts.WaitTimeout)
	defer cancel()

	waited, err := deps.WaitPane(waitCtx, panewait.Options{
		Selector:     target,
		Until:        panewait.UntilInputReady,
		Settle:       opts.WaitSettle,
		PollInterval: defaultWaitPollInterval,
		Timeout:      opts.WaitTimeout,
	})
	if err != nil {
		if dispatchDeadlineExpired(opts.Context, waitCtx) {
			waited = panewait.Report{
				Selector: target,
				Target:   target,
				Until:    panewait.UntilInputReady,
				State:    panewait.StateUnknown,
				Reason:   panewait.ReasonTimeout,
				Elapsed:  opts.WaitTimeout,
			}
			report.Wait.Outcome = newWaitOutcome(waited)
			setWaitStepStatus(&report, StatusFailed)
			report.Wait.Status = StatusFailed
			return report, fmt.Errorf("dispatch wait ended before input-ready: %s", waited.Reason)
		}
		setWaitStepStatus(&report, StatusFailed)
		report.Wait.Status = StatusFailed
		return report, fmt.Errorf("wait for dispatch result: %w", err)
	}
	report.Wait.Outcome = newWaitOutcome(waited)

	if waited.Reason != panewait.ReasonPaneGone {
		text, captureErr := deps.CapturePaneContext(waitCtx, target, opts.ResultLines)
		if captureErr != nil {
			if dispatchDeadlineExpired(opts.Context, waitCtx) {
				waited.Reason = panewait.ReasonTimeout
				waited.ConditionMet = false
				if waited.State == "" {
					waited.State = panewait.StateUnknown
				}
				report.Wait.Outcome = newWaitOutcome(waited)
				setWaitStepStatus(&report, StatusFailed)
				report.Wait.Status = StatusFailed
				return report, fmt.Errorf("dispatch wait ended before input-ready: %s", waited.Reason)
			}
			setWaitStepStatus(&report, StatusFailed)
			report.Wait.Status = StatusFailed
			return report, fmt.Errorf("capture dispatch result: %w", captureErr)
		}
		report.Result = &ResultReport{Lines: opts.ResultLines, Text: text}
	}

	if !waited.ConditionMet {
		setWaitStepStatus(&report, StatusFailed)
		report.Wait.Status = StatusFailed
		return report, fmt.Errorf("dispatch wait ended before input-ready: %s", waited.Reason)
	}
	setWaitStepStatus(&report, StatusOK)
	report.Wait.Status = StatusOK
	return report, nil
}

func dispatchDeadlineExpired(parent, waitCtx context.Context) bool {
	return !errors.Is(parent.Err(), context.Canceled) && errors.Is(waitCtx.Err(), context.DeadlineExceeded)
}

func newWaitOutcome(report panewait.Report) *WaitOutcome {
	return &WaitOutcome{
		Target:             report.Target,
		PaneID:             report.PaneID,
		State:              report.State,
		RawState:           report.RawState,
		Reason:             report.Reason,
		ConditionMet:       report.ConditionMet,
		TransitionObserved: report.TransitionObserved,
		Samples:            report.Samples,
		LastLine:           report.LastLine,
		Signals:            append([]string(nil), report.Signals...),
		Elapsed:            report.Elapsed.String(),
	}
}

func setWaitStepStatus(report *Report, status string) {
	for i := range report.Steps {
		if report.Steps[i].Name == "wait-result" {
			report.Steps[i].Status = status
			return
		}
	}
}
