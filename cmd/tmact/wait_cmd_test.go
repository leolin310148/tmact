package main

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/leolin310148/tmact/internal/panewait"
)

func TestWaitJSONIncludesTerminalReasonAndOptions(t *testing.T) {
	resetCLIHooks := stubCLIHooks(t)
	defer resetCLIHooks()

	paneWaitRun = func(_ context.Context, options panewait.Options) (panewait.Report, error) {
		if options.Selector != "%7" || options.Until != panewait.UntilInputReady || !options.RequireTransition {
			t.Fatalf("options = %#v", options)
		}
		if options.Settle != 2*time.Second || options.PollInterval != 250*time.Millisecond || options.Timeout != time.Minute {
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

	out, err := captureRun(t, "-t", "%7", "wait", "--until", "input-ready", "--require-transition", "--settle", "2s", "--poll-interval", "250ms", "--timeout", "1m", "--json")
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
	if report.Elapsed != "3s" || report.Settle != "2s" || report.PollInterval != "250ms" || report.Timeout != "1m0s" {
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

func TestWaitValidation(t *testing.T) {
	tests := [][]string{
		{"wait", "--until", "working"},
		{"wait", "--target", "%7"},
		{"wait", "--target", "work", "--until", "working"},
		{"wait", "--target", "work:0", "--until", "working"},
		{"wait", "--target", "peer-a@%7", "--until", "working"},
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
