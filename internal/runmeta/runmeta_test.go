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

	statuses, err := List(dir, "loop", now)
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
	if len(status.RecentProblems) != 1 || status.RecentProblems[0].Type != "stop" {
		t.Fatalf("problems = %#v", status.RecentProblems)
	}
}

func TestSelectRunByConfig(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "workflow.yaml")
	if err := os.WriteFile(configPath, []byte("target: work:0.0\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run, err := Register(dir, RegisterOptions{Kind: "workflow", ConfigPath: configPath, Target: "work:0.0"})
	if err != nil {
		t.Fatal(err)
	}
	statuses, err := List(dir, "workflow", time.Now())
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
	configPath := filepath.Join(t.TempDir(), "workflow.yaml")
	statuses := []Status{
		{
			Run: Run{
				ID:         "old",
				Kind:       "workflow",
				ConfigPath: configPath,
				Status:     "stopped",
			},
			RuntimeStatus: "stopped",
		},
		{
			Run: Run{
				ID:         "current",
				Kind:       "workflow",
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
