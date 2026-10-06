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

	// DefaultNeedsHumanSettle is how long a blocker must stay on screen before
	// a wait reports needs_human. Claude PermissionRequest hooks can approve a
	// dialog within a second or two of it rendering; requiring persistence
	// keeps those transient dialogs from ending the wait while still reporting
	// a real, unanswered prompt promptly.
	DefaultNeedsHumanSettle = 5 * time.Second
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
	// NeedsHumanSettle is the continuous time a blocker must persist before
	// the wait ends with needs_human. Zero selects DefaultNeedsHumanSettle.
	NeedsHumanSettle time.Duration
	PollInterval     time.Duration
	Timeout          time.Duration
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
	ResolveTarget func(context.Context, string) (tmux.CapturePaneInfo, error)
	// CapturePane may preserve ANSI attributes for draft/suggestion detection.
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
		CapturePane:     tmux.CapturePaneANSIContext,
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
	blockerSince := time.Time{}
	blockerActive := false
	needsHumanSettle := options.NeedsHumanSettle
	if needsHumanSettle == 0 {
		needsHumanSettle = DefaultNeedsHumanSettle
	}

	// waitNext sleeps one poll interval, shortened so the next sample lands
	// on the overall deadline or on the end of a window started at since. It
	// reports done when the wait must return report with the given error.
	waitNext := func(since time.Time, window time.Duration) (bool, error) {
		now := deps.Now()
		delay := options.PollInterval
		if remaining := deadline.Sub(now); remaining < delay {
			delay = remaining
		}
		if window > 0 {
			if remaining := window - now.Sub(since); remaining < delay {
				delay = remaining
			}
		}
		if err := deps.Wait(waitCtx, delay); err != nil {
			if done, contextErr := finishContext(ctx, waitCtx, &report, deps.Now()); done {
				return true, contextErr
			}
			return true, err
		}
		return false, nil
	}

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

		classified := panestate.ClassifyANSI(raw, raw)
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
		// A blocker counts only once it has persisted for the confirmation
		// window: hooks can approve a permission dialog moments after it
		// renders. Until then it neither ends the wait nor counts as a
		// transition, and it interrupts any input-ready settling.
		if state == UntilNeedsHuman {
			conditionActive = false
			if !blockerActive {
				blockerSince = now
				blockerActive = true
			}
			if now.Sub(blockerSince) >= needsHumanSettle {
				if previousState != "" && state != previousState {
					report.TransitionObserved = true
				}
				report.Reason = ReasonNeedsHuman
				report.ConditionMet = options.Until == UntilNeedsHuman && (!options.RequireTransition || report.TransitionObserved)
				return finish(report, now), nil
			}
			if done, err := waitNext(blockerSince, needsHumanSettle); done {
				return report, err
			}
			continue
		}
		blockerActive = false
		if previousState != "" && state != previousState {
			report.TransitionObserved = true
		}
		previousState = state

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

		since, window := time.Time{}, time.Duration(0)
		if conditionActive {
			since, window = conditionSince, options.Settle
		}
		if done, err := waitNext(since, window); done {
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
	if options.NeedsHumanSettle < 0 {
		return errors.New("wait needs-human settle cannot be negative")
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
	if result.Asking || result.State == panestate.StateBlocked || result.State == panestate.StateWaitingPermission || result.State == panestate.StateWaitingQuota {
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
