package dispatch_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/leolin310148/tmact/internal/dispatch"
	"github.com/leolin310148/tmact/internal/panewait"
	"github.com/leolin310148/tmact/internal/tmux"
)

func TestDispatchWaitTerminalCases(t *testing.T) {
	tests := []struct {
		name, baseline, state, reason, wantErr, baselineState, status string
		met, result                                                   bool
	}{
		{"immediate idle", "Claude Code\n❯ do the thing\nDone\n❯ ", panewait.UntilInputReady, panewait.ReasonConditionMet, "", panewait.UntilInputReady, dispatch.StatusOK, true, true},
		{"working to idle", "Claude Code\nWorking (esc to interrupt)", panewait.UntilInputReady, panewait.ReasonConditionMet, "", panewait.UntilWorking, dispatch.StatusOK, true, true},
		{"permission", "Waiting for approval\nAllow this command?\n  1. Yes\n❯ 2. No", panewait.UntilNeedsHuman, panewait.ReasonNeedsHuman, panewait.ReasonNeedsHuman, panewait.UntilNeedsHuman, dispatch.StatusFailed, false, true},
		{"timeout", "Claude Code\nWorking (esc to interrupt)", panewait.UntilWorking, panewait.ReasonTimeout, panewait.ReasonTimeout, panewait.UntilWorking, dispatch.StatusFailed, false, true},
		{"disappeared pane", "Claude Code\nWorking (esc to interrupt)", panewait.StateGone, panewait.ReasonPaneGone, panewait.ReasonPaneGone, panewait.UntilWorking, dispatch.StatusFailed, false, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec, deps := baseDeps()
			deps.ListSessionPanes = func(string) ([]tmux.Pane, error) { return []tmux.Pane{claudePane()}, nil }
			sawResultLines := false
			deps.CapturePane = func(_ string, lines int) (string, error) {
				if lines == 42 {
					sawResultLines = true
				}
				if len(rec.pastes) < 2 {
					return "Claude Code\n❯ ", nil
				}
				return tt.baseline, nil
			}
			waitCalls := 0
			deps.WaitPane = func(ctx context.Context, options panewait.Options) (panewait.Report, error) {
				waitCalls++
				if ctx == nil || options.Selector != "%1" || options.Until != panewait.UntilInputReady || options.RequireTransition || options.Timeout != 2*time.Minute || options.Settle != 2*time.Second {
					t.Fatalf("wait options = %#v", options)
				}
				return panewait.Report{Target: "work:0.0", PaneID: "%1", State: tt.state, RawState: tt.state, Reason: tt.reason, ConditionMet: tt.met, Samples: 2, Elapsed: 3 * time.Second}, nil
			}

			opts := baseOpts()
			opts.Execute, opts.Wait = true, true
			opts.WaitTimeout, opts.WaitSettle, opts.ResultLines = 2*time.Minute, 2*time.Second, 42
			report, err := dispatch.RunWithDeps(opts, deps)
			if tt.wantErr == "" && err != nil {
				t.Fatal(err)
			}
			if tt.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tt.wantErr)) {
				t.Fatalf("err = %v, want %q", err, tt.wantErr)
			}
			if waitCalls != 1 || report.Wait == nil || report.Wait.Baseline == nil || !report.Wait.Baseline.Accepted || report.Wait.Baseline.Evidence == "" {
				t.Fatalf("wait report = %#v, calls = %d", report.Wait, waitCalls)
			}
			if report.Wait.Baseline.State != tt.baselineState || report.Wait.Status != tt.status || report.Wait.Outcome.ConditionMet != tt.met {
				t.Fatalf("wait report = %#v", report.Wait)
			}
			if (report.Result != nil) != tt.result || stepStatus(t, report, "wait-result") != tt.status {
				t.Fatalf("result = %#v steps = %#v", report.Result, report.Steps)
			}
			if sawResultLines != tt.result {
				t.Fatalf("result capture = %t, want %t", sawResultLines, tt.result)
			}
			if tt.name == "permission" && len(rec.keys) != 0 {
				t.Fatalf("permission prompt received keys: %#v", rec.keys)
			}
		})
	}
}

func TestDispatchWaitDryRunPlansWithoutWaiting(t *testing.T) {
	_, deps := baseDeps()
	deps.WaitPane = func(context.Context, panewait.Options) (panewait.Report, error) {
		t.Fatal("dry-run must not wait")
		return panewait.Report{}, nil
	}
	opts := baseOpts()
	opts.Wait, opts.WaitTimeout, opts.ResultLines = true, time.Minute, 20
	report, err := dispatch.RunWithDeps(opts, deps)
	if err != nil {
		t.Fatal(err)
	}
	if report.Wait == nil || report.Wait.Status != dispatch.StatusPlanned || report.Wait.Outcome != nil || report.Result != nil {
		t.Fatalf("report = %#v", report)
	}
}
