package web

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/leolin310148/tmact/internal/panestate"
	"github.com/leolin310148/tmact/internal/prompt"
	"github.com/leolin310148/tmact/internal/statusd"
)

func TestProjectWebSnapshotKeepsOnlyBrowserPaneFields(t *testing.T) {
	now := time.Date(2026, 8, 8, 12, 0, 0, 0, time.UTC)
	question := &prompt.Prompt{
		Type:     prompt.TypeChoicePrompt,
		Question: "Choose",
		Options:  []prompt.Option{{Number: 1, Label: "one"}},
	}
	snap := statusd.Snapshot{
		Version:      1,
		Timestamp:    now,
		GeneratedBy:  "tmact statusd",
		IntervalMS:   500,
		StaleAfterMS: 10_000,
		Sessions: map[string]statusd.SessionStatus{
			"work": {Session: "work", UpdatedAt: now},
		},
		Panes: map[string]statusd.PaneStatus{
			"work:0.0": {
				Target:         "work:0.0",
				PaneID:         "%7",
				Session:        "work",
				SessionID:      "$4",
				WindowIndex:    0,
				Window:         "node",
				PaneIndex:      0,
				CWD:            "/tmp/work",
				CurrentCommand: "node",
				Runtime:        "codex",
				Tag:            "cx",
				State:          panestate.StateWaitingInput,
				Idle:           true,
				InputReady:     true,
				Asking:         true,
				Confidence:     "high",
				Signals:        []string{"pane_text"},
				Prompt:         question,
				LastLine:       "ready",
				UpdatedAt:      now,
			},
		},
	}

	data, err := json.Marshal(projectWebSnapshot(snap))
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	if _, ok := got["sessions"]; ok {
		t.Fatal("compact Web snapshot unexpectedly contains sessions")
	}
	panes := got["panes"].(map[string]any)
	pane := panes["work:0.0"].(map[string]any)
	for _, field := range []string{"session_id", "window", "current_command", "tag", "input_ready", "confidence", "signals", "last_line", "updated_at"} {
		if _, ok := pane[field]; ok {
			t.Fatalf("compact Web pane unexpectedly contains %q", field)
		}
	}
	for _, field := range []string{"pane_id", "session", "cwd", "runtime", "state", "idle", "running", "asking", "prompt"} {
		if _, ok := pane[field]; !ok {
			t.Fatalf("compact Web pane is missing %q", field)
		}
	}
}

func TestWebSnapshotStreamStateDeduplicatesAndHeartbeats(t *testing.T) {
	base := time.Date(2026, 8, 8, 12, 0, 0, 0, time.UTC)
	snap := statusd.Snapshot{
		Version:      1,
		Timestamp:    base,
		IntervalMS:   500,
		StaleAfterMS: 10_000,
		Panes: map[string]statusd.PaneStatus{
			"work:0.0": {Target: "work:0.0", PaneID: "%7", Session: "work"},
		},
	}
	var state webSnapshotStreamState

	event, _, emit, err := state.next(snap)
	if err != nil || !emit || event != "snapshot" {
		t.Fatalf("initial next = event %q emit %v err %v", event, emit, err)
	}

	snap.Timestamp = base.Add(500 * time.Millisecond)
	event, _, emit, err = state.next(snap)
	if err != nil || emit || event != "" {
		t.Fatalf("unchanged next = event %q emit %v err %v", event, emit, err)
	}

	snap.Timestamp = base.Add(5 * time.Second)
	event, payload, emit, err := state.next(snap)
	if err != nil || !emit || event != "heartbeat" {
		t.Fatalf("heartbeat next = event %q emit %v err %v", event, emit, err)
	}
	if got := payload.(webSnapshotHeartbeat).Timestamp; !got.Equal(snap.Timestamp) {
		t.Fatalf("heartbeat timestamp = %v, want %v", got, snap.Timestamp)
	}

	pane := snap.Panes["work:0.0"]
	pane.Running = true
	snap.Panes["work:0.0"] = pane
	snap.Timestamp = base.Add(5500 * time.Millisecond)
	event, _, emit, err = state.next(snap)
	if err != nil || !emit || event != "snapshot" {
		t.Fatalf("changed next = event %q emit %v err %v", event, emit, err)
	}
}

func TestWebSnapshotHeartbeatIntervalTracksStaleThreshold(t *testing.T) {
	if got := webSnapshotHeartbeatInterval(30_000); got != 5*time.Second {
		t.Fatalf("large stale threshold interval = %v, want 5s", got)
	}
	if got := webSnapshotHeartbeatInterval(4_000); got != 2*time.Second {
		t.Fatalf("small stale threshold interval = %v, want 2s", got)
	}
}
