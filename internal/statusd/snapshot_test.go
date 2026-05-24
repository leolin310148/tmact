package statusd

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/leolin310148/tmact/internal/panestatus"
	"github.com/leolin310148/tmact/internal/tmux"
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
		InitialSamples:  1,
		RunningDebounce: 5 * time.Second,
		Now:             func() time.Time { return now },
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

func TestBuildSessionsMarksRunningWhenAnyPaneRuns(t *testing.T) {
	now := time.Date(2026, 5, 12, 2, 0, 0, 0, time.UTC)
	sessions := buildSessions(map[string]PaneStatus{
		"work:0.0": {
			Target:       "work:0.0",
			Session:      "work",
			WindowIndex:  0,
			WindowActive: true,
			PaneIndex:    0,
			Active:       true,
			Runtime:      panestatus.RuntimeCodex,
			Tag:          "cx",
			State:        "waiting_input",
			Running:      false,
		},
		"work:1.0": {
			Target:       "work:1.0",
			Session:      "work",
			WindowIndex:  1,
			WindowActive: false,
			PaneIndex:    0,
			Active:       true,
			Runtime:      panestatus.RuntimeClaude,
			Tag:          "cc",
			State:        "working",
			Running:      true,
		},
	}, now)

	session := sessions["work"]
	if session.ActiveTarget != "work:0.0" {
		t.Fatalf("active target = %q", session.ActiveTarget)
	}
	if !session.Running {
		t.Fatalf("session should be running when any pane is running: %#v", session)
	}
	if session.Tag != "cx" || session.State != "waiting_input" {
		t.Fatalf("session active-pane details changed: %#v", session)
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

func TestBuildSnapshotMarksProceedQuestionAsAsking(t *testing.T) {
	now := time.Date(2026, 5, 12, 2, 0, 0, 0, time.UTC)
	cfg := Config{
		Now: func() time.Time { return now },
		ListPanes: func() ([]tmux.Pane, error) {
			return []tmux.Pane{{Session: "hc-api-sb3", WindowIndex: 0, WindowActive: true, PaneIndex: 0, PaneID: "%1", CurrentCommand: "codex", Active: true}}, nil
		},
		CapturePane: func(target string, lines int) (string, error) {
			return "Do you want to Proceed?\n  1. Yes\n  2. No\n", nil
		},
	}

	snapshot, err := BuildSnapshot(context.Background(), cfg, NewMemory())
	if err != nil {
		t.Fatalf("BuildSnapshot returned error: %v", err)
	}
	pane := snapshot.Panes["hc-api-sb3:0.0"]
	if !pane.Asking {
		t.Fatalf("pane should be asking: %#v", pane)
	}
	if pane.State != "waiting_permission" {
		t.Fatalf("state = %q", pane.State)
	}
	if !snapshot.Sessions["hc-api-sb3"].Asking {
		t.Fatalf("session should be asking: %#v", snapshot.Sessions["hc-api-sb3"])
	}
	if pane.Prompt == nil || pane.Prompt.Type != "generic_confirmation" {
		t.Fatalf("prompt = %#v", pane.Prompt)
	}
}

func TestBuildSnapshotMarksTrailingChoicePromptAsAsking(t *testing.T) {
	now := time.Date(2026, 5, 12, 2, 0, 0, 0, time.UTC)
	cfg := Config{
		Now: func() time.Time { return now },
		ListPanes: func() ([]tmux.Pane, error) {
			return []tmux.Pane{{
				Session:        "hc-api",
				WindowIndex:    0,
				WindowName:     "claude",
				WindowActive:   true,
				PaneIndex:      0,
				PaneID:         "%1",
				CurrentCommand: "2.1.144",
				Active:         true,
			}}, nil
		},
		CapturePane: func(target string, lines int) (string, error) {
			return `
Skill 位置

4 個 skill 要放哪?這影響是否進版控、team 是否看得到、以及是否馬上在 worktree 可用。

❯ 1. 專案 .claude/skills/ (推薦)
  2. 個人 ~/.claude/skills/
  3. Type something.

Enter to select · ↑/↓ to navigate · Esc to cancel
`, nil
		},
	}

	snapshot, err := BuildSnapshot(context.Background(), cfg, NewMemory())
	if err != nil {
		t.Fatalf("BuildSnapshot returned error: %v", err)
	}
	pane := snapshot.Panes["hc-api:0.0"]
	if !pane.Asking {
		t.Fatalf("pane should be asking: %#v", pane)
	}
	if !snapshot.Sessions["hc-api"].Asking {
		t.Fatalf("session should be asking: %#v", snapshot.Sessions["hc-api"])
	}
	if pane.Prompt == nil || pane.Prompt.Type != "choice_prompt" {
		t.Fatalf("prompt = %#v", pane.Prompt)
	}
}

func TestBuildSnapshotUsesInitialSamplesOnColdStart(t *testing.T) {
	now := time.Date(2026, 5, 12, 2, 0, 0, 0, time.UTC)
	captures := 0
	cfg := Config{
		InitialSamples: 2,
		Now:            func() time.Time { return now },
		ListPanes: func() ([]tmux.Pane, error) {
			return []tmux.Pane{{Session: "worker", WindowIndex: 0, PaneIndex: 0, PaneID: "%1", CurrentCommand: "codex", Active: true}}, nil
		},
		CapturePane: func(target string, lines int) (string, error) {
			captures++
			return "same output\n", nil
		},
	}

	snapshot, err := BuildSnapshot(context.Background(), cfg, NewMemory())
	if err != nil {
		t.Fatalf("BuildSnapshot returned error: %v", err)
	}
	if captures != 2 {
		t.Fatalf("captures = %d", captures)
	}
	pane := snapshot.Panes["worker:0.0"]
	if pane.State != "idle" || !pane.Idle {
		t.Fatalf("pane should be idle after stable cold-start samples: %#v", pane)
	}
}

func TestBuildSnapshotSkipsCaptureForShellRuntime(t *testing.T) {
	now := time.Date(2026, 5, 12, 2, 0, 0, 0, time.UTC)
	captures := 0
	cfg := Config{
		Now: func() time.Time { return now },
		ListPanes: func() ([]tmux.Pane, error) {
			return []tmux.Pane{{Session: "shell", WindowIndex: 0, PaneIndex: 0, PaneID: "%1", CurrentCommand: "zsh", Active: true}}, nil
		},
		CapturePane: func(target string, lines int) (string, error) {
			captures++
			return "project $\n", nil
		},
	}

	snapshot, err := BuildSnapshot(context.Background(), cfg, NewMemory())
	if err != nil {
		t.Fatalf("BuildSnapshot returned error: %v", err)
	}
	if captures != 0 {
		t.Fatalf("captures = %d", captures)
	}
	pane := snapshot.Panes["shell:0.0"]
	if pane.Runtime != panestatus.RuntimeShell {
		t.Fatalf("runtime = %q", pane.Runtime)
	}
	if pane.State != "unknown" {
		t.Fatalf("state = %q", pane.State)
	}
}

func TestBuildSnapshotDefaultsToFortyCaptureLines(t *testing.T) {
	now := time.Date(2026, 5, 12, 2, 0, 0, 0, time.UTC)
	var gotLines []int
	cfg := Config{
		InitialSamples: 1,
		Now:            func() time.Time { return now },
		ListPanes: func() ([]tmux.Pane, error) {
			return []tmux.Pane{{Session: "agent", WindowIndex: 0, PaneIndex: 0, PaneID: "%1", CurrentCommand: "codex", Active: true}}, nil
		},
		CapturePane: func(target string, lines int) (string, error) {
			gotLines = append(gotLines, lines)
			return "›\n", nil
		},
	}

	if _, err := BuildSnapshot(context.Background(), cfg, NewMemory()); err != nil {
		t.Fatalf("BuildSnapshot returned error: %v", err)
	}
	if !reflect.DeepEqual(gotLines, []int{40}) {
		t.Fatalf("lines = %#v", gotLines)
	}
}

func TestRunOnceDoesNotPublishSnapshotOnScanFailure(t *testing.T) {
	good := newSnapshot(Config{}, time.Date(2026, 5, 12, 2, 0, 0, 0, time.UTC))
	good.Summary.Panes = 7

	daemon := NewDaemon(Config{
		ListPanes: func() ([]tmux.Pane, error) {
			return nil, errors.New("tmux unavailable")
		},
	})
	daemon.Store().Publish(good)
	if _, err := daemon.RunOnce(context.Background()); err == nil {
		t.Fatal("expected scan error")
	}

	got, ok := daemon.Store().Latest()
	if !ok {
		t.Fatal("store should still hold the prior snapshot")
	}
	if got.Summary.Panes != 7 {
		t.Fatalf("snapshot was overwritten: %#v", got.Summary)
	}
}

func TestRunOncePublishesSnapshot(t *testing.T) {
	daemon := NewDaemon(Config{
		InitialSamples: 1,
		ListPanes: func() ([]tmux.Pane, error) {
			return []tmux.Pane{{Session: "alpha", PaneID: "%1", Active: true}}, nil
		},
		CapturePane: func(string, int) (string, error) { return "ready\n", nil },
	})
	if _, err := daemon.RunOnce(context.Background()); err != nil {
		t.Fatalf("RunOnce returned error: %v", err)
	}
	got, ok := daemon.Store().Latest()
	if !ok {
		t.Fatal("Store should hold a snapshot after RunOnce")
	}
	if got.Summary.Panes != 1 {
		t.Fatalf("panes = %d, want 1", got.Summary.Panes)
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
		"work @ai-asking=?",
		"work @row-bucket=2",
	}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("calls = %#v, want %#v", calls, want)
	}
}

func TestPublishTmuxOptionsSkipsUnchangedWithCache(t *testing.T) {
	var calls []string
	cfg := Config{
		SetSessionOption: func(session string, key string, value string) error {
			calls = append(calls, session+" "+key+"="+value)
			return nil
		},
	}
	cache := NewTmuxOptionCache()
	snapshot := Snapshot{Sessions: map[string]SessionStatus{
		"work": {Session: "work", Tag: "cx", Running: true, Asking: true, RowBucket: 2},
	}}

	if err := PublishTmuxOptions(cfg, snapshot, cache); err != nil {
		t.Fatalf("first PublishTmuxOptions returned error: %v", err)
	}
	if err := PublishTmuxOptions(cfg, snapshot, cache); err != nil {
		t.Fatalf("second PublishTmuxOptions returned error: %v", err)
	}
	want := []string{
		"work @ai-tag=cx",
		"work @ai-running=▸",
		"work @ai-asking=?",
		"work @row-bucket=2",
	}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("calls = %#v, want %#v", calls, want)
	}

	changed := Snapshot{Sessions: map[string]SessionStatus{
		"work": {Session: "work", Tag: "cx", Running: false, Asking: true, RowBucket: 2},
	}}
	if err := PublishTmuxOptions(cfg, changed, cache); err != nil {
		t.Fatalf("changed PublishTmuxOptions returned error: %v", err)
	}
	want = append(want, "work @ai-running=")
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("calls after change = %#v, want %#v", calls, want)
	}

	if err := PublishTmuxOptions(cfg, changed, cache); err != nil {
		t.Fatalf("repeated empty PublishTmuxOptions returned error: %v", err)
	}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("calls after repeated empty = %#v, want %#v", calls, want)
	}
}

