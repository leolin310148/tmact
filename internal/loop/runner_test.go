package loop

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"tmact/internal/prompt"
)

func TestLoopStopsOnGenericPrompt(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "loop.jsonl")
	runner := NewRunner(Config{
		Target:                 "demo:0.0",
		CaptureLines:           80,
		IdleAfter:              Duration{Duration: time.Second},
		PollInterval:           Duration{Duration: time.Second},
		StopOnPermissionPrompt: true,
		LogPath:                logPath,
		Actions:                []ActionConfig{{Name: "nudge", Type: "send_text", Text: "go"}},
	}, Options{DryRun: true})
	runner.capturePane = func(target string, lines int) (string, error) {
		return "Allow this command?\n  1. Yes\n❯ 2. No\n", nil
	}

	if err := runner.Run(context.Background()); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	var got event
	if err := json.Unmarshal(data[:len(data)-1], &got); err != nil {
		t.Fatal(err)
	}
	if got.Type != "stop" || got.Reason != "permission_prompt" {
		t.Fatalf("event = %#v", got)
	}
	details, ok := got.Details.(map[string]interface{})
	if !ok {
		t.Fatalf("details = %#v", got.Details)
	}
	if details["type"] != prompt.TypeCommandApproval || details["title"] != "Allow this command?" {
		t.Fatalf("details = %#v", details)
	}
}

func TestLoopPaneStatePreservesDirectoryAccessPrompt(t *testing.T) {
	runner := NewRunner(Config{
		Target:       "demo:0.0",
		CaptureLines: 80,
		IdleAfter:    Duration{Duration: time.Second},
	}, Options{DryRun: true})
	runner.capturePane = func(target string, lines int) (string, error) {
		return "Allow directory access\n/tmp/project\nDo you want to allow this?\n  1. Yes\n❯ 2. No\n", nil
	}

	state, _, err := runner.observe(time.Now(), "", time.Now().Add(-time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if state.InteractivePrompt == nil || state.InteractivePrompt.Type != prompt.TypeDirectoryAccess {
		t.Fatalf("interactive prompt = %#v", state.InteractivePrompt)
	}
	if state.PermissionPrompt == nil || state.PermissionPrompt.Title != "Allow directory access" || state.PermissionPrompt.Path != "/tmp/project" {
		t.Fatalf("permission prompt = %#v", state.PermissionPrompt)
	}
}
