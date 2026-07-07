package statusd

import (
	"context"
	"testing"
	"time"

	"github.com/leolin310148/tmact/internal/panestate"
	"github.com/leolin310148/tmact/internal/shellhook"
	"github.com/leolin310148/tmact/internal/tmux"
)

func hookMergeSnapshot(pane PaneStatus) Snapshot {
	now := time.Date(2026, 7, 7, 10, 0, 0, 0, time.UTC)
	snapshot := Snapshot{
		Version:   1,
		Timestamp: now,
		Sessions:  map[string]SessionStatus{},
		Panes:     map[string]PaneStatus{pane.Target: pane},
	}
	snapshot.Sessions = buildSessions(snapshot.Panes, now)
	snapshot.Summary = summarize(snapshot)
	return snapshot
}

func activeHookState(paneID string) shellhook.PaneState {
	return activeHookStateSince(paneID, time.Date(2026, 7, 7, 10, 0, 0, 0, time.UTC).Add(-2*time.Second))
}

func activeHookStateSince(paneID string, startedAt time.Time) shellhook.PaneState {
	return shellhook.PaneState{
		PaneID: paneID,
		Active: &shellhook.CommandRecord{CommandID: "c1", Command: "make build", StartedAt: startedAt},
	}
}

func completedHookState(paneID string) shellhook.PaneState {
	code := 0
	return shellhook.PaneState{
		PaneID:    paneID,
		Completed: &shellhook.CommandRecord{CommandID: "c1", Command: "make build", Matched: true, ExitCode: &code},
	}
}

func TestApplyShellHooksActiveMarksWorking(t *testing.T) {
	snapshot := hookMergeSnapshot(PaneStatus{
		Target: "main:0.0", PaneID: "%1", Session: "main",
		State: panestate.StateIdle, Idle: true, InputReady: true,
	})
	merged := ApplyShellHooks(snapshot, map[string]shellhook.PaneState{"%1": activeHookState("%1")})

	pane := merged.Panes["main:0.0"]
	if !pane.Running || pane.Idle || pane.InputReady {
		t.Fatalf("pane = %+v, want running and not idle/input-ready", pane)
	}
	if pane.State != panestate.StateWorking {
		t.Fatalf("state = %q, want working", pane.State)
	}
	if !hasSignal(pane.Signals, SignalShellHook) || !hasSignal(pane.Signals, SignalShellHookActive) {
		t.Fatalf("signals = %v", pane.Signals)
	}
	if merged.Summary.Working != 1 {
		t.Fatalf("summary.Working = %d, want 1", merged.Summary.Working)
	}
	if session := merged.Sessions["main"]; !session.Running {
		t.Fatalf("session = %+v, want running", session)
	}
}

func TestApplyShellHooksCompletedMarksIdle(t *testing.T) {
	// The capture heuristic still says running via the hash debounce (the
	// prompt redraw changed the pane), but the matching precmd is stronger
	// evidence that the shell is back at its prompt.
	snapshot := hookMergeSnapshot(PaneStatus{
		Target: "main:0.0", PaneID: "%1", Session: "main",
		State: panestate.StateWorking, Running: true,
	})
	merged := ApplyShellHooks(snapshot, map[string]shellhook.PaneState{"%1": completedHookState("%1")})

	pane := merged.Panes["main:0.0"]
	if pane.Running || !pane.Idle || !pane.InputReady {
		t.Fatalf("pane = %+v, want idle/input-ready and not running", pane)
	}
	if pane.State != panestate.StateIdle {
		t.Fatalf("state = %q, want idle", pane.State)
	}
	if !hasSignal(pane.Signals, SignalShellHookCompleted) {
		t.Fatalf("signals = %v", pane.Signals)
	}
	if merged.Summary.Working != 0 {
		t.Fatalf("summary.Working = %d, want 0", merged.Summary.Working)
	}
}

func TestApplyShellHooksCompletedKeepsWaitingInput(t *testing.T) {
	snapshot := hookMergeSnapshot(PaneStatus{
		Target: "main:0.0", PaneID: "%1", Session: "main",
		State: panestate.StateWaitingInput, Signals: []string{"waiting_input_text"},
	})
	merged := ApplyShellHooks(snapshot, map[string]shellhook.PaneState{"%1": completedHookState("%1")})

	pane := merged.Panes["main:0.0"]
	if pane.State != panestate.StateWaitingInput {
		t.Fatalf("state = %q, want waiting_input preserved", pane.State)
	}
	if !pane.InputReady || !pane.Idle {
		t.Fatalf("pane = %+v, want idle/input-ready", pane)
	}
}

