package runmeta

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRegisterAndListRunStatus(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "loop.jsonl")
	if err := os.WriteFile(logPath, []byte(strings.Join([]string{
		`{"ts":"2026-05-12T08:00:00Z","type":"action","target":"work:0.0","action":"prompt","status":"ok"}`,
		`{"ts":"2026-05-12T08:01:00Z","type":"stop","target":"work:0.0","reason":"max_runtime"}`,
	}, "\n")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	now := time.Date(2026, 5, 12, 8, 0, 0, 0, time.UTC)
	run, err := Register(dir, RegisterOptions{
		Kind:       "loop",
		ConfigPath: "examples/night-loop.yaml",
		Target:     "work:0.0",
		LogPath:    logPath,
		Now:        now,
	})
	if err != nil {
		t.Fatal(err)
	}
	if run.ID == "" || !strings.HasPrefix(run.ID, "loop-night-loop-") {
		t.Fatalf("id = %q", run.ID)
	}
	control, err := ReadControl(dir, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if control.DesiredState != DesiredRunning {
		t.Fatalf("control = %#v", control)
	}

	statuses, err := List(dir, "loop", now.Add(2*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if len(statuses) != 1 {
		t.Fatalf("statuses len = %d", len(statuses))
	}
	status := statuses[0]
	if status.RuntimeStatus != "running" {
		t.Fatalf("runtime status = %q", status.RuntimeStatus)
	}
	if status.LastEvent == nil || status.LastEvent.Type != "stop" || status.LastEvent.Reason != "max_runtime" {
		t.Fatalf("last event = %#v", status.LastEvent)
	}
	if len(status.RecentProblems) != 0 {
		t.Fatalf("problems = %#v", status.RecentProblems)
	}
}

func TestBuildStatusScopesSharedLogToRunWindowAndTarget(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "shared.jsonl")
	start := time.Date(2026, 7, 15, 1, 0, 0, 500_000_000, time.UTC)
	run := Run{
		ID:         "loop-first-1",
		Kind:       "loop",
		Target:     "work:0.0",
		LogPath:    logPath,
		StartedAt:  start,
		StoppedAt:  start.Add(10 * time.Minute),
		Status:     "stopped",
		Reason:     "requested",
		PID:        1,
		ConfigPath: filepath.Join(dir, "loop.yaml"),
	}
	lines := []string{
		`{"ts":"2026-07-15T00:59:59Z","type":"error","target":"work:0.0","reason":"before run"}`,
		`{"ts":"2026-07-15T01:00:00Z","type":"state","target":"work:0.0","status":"ready"}`,
		`{"ts":"2026-07-15T01:05:00Z","type":"action","target":"other:0.0","status":"failed","reason":"other target"}`,
		`{"ts":"2026-07-15T01:06:00Z","type":"action","target":"work:0.0","action":"prompt","status":"ok"}`,
		`{"ts":"2026-07-15T01:10:00Z","type":"stop","target":"work:0.0","reason":"requested"}`,
		`{"ts":"2026-07-15T01:10:00Z","run_id":"loop-second-2","type":"stop","target":"work:0.0","reason":"permission_prompt"}`,
		`{"ts":"2026-07-15T01:10:00Z","type":"flow","target":"work:0.0","status":"ok"}`,
		`{"ts":"2026-07-15T01:20:00Z","type":"stop","target":"work:0.0","reason":"permission_prompt"}`,
	}
	if err := os.WriteFile(logPath, []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	status, err := BuildStatus(run, start.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if status.LastEvent == nil || status.LastEvent.Reason != "requested" {
		t.Fatalf("last event = %#v", status.LastEvent)
	}
	if len(status.RecentProblems) != 0 {
		t.Fatalf("later or other-target problems leaked into run: %#v", status.RecentProblems)
	}
}

func TestHeartbeatAndFinishPreserveLatestPhase(t *testing.T) {
	dir := t.TempDir()
	start := time.Date(2026, 5, 12, 8, 0, 0, 0, time.UTC)
	run, err := Register(dir, RegisterOptions{
		Kind:       "loop",
		ConfigPath: "loop.yaml",
		Target:     "demo:0.0",
		Now:        start,
	})
	if err != nil {
		t.Fatal(err)
	}
	beat := start.Add(time.Minute)
	if err := Heartbeat(dir, run, "waiting_idle", beat); err != nil {
		t.Fatal(err)
	}
	finish := beat.Add(time.Minute)
	if err := Finish(dir, run, "stopped", "requested", finish); err != nil {
		t.Fatal(err)
	}
	got, err := Read(dir, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != "stopped" || got.Phase != "stopped" || !got.HeartbeatAt.Equal(finish) {
		t.Fatalf("run = %#v", got)
	}
	statuses, err := List(dir, "loop", finish)
	if err != nil {
		t.Fatal(err)
	}
	if len(statuses) != 1 {
		t.Fatalf("control file leaked into run list: %#v", statuses)
	}
}

func TestSelectRunByConfig(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "loop.yaml")
	if err := os.WriteFile(configPath, []byte("target: work:0.0\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run, err := Register(dir, RegisterOptions{Kind: "loop", ConfigPath: configPath, Target: "work:0.0"})
	if err != nil {
		t.Fatal(err)
	}
	statuses, err := List(dir, "loop", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	selected, err := Select(statuses, "", configPath)
	if err != nil {
		t.Fatal(err)
	}
	if selected.ID != run.ID {
		t.Fatalf("selected id = %q want %q", selected.ID, run.ID)
	}
}

func TestSelectRunByConfigPrefersActiveRun(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "loop.yaml")
	statuses := []Status{
		{
			Run: Run{
				ID:         "old",
				Kind:       "loop",
				ConfigPath: configPath,
				Status:     "stopped",
			},
			RuntimeStatus: "stopped",
		},
		{
			Run: Run{
				ID:         "current",
				Kind:       "loop",
				ConfigPath: configPath,
				Status:     "running",
			},
			RuntimeStatus: "running",
		},
	}
	selected, err := Select(statuses, "", configPath)
	if err != nil {
		t.Fatal(err)
	}
	if selected.ID != "current" {
		t.Fatalf("selected id = %q", selected.ID)
	}
}
