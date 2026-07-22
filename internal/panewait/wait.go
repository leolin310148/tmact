// Package panewait implements bounded, read-only waiting for tmux pane state
// transitions. It never sends input or answers prompts.
package panewait

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/leolin310148/tmact/internal/panestate"
	"github.com/leolin310148/tmact/internal/tmux"
)

const (
	UntilInputReady = "input-ready"
	UntilWorking    = "working"
	UntilNeedsHuman = "needs-human"
	UntilGone       = "gone"

	ReasonConditionMet = "condition_met"
	ReasonNeedsHuman   = "needs_human"
	ReasonTimeout      = "timeout"
	ReasonPaneGone     = "pane_gone"

	StateUnknown = "unknown"
	StateGone    = "gone"

	captureLines = 200
)

var validConditions = map[string]bool{
	UntilInputReady: true,
	UntilWorking:    true,
	UntilNeedsHuman: true,
	UntilGone:       true,
}

// Options configures one bounded wait.
type Options struct {
	Selector          string
	Until             string
	RequireTransition bool
	Settle            time.Duration
	PollInterval      time.Duration
	Timeout           time.Duration
}

// Report is the terminal observation from a wait. ConditionMet means the
// requested condition was observed; it never means the pane's task succeeded.
type Report struct {
	Selector           string
	Target             string
	PaneID             string
	Until              string
	State              string
	RawState           string
	Reason             string
	ConditionMet       bool
	TransitionObserved bool
	Samples            int
	LastLine           string
	Signals            []string
	StartedAt          time.Time
	FinishedAt         time.Time
	Elapsed            time.Duration
}

// Dependencies isolates tmux reads and time so waits can be tested without a
// live tmux server or wall-clock sleeps.
type Dependencies struct {
	ResolveTarget func(string) (tmux.CapturePaneInfo, error)
	CapturePane   func(context.Context, string, int) (string, error)
	IsTargetGone  func(error) bool
	Now           func() time.Time
	Wait          func(context.Context, time.Duration) error
}

// DefaultDependencies wires the wait to read-only tmux helpers.
func DefaultDependencies() Dependencies {
	return Dependencies{
		ResolveTarget: tmux.CapturePaneInfoForTarget,
		CapturePane:   tmux.CapturePaneContext,
		IsTargetGone:  tmux.IsTargetGoneError,
		Now:           time.Now,
		Wait:          waitContext,
	}
}

// Run waits using the real clock and tmux helpers.
func Run(ctx context.Context, options Options) (Report, error) {
	return RunWithDependencies(ctx, options, DefaultDependencies())
}