func TestApplyShellHooksNeverOverridesAsking(t *testing.T) {
	asking := PaneStatus{
		Target: "main:0.0", PaneID: "%1", Session: "main",
		State: panestate.StateWaitingPermission, Asking: true,
		Idle: false, InputReady: true, Running: false,
	}
	for name, state := range map[string]shellhook.PaneState{
		"active":    activeHookState("%1"),
		"completed": completedHookState("%1"),
	} {
		merged := ApplyShellHooks(hookMergeSnapshot(asking), map[string]shellhook.PaneState{"%1": state})
		pane := merged.Panes["main:0.0"]
		if pane.State != panestate.StateWaitingPermission || !pane.Asking {
			t.Fatalf("%s: pane = %+v, want asking preserved", name, pane)
		}
		// Every capture-derived flag survives: an asking pane must not count
		// as working anywhere downstream (summary, session, @ai-running).
		if pane.Running != asking.Running || pane.Idle != asking.Idle || pane.InputReady != asking.InputReady {
			t.Fatalf("%s: flags changed on asking pane: %+v", name, pane)
		}
		if !hasSignal(pane.Signals, SignalShellHook) {
			t.Fatalf("%s: signals = %v, want shell_hook recorded", name, pane.Signals)
		}
		if merged.Summary.Working != 0 {
			t.Fatalf("%s: summary.Working = %d, want 0", name, merged.Summary.Working)
		}
		if session := merged.Sessions["main"]; session.Running || !session.Asking {
			t.Fatalf("%s: session = %+v, want asking and not running", name, session)
		}
	}
}

func TestApplyShellHooksStaleActiveLosesToVisiblePrompt(t *testing.T) {
	// The precmd was lost (emits are fire-and-forget): the capture shows an
	// input-ready prompt, nothing changed for a while, and the "active"
	// command started well past the grace window. The capture wins.
	snapshotTime := time.Date(2026, 7, 7, 10, 0, 0, 0, time.UTC)
	pane := PaneStatus{
		Target: "main:0.0", PaneID: "%1", Session: "main",
		State: panestate.StateWaitingInput, Idle: true, InputReady: true,
	}
	stale := activeHookStateSince("%1", snapshotTime.Add(-30*time.Second))
	merged := ApplyShellHooks(hookMergeSnapshot(pane), map[string]shellhook.PaneState{"%1": stale})

	got := merged.Panes["main:0.0"]
	if got.Running || got.State != panestate.StateWaitingInput || !got.InputReady {
		t.Fatalf("pane = %+v, want capture waiting_input preserved", got)
	}
	if !hasSignal(got.Signals, SignalShellHookActiveStale) || hasSignal(got.Signals, SignalShellHookActive) {
		t.Fatalf("signals = %v, want stale active signal only", got.Signals)
	}

	// A fresh active command wins even over a visible prompt: the prompt on
	// screen is a leftover from just before the command started.
	fresh := activeHookStateSince("%1", snapshotTime.Add(-2*time.Second))
	merged = ApplyShellHooks(hookMergeSnapshot(pane), map[string]shellhook.PaneState{"%1": fresh})
	got = merged.Panes["main:0.0"]
	if !got.Running || got.State != panestate.StateWorking {
		t.Fatalf("pane = %+v, want fresh active to win", got)
	}
}

func TestApplyShellHooksStaleActiveLosesToIdleCapture(t *testing.T) {
	// Same as the waiting_input case, but the capture settled on a plain idle
	// prompt. A stale active hook (precmd lost) must not drag it back to
	// working; a fresh active still wins over the leftover prompt.
	snapshotTime := time.Date(2026, 7, 7, 10, 0, 0, 0, time.UTC)
	pane := PaneStatus{
		Target: "main:0.0", PaneID: "%1", Session: "main",
		State: panestate.StateIdle, Idle: true, InputReady: true,
	}
	stale := activeHookStateSince("%1", snapshotTime.Add(-30*time.Second))
	merged := ApplyShellHooks(hookMergeSnapshot(pane), map[string]shellhook.PaneState{"%1": stale})

	got := merged.Panes["main:0.0"]
	if got.Running || got.State != panestate.StateIdle || !got.Idle || !got.InputReady {
		t.Fatalf("pane = %+v, want capture idle preserved", got)
	}
	if merged.Summary.Working != 0 {
		t.Fatalf("summary.Working = %d, want 0", merged.Summary.Working)
	}
	if !hasSignal(got.Signals, SignalShellHookActiveStale) || hasSignal(got.Signals, SignalShellHookActive) {
		t.Fatalf("signals = %v, want stale active signal only", got.Signals)
	}

	// A fresh active command wins even over a visible idle prompt.
	fresh := activeHookStateSince("%1", snapshotTime.Add(-2*time.Second))
	merged = ApplyShellHooks(hookMergeSnapshot(pane), map[string]shellhook.PaneState{"%1": fresh})
	got = merged.Panes["main:0.0"]
	if !got.Running || got.State != panestate.StateWorking {
		t.Fatalf("pane = %+v, want fresh active to win", got)
	}
}

