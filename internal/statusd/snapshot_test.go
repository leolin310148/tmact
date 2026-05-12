package statusd

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"tmact/internal/panestatus"
	"tmact/internal/tmux"
)

func TestRuntimeTag(t *testing.T) {
	tests := map[string]string{
		panestatus.RuntimeClaude:  "cc",
		panestatus.RuntimeCodex:   "cx",
		panestatus.RuntimeCopilot: "cp",
		panestatus.RuntimeGemini:  "g",
		panestatus.RuntimeTmact:   "tm",
		panestatus.RuntimeShell:   "$",
		panestatus.RuntimeUnknown: "$",
		"other":                   "$",
	}
	for runtime, want := range tests {
		if got := RuntimeTag(runtime); got != want {
			t.Fatalf("RuntimeTag(%q) = %q, want %q", runtime, got, want)
		}
	}
}

func TestBuildSnapshotAggregatesSessionsAndDebouncesRunning(t *testing.T) {
	now := time.Date(2026, 5, 12, 2, 0, 0, 0, time.UTC)
	mem := NewMemory()
	captures := []string{"ready\nproject $\n", "ready\nbeta $\n"}
	cfg := Config{
		Now: func() time.Time { return now },
		ListPanes: func() ([]tmux.Pane, error) {
			return []tmux.Pane{
				{Session: "alpha", WindowIndex: 0, PaneIndex: 0, PaneID: "%1", CurrentCommand: "codex", Active: true},
				{Session: "beta", WindowIndex: 0, PaneIndex: 0, PaneID: "%2", CurrentCommand: "zsh", Active: true},
			}, nil
		},
		CapturePane: func(target string, lines int) (string, error) {
			next := captures[0]
			captures = captures[1:]
			return next, nil
		},
	}

	first, err := BuildSnapshot(context.Background(), cfg, mem)
	if err != nil {
		t.Fatalf("BuildSnapshot returned error: %v", err)
	}
	if first.Summary.Sessions != 2 || first.Summary.Panes != 2 {
		t.Fatalf("summary = %#v", first.Summary)
	}
	if first.Sessions["alpha"].Tag != "cx" {
		t.Fatalf("alpha tag = %q", first.Sessions["alpha"].Tag)
	}
	if first.Sessions["beta"].RowBucket != 1 {
		t.Fatalf("beta row bucket = %d", first.Sessions["beta"].RowBucket)
	}

	now = now.Add(time.Second)
	captures = []string{"ready\nproject $\nchanged\n", "ready\nbeta $\n"}
	second, err := BuildSnapshot(context.Background(), cfg, mem)
	if err != nil {
		t.Fatalf("BuildSnapshot returned error: %v", err)
	}
	if !second.Panes["alpha:0.0"].Running {
		t.Fatalf("alpha pane should be running after changed capture: %#v", second.Panes["alpha:0.0"])
	}
	if second.Summary.Working != 1 {
		t.Fatalf("working count = %d", second.Summary.Working)
	}

	now = now.Add(10 * time.Second)
	captures = []string{"ready\nproject $\nchanged\n", "ready\nbeta $\n"}
	third, err := BuildSnapshot(context.Background(), cfg, mem)
	if err != nil {
		t.Fatalf("BuildSnapshot returned error: %v", err)
	}
	if third.Panes["alpha:0.0"].Running {
		t.Fatalf("alpha pane should no longer be running: %#v", third.Panes["alpha:0.0"])
	}
}

func TestBuildSnapshotUsesActivePaneFromActiveWindow(t *testing.T) {
	now := time.Date(2026, 5, 12, 2, 0, 0, 0, time.UTC)
	cfg := Config{
		Now: func() time.Time { return now },
		ListPanes: func() ([]tmux.Pane, error) {
			return []tmux.Pane{
				{Session: "work", WindowIndex: 0, WindowName: "codex", WindowActive: false, PaneIndex: 0, PaneID: "%1", CurrentCommand: "codex", Active: true},
				{Session: "work", WindowIndex: 1, WindowName: "claude", WindowActive: true, PaneIndex: 0, PaneID: "%2", CurrentCommand: "claude", Active: true},
			}, nil
		},
		CapturePane: func(target string, lines int) (string, error) {
			return "ready\n›\n", nil
		},
	}

	snapshot, err := BuildSnapshot(context.Background(), cfg, NewMemory())
	if err != nil {
		t.Fatalf("BuildSnapshot returned error: %v", err)
	}
	session := snapshot.Sessions["work"]
	if session.ActiveTarget != "work:1.0" {
		t.Fatalf("active target = %q", session.ActiveTarget)
	}
	if session.Tag != "cc" {
		t.Fatalf("tag = %q", session.Tag)
	}
}

