package panewait

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/leolin310148/tmact/internal/tmux"
)

var errGone = errors.New("pane gone")

func TestWaitDistinguishesSuggestionFromDraft(t *testing.T) {
	for _, tc := range []struct {
		name, input, reason string
	}{
		{"suggestion", "\x1b[2mnext task\x1b[22m", ReasonConditionMet},
		{"draft", "\x1b[0mmy unsent task", ReasonTimeout},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fake := &fakeWait{captures: []string{"⏺ finished\n❯ " + tc.input + "\n⏵⏵ auto mode on · 1 shell · ← for agents"}}
			options := baseOptions(UntilInputReady)
			options.Settle = time.Second
			report, err := RunWithDependencies(context.Background(), options, fake.dependencies())
			if err != nil || report.Reason != tc.reason {
				t.Fatalf("report=%#v err=%v", report, err)
			}
		})
	}
}

type fakeWait struct {
	now      time.Time
	captures []string
	index    int
	resolves int
	goneAt   int
}

func (f *fakeWait) dependencies() Dependencies {
	return Dependencies{
		ResolveTarget: func(context.Context, string) (tmux.CapturePaneInfo, error) {
			f.resolves++
			if f.goneAt > 0 && f.resolves >= f.goneAt {
				return tmux.CapturePaneInfo{}, errGone
			}
			return tmux.CapturePaneInfo{Target: "work:0.0", PaneID: "%7"}, nil
		},
		CapturePane: func(context.Context, string, int) (string, error) {
			if len(f.captures) == 0 {
				return "", nil
			}
			index := f.index
			if index >= len(f.captures) {
				index = len(f.captures) - 1
			}
			f.index++
			return f.captures[index], nil
		},
		IsTargetGone: func(err error) bool { return errors.Is(err, errGone) },
		Now:          func() time.Time { return f.now },
		Wait: func(ctx context.Context, delay time.Duration) error {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
				f.now = f.now.Add(delay)
				return nil
			}
		},
		DeadlineContext: context.WithTimeout,
	}
}

func baseOptions(until string) Options {
	return Options{
		Selector:     "work",
		Until:        until,
		PollInterval: time.Second,
		Timeout:      10 * time.Second,
	}
}

func TestWaitRequiresTransitionAndSettlesCondition(t *testing.T) {
	fake := &fakeWait{captures: []string{
		"Working (esc to interrupt)\n",
		"❯ \n",
		"❯ \n",
		"❯ \n",
	}}
	options := baseOptions(UntilInputReady)
	options.RequireTransition = true
	options.Settle = 2 * time.Second

	report, err := RunWithDependencies(context.Background(), options, fake.dependencies())
	if err != nil {
		t.Fatal(err)
	}
	if !report.ConditionMet || report.Reason != ReasonConditionMet || report.State != UntilInputReady {
		t.Fatalf("report = %#v", report)
	}
	if !report.TransitionObserved || report.Samples != 4 || report.Elapsed != 3*time.Second {
		t.Fatalf("transition report = %#v", report)
	}
}

func TestWaitRequireTransitionDoesNotAcceptInitialCondition(t *testing.T) {
	fake := &fakeWait{captures: []string{"❯ \n", "Working (esc to interrupt)\n", "❯ \n"}}
	options := baseOptions(UntilInputReady)
	options.RequireTransition = true

	report, err := RunWithDependencies(context.Background(), options, fake.dependencies())
	if err != nil {
		t.Fatal(err)
	}
	if !report.ConditionMet || !report.TransitionObserved || report.Samples != 3 {
		t.Fatalf("report = %#v", report)
	}
}

func TestWaitAcceptsImmediateConditionWithoutTransition(t *testing.T) {
	fake := &fakeWait{captures: []string{"❯ \n"}}
	report, err := RunWithDependencies(context.Background(), baseOptions(UntilInputReady), fake.dependencies())
	if err != nil {
		t.Fatal(err)
	}
	if !report.ConditionMet || report.TransitionObserved || report.Samples != 1 {
		t.Fatalf("report = %#v", report)
	}
}

