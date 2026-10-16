package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/leolin310148/tmact/internal/panewait"
	"github.com/leolin310148/tmact/internal/tmux"
	"github.com/leolin310148/tmact/internal/web"
)

func TestWaitJSONIncludesTerminalReasonAndOptions(t *testing.T) {
	resetCLIHooks := stubCLIHooks(t)
	defer resetCLIHooks()

	paneWaitRun = func(_ context.Context, options panewait.Options) (panewait.Report, error) {
		if options.Selector != "%7" || options.Until != panewait.UntilInputReady || !options.RequireTransition {
			t.Fatalf("options = %#v", options)
		}
		if options.Settle != 2*time.Second || options.NeedsHumanSettle != 3*time.Second || options.PollInterval != 250*time.Millisecond || options.Timeout != time.Minute {
			t.Fatalf("durations = %#v", options)
		}
		started := time.Date(2026, 7, 22, 12, 0, 0, 0, time.UTC)
		return panewait.Report{
			Selector:           options.Selector,
			Target:             "work:0.0",
			PaneID:             "%7",
			Until:              options.Until,
			State:              panewait.UntilInputReady,
			RawState:           "waiting_input",
			Reason:             panewait.ReasonConditionMet,
			ConditionMet:       true,
			TransitionObserved: true,
			Samples:            4,
			Signals:            []string{"waiting_input_text"},
			StartedAt:          started,
			FinishedAt:         started.Add(3 * time.Second),
			Elapsed:            3 * time.Second,
		}, nil
	}

	out, err := captureRun(t, "-t", "%7", "wait", "--until", "input-ready", "--require-transition", "--settle", "2s", "--needs-human-settle", "3s", "--poll-interval", "250ms", "--timeout", "1m", "--json")
	if err != nil {
		t.Fatal(err)
	}
	var report waitCommandReport
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatal(err)
	}
	if report.Target != "work:0.0" || report.PaneID != "%7" || report.Reason != panewait.ReasonConditionMet || !report.ConditionMet {
		t.Fatalf("report = %#v", report)
	}
	if report.Elapsed != "3s" || report.Settle != "2s" || report.NeedsHumanSettle != "3s" || report.PollInterval != "250ms" || report.Timeout != "1m0s" {
		t.Fatalf("duration report = %#v", report)
	}
}

func TestWaitAcceptsExactSessionAndPrintsText(t *testing.T) {
	resetCLIHooks := stubCLIHooks(t)
	defer resetCLIHooks()

	paneWaitRun = func(_ context.Context, options panewait.Options) (panewait.Report, error) {
		if options.Selector != "work" || options.Until != panewait.UntilWorking {
			t.Fatalf("options = %#v", options)
		}
		return panewait.Report{
			Selector:     options.Selector,
			Target:       "work:0.0",
			PaneID:       "%7",
			Until:        options.Until,
			State:        panewait.UntilWorking,
			RawState:     "working",
			Reason:       panewait.ReasonConditionMet,
			ConditionMet: true,
		}, nil
	}

	out, err := captureRun(t, "wait", "--session", "work", "--until", "working")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "reason=condition_met") || !strings.Contains(out, "target=work:0.0") {
		t.Fatalf("output = %q", out)
	}
}

func TestWaitNumberedTargetDefersLiveResolutionToBoundedWait(t *testing.T) {
	t.Chdir(t.TempDir())
	resetCLIHooks := stubCLIHooks(t)
	defer resetCLIHooks()

	tmactNow = func() time.Time { return time.Date(2026, 7, 22, 12, 0, 0, 0, time.UTC) }
	if err := writeTargetCache(targetCache{
		GeneratedAt: tmactNow(),
		Panes:       []listPaneRow{{Index: 0, Target: "%42"}},
	}); err != nil {
		t.Fatal(err)
	}
	listTargetTmuxPanes = func(string) ([]tmux.Pane, error) {
		t.Fatal("wait must not issue an unbounded cache-validation tmux call")
		return nil, nil
	}
	paneWaitRun = func(_ context.Context, options panewait.Options) (panewait.Report, error) {
		if options.Selector != "%42" {
			t.Fatalf("selector = %q", options.Selector)
		}
		return panewait.Report{Selector: options.Selector, Target: "work:0.0", PaneID: "%42", Until: options.Until, State: panewait.UntilWorking, RawState: "working", Reason: panewait.ReasonConditionMet, ConditionMet: true}, nil
	}

	if _, err := captureRun(t, "-t", "0", "wait", "--until", "working", "--timeout", "1s"); err != nil {
		t.Fatal(err)
	}
}

func TestWaitReturnsReportAndErrorForTerminalBlocker(t *testing.T) {
	resetCLIHooks := stubCLIHooks(t)
	defer resetCLIHooks()

	paneWaitRun = func(_ context.Context, options panewait.Options) (panewait.Report, error) {
		return panewait.Report{
			Selector: options.Selector,
			Target:   "work:0.0",
			PaneID:   "%7",
			Until:    options.Until,
			State:    panewait.UntilNeedsHuman,
			RawState: "waiting_permission",
			Reason:   panewait.ReasonNeedsHuman,
		}, nil
	}

	out, err := captureRun(t, "wait", "--target", "%7", "--until", "input-ready", "--json")
	if err == nil || !strings.Contains(err.Error(), "needs_human") {
		t.Fatalf("err = %v", err)
	}
	var report waitCommandReport
	if jsonErr := json.Unmarshal([]byte(out), &report); jsonErr != nil {
		t.Fatal(jsonErr)
	}
	if report.Reason != panewait.ReasonNeedsHuman || report.ConditionMet {
		t.Fatalf("report = %#v", report)
	}
}

