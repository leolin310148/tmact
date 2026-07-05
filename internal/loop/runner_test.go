package loop

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/leolin310148/tmact/internal/prompt"
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

func TestLoopPeerTargetUsesStatusdPaneEndpoints(t *testing.T) {
	enter := true
	var gotDiffPane, gotDiffLines string
	var gotInputs []map[string]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/pane/diff":
			gotDiffPane = r.URL.Query().Get("pane")
			gotDiffLines = r.URL.Query().Get("lines")
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"t":"patch","from":0,"lines":["ready"],"cursor":"c1"}`))
		case "/api/pane/input":
			if r.URL.Query().Get("pane") != "%7" {
				t.Fatalf("input pane = %q", r.URL.Query().Get("pane"))
			}
			defer r.Body.Close()
			var msg map[string]string
			if err := json.NewDecoder(r.Body).Decode(&msg); err != nil {
				t.Fatal(err)
			}
			gotInputs = append(gotInputs, msg)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"ok":true}`))
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer srv.Close()

	configPath := filepath.Join(t.TempDir(), "statusd.json")
	if err := os.WriteFile(configPath, []byte(`{"peers":[{"name":"peer-a","url":"`+srv.URL+`"}]}`), 0o644); err != nil {
		t.Fatal(err)
	}

	runner := NewRunner(Config{
		Target:        "peer-a@%7",
		StatusdConfig: configPath,
		CaptureLines:  80,
		IdleAfter:     Duration{Duration: time.Second},
		PollInterval:  Duration{Duration: time.Second},
		Actions:       []ActionConfig{{Name: "nudge", Type: "send_text", Text: "go", Enter: &enter}},
	}, Options{Once: true})

	if err := runner.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if gotDiffPane != "%7" || gotDiffLines != "80" {
		t.Fatalf("diff pane=%q lines=%q", gotDiffPane, gotDiffLines)
	}
	if len(gotInputs) != 1 || gotInputs[0]["t"] != "send" || gotInputs[0]["s"] != "go" {
		t.Fatalf("inputs = %#v", gotInputs)
	}
}

func TestLoopPeerTargetRejectsNonPaneRemoteTarget(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "statusd.json")
	if err := os.WriteFile(configPath, []byte(`{"peers":[{"name":"peer-a","url":"http://peer-a.example:7890"}]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	runner := NewRunner(Config{
		Target:        "peer-a@work:0.0",
		StatusdConfig: configPath,
		Actions:       []ActionConfig{{Name: "nudge", Type: "send_text", Text: "go"}},
	}, Options{Once: true})

	err := runner.Run(context.Background())
	if err == nil || !strings.Contains(err.Error(), "peer target must be a tmux pane id") {
		t.Fatalf("err = %v", err)
	}
}
