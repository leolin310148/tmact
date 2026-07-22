package panewait

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/leolin310148/tmact/internal/tmux"
)

var errGone = errors.New("pane gone")

type fakeWait struct {
	now      time.Time
	captures []string
	index    int
	resolves int
	goneAt   int
}

func (f *fakeWait) dependencies() Dependencies {
	return Dependencies{
		ResolveTarget: func(string) (tmux.CapturePaneInfo, error) {
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

func TestWaitPermissionPromptReturnsNeedsHumanImmediately(t *testing.T) {
	fake := &fakeWait{captures: []string{"Allow directory access\nThis action may read or write paths outside your allowed directory list.\nDo you want to allow this?\n  1. Yes\n❯ 2. Yes, and add these directories to the allowed list\n  3. No (Esc)\n"}}
	options := baseOptions(UntilInputReady)
	options.RequireTransition = true
	options.Settle = 5 * time.Second

	report, err := RunWithDependencies(context.Background(), options, fake.dependencies())
	if err != nil {
		t.Fatal(err)
	}
	if report.ConditionMet || report.Reason != ReasonNeedsHuman || report.State != UntilNeedsHuman || report.Samples != 1 {
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