func TestApplyShellHooksCompletedDoesNotDowngradeWorkingText(t *testing.T) {
	// Capture explicitly sees "esc to interrupt"-style output: something is
	// still producing output outside the shell's foreground command, so a
	// stale completed hook must not force idle.
	snapshot := hookMergeSnapshot(PaneStatus{
		Target: "main:0.0", PaneID: "%1", Session: "main",
		State: panestate.StateWorking, Running: true, Signals: []string{"working_text"},
	})
	merged := ApplyShellHooks(snapshot, map[string]shellhook.PaneState{"%1": completedHookState("%1")})

	pane := merged.Panes["main:0.0"]
	if !pane.Running || pane.State != panestate.StateWorking {
		t.Fatalf("pane = %+v, want working preserved", pane)
	}
	if !hasSignal(pane.Signals, SignalShellHookCompleted) {
		t.Fatalf("signals = %v, want shell_hook_completed recorded", pane.Signals)
	}
}

func TestApplyShellHooksLeavesPanesWithoutHookData(t *testing.T) {
	original := PaneStatus{
		Target: "main:0.0", PaneID: "%1", Session: "main",
		State: panestate.StateWorking, Running: true,
	}
	merged := ApplyShellHooks(hookMergeSnapshot(original), map[string]shellhook.PaneState{"%9": completedHookState("%9")})

	pane := merged.Panes["main:0.0"]
	if pane.State != original.State || pane.Running != original.Running || len(pane.Signals) != 0 {
		t.Fatalf("pane = %+v, want untouched", pane)
	}
}

func TestDaemonRunOnceAppliesAndPrunesShellHooks(t *testing.T) {
	now := time.Date(2026, 7, 7, 10, 0, 0, 0, time.UTC)
	cfg := Config{
		TmuxOptions:    false,
		InitialSamples: 1,
		Now:            func() time.Time { return now },
		ListPanes: func() ([]tmux.Pane, error) {
			return []tmux.Pane{
				{Session: "alpha", WindowIndex: 0, PaneIndex: 0, PaneID: "%1", CurrentCommand: "zsh", Active: true},
			}, nil
		},
		CapturePane: func(target string, lines int) (string, error) {
			return "ready\nproject $\n", nil
		},
	}
	daemon := NewDaemon(cfg)
	mustHookRecord(t, daemon, shellhook.Event{Type: shellhook.TypePreexec, PaneID: "%1", Command: "make build", Timestamp: now})
	// Stale entry for a pane that no longer exists must get pruned.
	mustHookRecord(t, daemon, shellhook.Event{Type: shellhook.TypePreexec, PaneID: "%99", Timestamp: now.Add(-2 * time.Minute)})

	snapshot, err := daemon.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	pane := snapshot.Panes["alpha:0.0"]
	if !pane.Running || pane.State != panestate.StateWorking {
		t.Fatalf("pane = %+v, want working via shell hook", pane)
	}
	if !hasSignal(pane.Signals, SignalShellHookActive) {
		t.Fatalf("signals = %v", pane.Signals)
	}
	if _, ok := daemon.Hooks().State("%1"); !ok {
		t.Fatal("hook state for live pane was pruned")
	}
	if _, ok := daemon.Hooks().State("%99"); ok {
		t.Fatal("hook state for dead pane was not pruned")
	}
}

func mustHookRecord(t *testing.T, daemon *Daemon, e shellhook.Event) {
	t.Helper()
	if err := daemon.Hooks().Record(e); err != nil {
		t.Fatalf("Record(%+v): %v", e, err)
	}
}

func TestApplyShellHooksSkipsErroredPanes(t *testing.T) {
	snapshot := hookMergeSnapshot(PaneStatus{
		Target: "main:0.0", PaneID: "%1", Session: "main",
		State: panestate.StateUnknown, Error: "capture failed",
	})
	merged := ApplyShellHooks(snapshot, map[string]shellhook.PaneState{"%1": activeHookState("%1")})

	pane := merged.Panes["main:0.0"]
	if pane.Running || len(pane.Signals) != 0 {
		t.Fatalf("pane = %+v, want untouched on capture error", pane)
	}
}
