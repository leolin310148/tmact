package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/leolin310148/tmact/internal/runmeta"
)

func TestLoopStatusPrintsRegisteredRuns(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "loop.jsonl")
	if err := os.WriteFile(logPath, []byte(`{"ts":"2026-05-12T08:00:00Z","type":"action","target":"work:0.0","action":"prompt","status":"ok"}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := runmeta.Write(dir, runmeta.Run{
		ID:         "loop-night-loop-123",
		Kind:       "loop",
		ConfigPath: "/repo/examples/night-loop.yaml",
		Target:     "work:0.0",
		LogPath:    logPath,
		PID:        os.Getpid(),
		StartedAt:  time.Date(2026, 5, 12, 8, 0, 0, 0, time.UTC),
		UpdatedAt:  time.Date(2026, 5, 12, 8, 0, 0, 0, time.UTC),
		Status:     "running",
	}); err != nil {
		t.Fatal(err)
	}

	out, err := captureRun(t, "loop", "status", "--run-dir", dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"loop-night-loop-123", "running", "work:0.0", "/repo/examples/night-loop.yaml", "action:prompt"} {
		if !strings.Contains(out, want) {
			t.Fatalf("output missing %q: %s", want, out)
		}
	}
}