func TestWaitTimeoutPrintsStructuredReport(t *testing.T) {
	resetCLIHooks := stubCLIHooks(t)
	defer resetCLIHooks()

	paneWaitRun = func(_ context.Context, options panewait.Options) (panewait.Report, error) {
		now := time.Now()
		return panewait.Report{
			Selector: options.Selector, Target: options.Selector, Until: options.Until,
			State: panewait.StateUnknown, RawState: panewait.StateUnknown,
			Reason: panewait.ReasonTimeout, StartedAt: now, FinishedAt: now,
		}, nil
	}

	out, err := captureRun(t, "wait", "--target", "%7", "--until", "working", "--timeout", "1ms", "--json")
	if err == nil || !strings.Contains(err.Error(), panewait.ReasonTimeout) {
		t.Fatalf("err = %v", err)
	}
	var report waitCommandReport
	if jsonErr := json.Unmarshal([]byte(out), &report); jsonErr != nil {
		t.Fatal(jsonErr)
	}
	if report.Reason != panewait.ReasonTimeout || report.ConditionMet {
		t.Fatalf("report = %#v", report)
	}
}

func TestWaitValidation(t *testing.T) {
	tests := [][]string{
		{"wait", "--until", "working"},
		{"wait", "--target", "%7"},
		{"wait", "--target", "work", "--until", "working"},
		{"wait", "--target", "work:0", "--until", "working"},
		{"wait", "--session", "peer-a@work", "--until", "working"},
		{"wait", "--session", "work:0.0", "--until", "working"},
		{"wait", "--target", "%7", "--session", "work", "--until", "working"},
		{"-t", "%7", "wait", "--target", "%8", "--until", "working"},
		{"wait", "--target", "%7", "--until", "done"},
		{"wait", "--target", "%7", "--until", "working", "--settle", "-1s"},
		{"wait", "--target", "%7", "--until", "working", "--poll-interval", "0"},
		{"wait", "--target", "%7", "--until", "working", "--timeout", "0"},
		{"wait", "--target", "%7", "--until", "working", "extra"},
	}
	for _, args := range tests {
		if _, err := captureRun(t, args...); err == nil {
			t.Errorf("expected validation error for %v", args)
		}
	}
}

func TestWaitPeerTargetObservesThePeerThroughItsCaptureEndpoint(t *testing.T) {
	defer stubCLIHooks(t)()
	paneWaitRun = func(context.Context, panewait.Options) (panewait.Report, error) {
		t.Fatal("peer wait must not use the local tmux wait")
		return panewait.Report{}, nil
	}
	fetchPeerPaneCapture = func(_ context.Context, peerName, baseURL, pane string, lines int, modes ...string) (web.PaneCapture, error) {
		if peerName != "mini" || pane != "%7" || strings.Join(modes, ",") != "ansi" {
			t.Fatalf("fetch got (%q, %q, %v)", peerName, pane, modes)
		}
		return web.PaneCapture{Target: "work:0.0", PaneID: "%7", Captures: map[string]string{"ansi": "remote screen"}}, nil
	}
	paneWaitRunWithDependencies = func(ctx context.Context, options panewait.Options, deps panewait.Dependencies) (panewait.Report, error) {
		if options.Selector != "mini@%7" {
			t.Fatalf("selector = %q", options.Selector)
		}
		info, err := deps.ResolveTarget(ctx, options.Selector)
		if err != nil {
			t.Fatal(err)
		}
		raw, err := deps.CapturePane(ctx, info.PaneID, 200)
		if err != nil {
			t.Fatal(err)
		}
		if info.Target != "mini@work:0.0" || info.PaneID != "mini@%7" || raw != "remote screen" {
			t.Fatalf("info = %#v raw = %q", info, raw)
		}
		return panewait.Report{Target: info.Target, PaneID: info.PaneID, Reason: panewait.ReasonConditionMet, ConditionMet: true}, nil
	}

	out, err := captureRun(t, "wait", "--target", "mini@%7", "--until", "input-ready", "--config", writePeerConfig(t), "--json")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, `"target": "mini@work:0.0"`) {
		t.Fatalf("output = %s", out)
	}
}

func TestPeerWaitDependenciesReportGonePaneAndRetryTransientErrors(t *testing.T) {
	defer stubCLIHooks(t)()
	calls := 0
	fetchPeerPaneCapture = func(context.Context, string, string, string, int, ...string) (web.PaneCapture, error) {
		calls++
		if calls < 3 {
			return web.PaneCapture{}, errors.New("connection reset")
		}
		return web.PaneCapture{}, fmt.Errorf("%w: mini@%%7", web.ErrPeerPaneGone)
	}
	waits := 0
	deps := peerWaitDependenciesWithWait("mini", "http://mini.example:7890", func(context.Context, time.Duration) error {
		waits++
		return nil
	})
	_, err := deps.ResolveTarget(context.Background(), "mini@%7")
	if !deps.IsTargetGone(err) {
		t.Fatalf("err = %v, want gone", err)
	}
	if calls != 3 || waits != 2 {
		t.Fatalf("calls = %d waits = %d, want 3 and 2", calls, waits)
	}

	calls = -10
	if _, err := deps.ResolveTarget(context.Background(), "mini@%7"); err == nil || deps.IsTargetGone(err) || !strings.Contains(err.Error(), "after 6 attempts") {
		t.Fatalf("err = %v, want transient failure after retries", err)
	}
}