func TestWaitPermissionPromptPreemptsSettleAndTransition(t *testing.T) {
	fake := &fakeWait{captures: []string{"Allow directory access\nThis action may read or write paths outside your allowed directory list.\nDo you want to allow this?\n  1. Yes\n❯ 2. Yes, and add these directories to the allowed list\n  3. No (Esc)\n"}}
	options := baseOptions(UntilInputReady)
	options.RequireTransition = true
	options.Settle = 30 * time.Second
	options.NeedsHumanSettle = 2 * time.Second

	report, err := RunWithDependencies(context.Background(), options, fake.dependencies())
	if err != nil {
		t.Fatal(err)
	}
	if report.ConditionMet || report.Reason != ReasonNeedsHuman || report.State != UntilNeedsHuman || report.Elapsed != 2*time.Second {
		t.Fatalf("report = %#v", report)
	}
}

const (
	claudeWorkingFrame = "⏺ Bash(go test ./...)\n  ⎿  ok\n\n✻ Crystallizing… (1m 7s · ↓ 5.0k tokens)\n\n────────\n\x1b[39m❯ \x1b[0m\n────────\n  ⏵⏵ accept edits on · ← for agents\n"
	claudeTipFrame     = "⏺ Bash(go test ./...)\n  ⎿  ok\n\n✻ Crystallizing… (1m 9s · ↓ 5.0k tokens)\n  ⎿  Tip: Use ctrl+v to paste images from your clipboard\n\n────────\n\x1b[39m❯ \x1b[0m\n────────\n  ⏵⏵ accept edits on · ← for agents\n"
	claudeHookedFrame  = "⏺ Bash(python3 check.py)\n  ⎿  ok\n  ⎿  Allowed by PermissionRequest hook\n\n✻ Crystallizing… (1m 12s · ↓ 5.1k tokens)\n\n────────\n\x1b[39m❯ \x1b[0m\n────────\n  ⏵⏵ accept edits on · ← for agents\n"
	claudeIdleFrame    = "⏺ Done.\n\n✻ Worked for 2m 3s\n\n────────\n\x1b[39m❯ \x1b[0m\n────────\n  ⏵⏵ accept edits on · ← for agents\n"
	claudeDialogFrame  = "────────\n Bash command\n python3 check.py\n This command requires approval\n Do you want to proceed?\n ❯ 1. Yes\n   2. Yes, and don’t ask again for: python3 *\n   3. No\n Esc to cancel · Tab to amend\n"
)

func TestWaitDoesNotTreatTipUnderSpinnerAsInputReady(t *testing.T) {
	captures := []string{claudeWorkingFrame}
	for range 60 {
		captures = append(captures, claudeTipFrame)
	}
	captures = append(captures, claudeIdleFrame)
	fake := &fakeWait{captures: captures}
	options := baseOptions(UntilInputReady)
	options.RequireTransition = true
	options.Settle = 30 * time.Second
	options.Timeout = 2 * time.Minute

	report, err := RunWithDependencies(context.Background(), options, fake.dependencies())
	if err != nil {
		t.Fatal(err)
	}
	if !report.ConditionMet || report.Elapsed < 90*time.Second {
		t.Fatalf("input-ready accepted while the spinner was still live: %#v", report)
	}
}

func TestWaitIgnoresPermissionDialogApprovedByHook(t *testing.T) {
	fake := &fakeWait{captures: []string{
		claudeWorkingFrame,
		claudeDialogFrame,
		claudeDialogFrame,
		claudeHookedFrame,
		claudeHookedFrame,
		claudeIdleFrame,
	}}
	options := baseOptions(UntilInputReady)
	options.RequireTransition = true
	options.Settle = 2 * time.Second

	report, err := RunWithDependencies(context.Background(), options, fake.dependencies())
	if err != nil {
		t.Fatal(err)
	}
	if !report.ConditionMet || report.Reason != ReasonConditionMet {
		t.Fatalf("transient hook-approved dialog ended the wait: %#v", report)
	}
}

func TestWaitReportsPersistentPermissionDialogAfterConfirmation(t *testing.T) {
	fake := &fakeWait{captures: []string{claudeWorkingFrame, claudeDialogFrame}}
	options := baseOptions(UntilInputReady)
	options.RequireTransition = true
	options.Settle = 2 * time.Minute
	options.Timeout = 5 * time.Minute

	report, err := RunWithDependencies(context.Background(), options, fake.dependencies())
	if err != nil {
		t.Fatal(err)
	}
	if report.ConditionMet || report.Reason != ReasonNeedsHuman || report.State != UntilNeedsHuman {
		t.Fatalf("report = %#v", report)
	}
	// One working sample, then the dialog must persist for the default
	// confirmation window — not for the much longer input-ready settle.
	if want := time.Second + DefaultNeedsHumanSettle; report.Elapsed != want {
		t.Fatalf("elapsed = %s, want %s", report.Elapsed, want)
	}
}

