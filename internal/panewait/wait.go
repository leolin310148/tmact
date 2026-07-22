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
	ResolveTarget   func(context.Context, string) (tmux.CapturePaneInfo, error)
	CapturePane     func(context.Context, string, int) (string, error)
	IsTargetGone    func(error) bool
	Now             func() time.Time
	Wait            func(context.Context, time.Duration) error
	DeadlineContext func(context.Context, time.Duration) (context.Context, context.CancelFunc)
}

// DefaultDependencies wires the wait to read-only tmux helpers.
func DefaultDependencies() Dependencies {
	return Dependencies{
		ResolveTarget:   tmux.CapturePaneInfoForTargetContext,
		CapturePane:     tmux.CapturePaneContext,
		IsTargetGone:    tmux.IsTargetGoneError,
		Now:             time.Now,
		Wait:            waitContext,
		DeadlineContext: context.WithTimeout,
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
	waitCtx, cancel := deps.DeadlineContext(ctx, options.Timeout)
	defer cancel()
	lookupTarget := options.Selector
	resolved := false
	previousState := ""
	conditionSince := time.Time{}
	conditionActive := false

	for {
		if done, err := finishContext(ctx, waitCtx, &report, deps.Now()); done {
			return report, err
		}

		info, err := deps.ResolveTarget(waitCtx, lookupTarget)
		if err != nil {
			if done, contextErr := finishContext(ctx, waitCtx, &report, deps.Now()); done {
				return report, contextErr
			}
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
		if done, contextErr := finishContext(ctx, waitCtx, &report, deps.Now()); done {
			return report, contextErr
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

		raw, err := deps.CapturePane(waitCtx, report.PaneID, captureLines)
		if err != nil {
			if done, contextErr := finishContext(ctx, waitCtx, &report, deps.Now()); done {
				return report, contextErr
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
		if done, contextErr := finishContext(ctx, waitCtx, &report, deps.Now()); done {
			return report, contextErr
		}

		classified := panestate.Classify(raw)
		state := NormalizeState(classified)
		now := deps.Now()
		report.Samples++
		report.State = state
		report.RawState = classified.State
		report.LastLine = classified.LastLine
		report.Signals = append(report.Signals[:0], classified.Signals...)
		if !now.Before(deadline) {
			report.Reason = ReasonTimeout
			return finish(report, now), nil
		}
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

		delay := options.PollInterval
		if remaining := deadline.Sub(now); remaining < delay {
			delay = remaining
		}
		if conditionActive {
			if remaining := options.Settle - now.Sub(conditionSince); remaining < delay {
				delay = remaining
			}
		}
		if err := deps.Wait(waitCtx, delay); err != nil {
			if done, contextErr := finishContext(ctx, waitCtx, &report, deps.Now()); done {
				return report, contextErr
			}
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
	if deps.ResolveTarget == nil || deps.CapturePane == nil || deps.IsTargetGone == nil || deps.Now == nil || deps.Wait == nil || deps.DeadlineContext == nil {
		return errors.New("wait dependencies are incomplete")
	}
	return nil
}

func finishContext(parent, waitCtx context.Context, report *Report, now time.Time) (bool, error) {
	if err := parent.Err(); err != nil {
		if !errors.Is(err, context.DeadlineExceeded) {
			return true, err
		}
		report.Reason = ReasonTimeout
		*report = finish(*report, now)
		return true, nil
	}
	if waitCtx.Err() != nil {
		report.Reason = ReasonTimeout
		*report = finish(*report, now)
		return true, nil
	}
	return false, nil
}

// NormalizeState maps pane classification onto the public wait conditions.
// Callers that already captured a baseline can use the same state vocabulary
// as Run without duplicating prompt/blocker rules.
func NormalizeState(result panestate.Result) string {
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
