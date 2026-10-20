package main

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/leolin310148/tmact/internal/modelroute"
)

func TestLogRoutingMetadataDefaultAndExplicitContent(t *testing.T) {
	t.Setenv("TMACT_ROUTING_LOG", filepath.Join(t.TempDir(), "routing.jsonl"))
	if err := modelroute.AppendLog(modelroute.LogEntry{Event: "decision", Trace: modelroute.Trace{DispatchID: "d_test"}, Input: &modelroute.Input{Task: "private task contents", Files: []modelroute.TaskFile{{Path: "task.md", Content: "private file contents"}}}, Selection: &modelroute.Selection{Difficulty: "advanced", Model: "gpt-6-astra"}}); err != nil {
		t.Fatal(err)
	}
	out, err := captureRun(t, "log", "routing", "--dispatch-id", "d_test", "--json")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, "private task contents") || strings.Contains(out, "private file contents") || !strings.Contains(out, "advanced") {
		t.Fatal(out)
	}
	out, err = captureRun(t, "log", "routing", "--dispatch-id", "d_test", "--show-content", "--json")
	if err != nil || !strings.Contains(out, "private file contents") {
		t.Fatalf("out=%s err=%v", out, err)
	}
	help, err := captureRun(t, "help", "log", "routing", "--json")
	if err != nil || !strings.Contains(help, "--show-content") || !strings.Contains(help, "routing.jsonl") {
		t.Fatalf("help=%s err=%v", help, err)
	}
}