func TestWaitNeedsHumanSettleIsConfigurable(t *testing.T) {
	fake := &fakeWait{captures: []string{claudeDialogFrame}}
	options := baseOptions(UntilNeedsHuman)
	options.NeedsHumanSettle = 2 * time.Second

	report, err := RunWithDependencies(context.Background(), options, fake.dependencies())
	if err != nil {
		t.Fatal(err)
	}
	if !report.ConditionMet || report.Reason != ReasonNeedsHuman || report.Elapsed != 2*time.Second {
		t.Fatalf("report = %#v", report)
	}
}

func TestWaitCanRequestNeedsHuman(t *testing.T) {
	fake := &fakeWait{captures: []string{"permission denied\n"}}
	report, err := RunWithDependencies(context.Background(), baseOptions(UntilNeedsHuman), fake.dependencies())
	if err != nil {
		t.Fatal(err)
	}
	if !report.ConditionMet || report.Reason != ReasonNeedsHuman {
		t.Fatalf("report = %#v", report)
	}
}

func TestWaitTimesOut(t *testing.T) {
	fake := &fakeWait{captures: []string{"plain output\n"}}
	options := baseOptions(UntilWorking)
	options.Timeout = 2 * time.Second

	report, err := RunWithDependencies(context.Background(), options, fake.dependencies())
	if err != nil {
		t.Fatal(err)
	}
	if report.ConditionMet || report.Reason != ReasonTimeout || report.Samples != 3 || report.Elapsed != 2*time.Second {
		t.Fatalf("report = %#v", report)
	}
}

func TestWaitReportsPaneGone(t *testing.T) {
	fake := &fakeWait{captures: []string{"plain output\n"}, goneAt: 2}
	report, err := RunWithDependencies(context.Background(), baseOptions(UntilGone), fake.dependencies())
	if err != nil {
		t.Fatal(err)
	}
	if !report.ConditionMet || report.Reason != ReasonPaneGone || report.State != StateGone || !report.TransitionObserved {
		t.Fatalf("report = %#v", report)
	}
}

func TestWaitInitialMissingPaneIsGone(t *testing.T) {
	fake := &fakeWait{goneAt: 1}
	report, err := RunWithDependencies(context.Background(), baseOptions(UntilInputReady), fake.dependencies())
	if err != nil {
		t.Fatal(err)
	}
	if report.ConditionMet || report.Reason != ReasonPaneGone || report.TransitionObserved {
		t.Fatalf("report = %#v", report)
	}
}

func TestWaitInitialGoneDoesNotSatisfyRequiredTransition(t *testing.T) {
	fake := &fakeWait{goneAt: 1}
	options := baseOptions(UntilGone)
	options.RequireTransition = true
	report, err := RunWithDependencies(context.Background(), options, fake.dependencies())
	if err != nil {
		t.Fatal(err)
	}
	if report.ConditionMet || report.Reason != ReasonPaneGone || report.TransitionObserved {
		t.Fatalf("report = %#v", report)
	}
}

func TestWaitInitialNeedsHumanDoesNotSatisfyRequiredTransition(t *testing.T) {
	fake := &fakeWait{captures: []string{"permission denied\n"}}
	options := baseOptions(UntilNeedsHuman)
	options.RequireTransition = true
	report, err := RunWithDependencies(context.Background(), options, fake.dependencies())
	if err != nil {
		t.Fatal(err)
	}
	if report.ConditionMet || report.Reason != ReasonNeedsHuman || report.TransitionObserved {
		t.Fatalf("report = %#v", report)
	}
}

func TestWaitCancellationInterruptsPolling(t *testing.T) {
	fake := &fakeWait{captures: []string{"plain output\n"}}
	deps := fake.dependencies()
	ctx, cancel := context.WithCancel(context.Background())
	deps.Wait = func(ctx context.Context, _ time.Duration) error {
		cancel()
		return ctx.Err()
	}

	_, err := RunWithDependencies(ctx, baseOptions(UntilWorking), deps)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v", err)
	}
}