func TestPublishTmuxOptionsRewritesRecreatedSessionWithSameName(t *testing.T) {
	var calls []string
	cfg := Config{
		SetSessionOption: func(session string, key string, value string) error {
			calls = append(calls, session+" "+key+"="+value)
			return nil
		},
	}
	cache := NewTmuxOptionCache()
	first := Snapshot{Sessions: map[string]SessionStatus{
		"sample": {Session: "sample", SessionID: "$1", Tag: "$", Running: false, Asking: false, RowBucket: 1},
	}}
	second := Snapshot{Sessions: map[string]SessionStatus{
		"sample": {Session: "sample", SessionID: "$2", Tag: "$", Running: false, Asking: false, RowBucket: 1},
	}}

	if err := PublishTmuxOptions(cfg, first, cache); err != nil {
		t.Fatalf("first PublishTmuxOptions returned error: %v", err)
	}
	if err := PublishTmuxOptions(cfg, second, cache); err != nil {
		t.Fatalf("second PublishTmuxOptions returned error: %v", err)
	}

	want := []string{
		"sample @ai-tag=$",
		"sample @ai-running=",
		"sample @ai-asking=",
		"sample @row-bucket=1",
		"sample @ai-tag=$",
		"sample @ai-running=",
		"sample @ai-asking=",
		"sample @row-bucket=1",
	}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("calls = %#v, want %#v", calls, want)
	}
}