func TestBuildSnapshotMarksAskingFromRecentApprovalText(t *testing.T) {
	now := time.Date(2026, 5, 12, 2, 0, 0, 0, time.UTC)
	cfg := Config{
		Now: func() time.Time { return now },
		ListPanes: func() ([]tmux.Pane, error) {
			return []tmux.Pane{{Session: "work", WindowIndex: 0, WindowActive: true, PaneIndex: 0, PaneID: "%1", CurrentCommand: "codex", Active: true}}, nil
		},
		CapturePane: func(target string, lines int) (string, error) {
			return "Waiting for approval\nsome status footer\n›\n", nil
		},
	}

	snapshot, err := BuildSnapshot(context.Background(), cfg, NewMemory())
	if err != nil {
		t.Fatalf("BuildSnapshot returned error: %v", err)
	}
	if !snapshot.Panes["work:0.0"].Asking {
		t.Fatalf("pane should be asking: %#v", snapshot.Panes["work:0.0"])
	}
	if !snapshot.Sessions["work"].Asking {
		t.Fatalf("session should be asking: %#v", snapshot.Sessions["work"])
	}
}

func TestRunOnceDoesNotOverwriteSnapshotOnScanFailure(t *testing.T) {
	path := filepath.Join(t.TempDir(), "status.json")
	good := newSnapshot(Config{}, time.Date(2026, 5, 12, 2, 0, 0, 0, time.UTC))
	good.Summary.Panes = 7
	if err := WriteSnapshot(path, good); err != nil {
		t.Fatalf("WriteSnapshot returned error: %v", err)
	}

	daemon := NewDaemon(Config{
		StatePath: path,
		ListPanes: func() ([]tmux.Pane, error) {
			return nil, errors.New("tmux unavailable")
		},
	})
	if _, err := daemon.RunOnce(context.Background()); err == nil {
		t.Fatal("expected scan error")
	}

	read, err := ReadSnapshot(path)
	if err != nil {
		t.Fatalf("ReadSnapshot returned error: %v", err)
	}
	if read.Summary.Panes != 7 {
		t.Fatalf("snapshot was overwritten: %#v", read.Summary)
	}
}

func TestWriteAndReadSnapshot(t *testing.T) {
	path := filepath.Join(t.TempDir(), "status.json")
	snapshot := newSnapshot(Config{}, time.Date(2026, 5, 12, 2, 0, 0, 0, time.UTC))
	snapshot.Summary.Panes = 3

	if err := WriteSnapshot(path, snapshot); err != nil {
		t.Fatalf("WriteSnapshot returned error: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatal(err)
	}
	read, err := ReadSnapshot(path)
	if err != nil {
		t.Fatalf("ReadSnapshot returned error: %v", err)
	}
	if read.Summary.Panes != 3 {
		t.Fatalf("panes = %d", read.Summary.Panes)
	}
}

func TestSnapshotStaleness(t *testing.T) {
	ts := time.Date(2026, 5, 12, 2, 0, 0, 0, time.UTC)
	snapshot := Snapshot{Timestamp: ts, StaleAfterMS: 1000}
	if snapshot.IsStale(ts.Add(500 * time.Millisecond)) {
		t.Fatal("snapshot should not be stale")
	}
	if !snapshot.IsStale(ts.Add(2 * time.Second)) {
		t.Fatal("snapshot should be stale")
	}
}

func TestPublishTmuxOptions(t *testing.T) {
	var calls []string
	cfg := Config{
		SetSessionOption: func(session string, key string, value string) error {
			calls = append(calls, session+" "+key+"="+value)
			return nil
		},
	}
	snapshot := Snapshot{Sessions: map[string]SessionStatus{
		"work": {Session: "work", Tag: "cx", Running: true, Asking: true, RowBucket: 2},
	}}

	if err := PublishTmuxOptions(cfg, snapshot); err != nil {
		t.Fatalf("PublishTmuxOptions returned error: %v", err)
	}
	want := []string{
		"work @ai-tag=cx",
		"work @ai-running=▸",
		"work @ai-asking=!",
		"work @row-bucket=2",
	}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("calls = %#v, want %#v", calls, want)
	}
}