func TestWaitDeadlineInterruptsBlockingDependencies(t *testing.T) {
	tests := []struct {
		name  string
		block string
	}{
		{name: "resolve", block: "resolve"},
		{name: "capture", block: "capture"},
		{name: "poll wait", block: "wait"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fake := &fakeWait{captures: []string{"plain output\n"}}
			deps := fake.dependencies()
			deadline := newManualDeadline()
			deps.DeadlineContext = deadline.Context
			started := make(chan struct{})

			if tt.block == "resolve" {
				deps.ResolveTarget = func(ctx context.Context, _ string) (tmux.CapturePaneInfo, error) {
					close(started)
					<-ctx.Done()
					return tmux.CapturePaneInfo{}, ctx.Err()
				}
			}
			if tt.block == "capture" {
				deps.CapturePane = func(ctx context.Context, _ string, _ int) (string, error) {
					close(started)
					<-ctx.Done()
					return "", ctx.Err()
				}
			}
			if tt.block == "wait" {
				deps.Wait = func(ctx context.Context, _ time.Duration) error {
					close(started)
					<-ctx.Done()
					return ctx.Err()
				}
			}

			type result struct {
				report Report
				err    error
			}
			resultCh := make(chan result, 1)
			go func() {
				report, err := RunWithDependencies(context.Background(), baseOptions(UntilWorking), deps)
				resultCh <- result{report: report, err: err}
			}()

			<-deadline.Armed
			<-started
			deadline.Expire()
			got := <-resultCh
			if got.err != nil {
				t.Fatal(got.err)
			}
			if got.report.Reason != ReasonTimeout || got.report.ConditionMet {
				t.Fatalf("report = %#v", got.report)
			}
		})
	}
}

func TestWaitOperatorCancellationDuringCaptureRemainsCancellation(t *testing.T) {
	fake := &fakeWait{}
	deps := fake.dependencies()
	started := make(chan struct{})
	deps.CapturePane = func(ctx context.Context, _ string, _ int) (string, error) {
		close(started)
		<-ctx.Done()
		return "", ctx.Err()
	}
	ctx, cancel := context.WithCancel(context.Background())
	type result struct {
		report Report
		err    error
	}
	resultCh := make(chan result, 1)
	go func() {
		report, err := RunWithDependencies(ctx, baseOptions(UntilWorking), deps)
		resultCh <- result{report: report, err: err}
	}()
	<-started
	cancel()
	got := <-resultCh
	if !errors.Is(got.err, context.Canceled) {
		t.Fatalf("err = %v", got.err)
	}
	if got.report.Reason == ReasonTimeout {
		t.Fatalf("operator cancellation reported as timeout: %#v", got.report)
	}
}

func TestWaitParentDeadlineReturnsStructuredTimeout(t *testing.T) {
	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancel()
	report, err := RunWithDependencies(ctx, baseOptions(UntilWorking), (&fakeWait{}).dependencies())
	if err != nil {
		t.Fatal(err)
	}
	if report.Reason != ReasonTimeout || report.ConditionMet {
		t.Fatalf("report = %#v", report)
	}
}

type manualDeadline struct {
	Armed  chan struct{}
	cancel context.CancelFunc
}

func newManualDeadline() *manualDeadline {
	return &manualDeadline{Armed: make(chan struct{})}
}

func (d *manualDeadline) Context(parent context.Context, _ time.Duration) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(parent)
	d.cancel = cancel
	close(d.Armed)
	return ctx, cancel
}

func (d *manualDeadline) Expire() {
	d.cancel()
}

func TestWaitMatchesWorking(t *testing.T) {
	fake := &fakeWait{captures: []string{"Thinking (esc to interrupt)\n"}}
	report, err := RunWithDependencies(context.Background(), baseOptions(UntilWorking), fake.dependencies())
	if err != nil {
		t.Fatal(err)
	}
	if !report.ConditionMet || report.Reason != ReasonConditionMet || report.State != UntilWorking {
		t.Fatalf("report = %#v", report)
	}
}

func TestWaitValidatesOptionsAndDependencies(t *testing.T) {
	valid := baseOptions(UntilWorking)
	deps := (&fakeWait{}).dependencies()
	for _, mutate := range []func(*Options){
		func(options *Options) { options.Selector = "" },
		func(options *Options) { options.Until = "done" },
		func(options *Options) { options.Settle = -time.Second },
		func(options *Options) { options.NeedsHumanSettle = -time.Second },
		func(options *Options) { options.PollInterval = 0 },
		func(options *Options) { options.Timeout = 0 },
	} {
		options := valid
		mutate(&options)
		if _, err := RunWithDependencies(context.Background(), options, deps); err == nil {
			t.Fatalf("expected validation error for %#v", options)
		}
	}

	deps.CapturePane = nil
	if _, err := RunWithDependencies(context.Background(), valid, deps); err == nil {
		t.Fatal("expected incomplete dependency error")
	}
}