// RunWithDependencies waits until a requested state or a terminal blocker is
// observed. Prompt states always preempt settling and transition requirements.
func RunWithDependencies(ctx context.Context, options Options, deps Dependencies) (Report, error) {
	if err := validate(options, deps); err != nil {
		return Report{}, err
	}
	if ctx == nil {
		return Report{}, errors.New("wait context is required")
	}

	started := deps.Now()
	report := Report{
		Selector:  options.Selector,
		Target:    options.Selector,
		Until:     options.Until,
		State:     StateUnknown,
		StartedAt: started,
	}
	deadline := started.Add(options.Timeout)
	lookupTarget := options.Selector
	resolved := false
	previousState := ""
	conditionSince := time.Time{}
	conditionActive := false

	for {
		if err := ctx.Err(); err != nil {
			return report, err
		}

		info, err := deps.ResolveTarget(lookupTarget)
		if err != nil {
			if deps.IsTargetGone(err) {
				now := deps.Now()
				report.State = StateGone
				report.RawState = StateGone
				report.Reason = ReasonPaneGone
				report.ConditionMet = options.Until == UntilGone && (!options.RequireTransition || resolved)
				if resolved {
					report.TransitionObserved = true
				}
				return finish(report, now), nil
			}
			return report, fmt.Errorf("resolve wait target %q: %w", lookupTarget, err)
		}
		if !resolved {
			report.Target = info.Target
			report.PaneID = info.PaneID
			lookupTarget = info.PaneID
			resolved = true
		} else if info.PaneID != report.PaneID {
			now := deps.Now()
			report.State = StateGone
			report.RawState = StateGone
			report.Reason = ReasonPaneGone
			report.ConditionMet = options.Until == UntilGone
			report.TransitionObserved = true
			return finish(report, now), nil
		}

		raw, err := deps.CapturePane(ctx, report.PaneID, captureLines)
		if err != nil {
			if ctxErr := ctx.Err(); ctxErr != nil {
				return report, ctxErr
			}
			if deps.IsTargetGone(err) {
				now := deps.Now()
				report.State = StateGone
				report.RawState = StateGone
				report.Reason = ReasonPaneGone
				report.ConditionMet = options.Until == UntilGone
				report.TransitionObserved = true
				return finish(report, now), nil
			}
			return report, fmt.Errorf("capture wait target %s: %w", report.PaneID, err)
		}

		classified := panestate.Classify(raw)
		state := normalizedState(classified)
		now := deps.Now()
		report.Samples++
		report.State = state
		report.RawState = classified.State
		report.LastLine = classified.LastLine
		report.Signals = append(report.Signals[:0], classified.Signals...)
		if previousState != "" && state != previousState {
			report.TransitionObserved = true
		}
		previousState = state

		if state == UntilNeedsHuman {
			report.Reason = ReasonNeedsHuman
			report.ConditionMet = options.Until == UntilNeedsHuman && (!options.RequireTransition || report.TransitionObserved)
			return finish(report, now), nil
		}

		matches := state == options.Until
		transitionSatisfied := !options.RequireTransition || report.TransitionObserved
		if matches && transitionSatisfied {
			if !conditionActive {
				conditionSince = now
				conditionActive = true
			}
			if options.Settle == 0 || now.Sub(conditionSince) >= options.Settle {
				report.Reason = ReasonConditionMet
				report.ConditionMet = true
				return finish(report, now), nil
			}
		} else {
			conditionActive = false
		}

		if !now.Before(deadline) {
			report.Reason = ReasonTimeout
			return finish(report, now), nil
		}

		delay := options.PollInterval
		if remaining := deadline.Sub(now); remaining < delay {
			delay = remaining
		}
		if conditionActive {
			if remaining := options.Settle - now.Sub(conditionSince); remaining < delay {
				delay = remaining
			}
		}
		if err := deps.Wait(ctx, delay); err != nil {
			return report, err
		}
	}
}

func validate(options Options, deps Dependencies) error {
	if options.Selector == "" {
		return errors.New("wait selector is required")
	}
	if !validConditions[options.Until] {
		return fmt.Errorf("unsupported wait condition %q", options.Until)
	}
	if options.Settle < 0 {
		return errors.New("wait settle cannot be negative")
	}
	if options.PollInterval <= 0 {
		return errors.New("wait poll interval must be positive")
	}
	if options.Timeout <= 0 {
		return errors.New("wait timeout must be positive")
	}
	if deps.ResolveTarget == nil || deps.CapturePane == nil || deps.IsTargetGone == nil || deps.Now == nil || deps.Wait == nil {
		return errors.New("wait dependencies are incomplete")
	}
	return nil
}

func normalizedState(result panestate.Result) string {
	if result.Asking || result.State == panestate.StateBlocked || result.State == panestate.StateWaitingPermission {
		return UntilNeedsHuman
	}
	switch result.State {
	case panestate.StateIdle, panestate.StateWaitingInput:
		return UntilInputReady
	case panestate.StateWorking:
		return UntilWorking
	default:
		return StateUnknown
	}
}

func finish(report Report, now time.Time) Report {
	report.FinishedAt = now
	report.Elapsed = now.Sub(report.StartedAt)
	return report
}

func waitContext(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
