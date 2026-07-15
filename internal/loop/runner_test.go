package loop

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/leolin310148/tmact/internal/prompt"
)

func TestLoopEventsIncludeManagedRunID(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "loop.jsonl")
	runner := NewRunner(Config{LogPath: logPath}, Options{RunID: "loop-demo-123"})
	if err := runner.emit(event{Timestamp: "2026-07-15T01:00:00Z", Type: "state", Target: "demo:0.0"}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"run_id":"loop-demo-123"`) {
		t.Fatalf("event missing managed run id: %s", data)
	}
}

func TestLoopStopsCooperativelyWhileWaitingForNextPoll(t *testing.T) {
	calls := 0
	runner := NewRunner(Config{
		Target:       "demo:0.0",
		CaptureLines: 20,
		IdleAfter:    Duration{Duration: time.Hour},
		PollInterval: Duration{Duration: time.Hour},
		Actions: []ActionConfig{{
			Name:         "later",
			Type:         "send_text",
			Text:         "go",
			InitialDelay: Duration{Duration: time.Hour},
		}},
	}, Options{
		DryRun: true,
		Control: func() (string, error) {
			calls++
			if calls >= 2 {
				return "stopped", nil
			}
			return "running", nil
		},
	})
	runner.capturePane = func(string, int) (string, error) { return "idle", nil }

	started := time.Now()
	err := runner.Run(context.Background())
	if !errors.Is(err, ErrStopRequested) {
		t.Fatalf("err = %v", err)
	}
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("cooperative stop took %s", elapsed)
	}
}

func TestLoopResumeAcknowledgesWithoutWaitingForLongPollInterval(t *testing.T) {
	var desired atomic.Value
	desired.Store("paused")
	phases := make(chan string, 8)
	runner := NewRunner(Config{
		Target:       "demo:0.0",
		CaptureLines: 20,
		IdleAfter:    Duration{Duration: time.Hour},
		PollInterval: Duration{Duration: time.Hour},
		Actions: []ActionConfig{{
			Name:         "later",
			Type:         "send_text",
			Text:         "go",
			InitialDelay: Duration{Duration: time.Hour},
		}},
	}, Options{
		DryRun: true,
		Control: func() (string, error) {
			return desired.Load().(string), nil
		},
		Heartbeat: func(phase string) error {
			phases <- phase
			return nil
		},
	})
	runner.capturePane = func(string, int) (string, error) { return "idle", nil }
	done := make(chan error, 1)
	go func() { done <- runner.Run(context.Background()) }()

	waitForPhase := func(want string) {
		t.Helper()
		timer := time.NewTimer(time.Second)
		defer timer.Stop()
		for {
			select {
			case phase := <-phases:
				if phase == want {
					return
				}
			case <-timer.C:
				t.Fatalf("timed out waiting for phase %q", want)
			}
		}
	}
	waitForPhase("paused")
	desired.Store("running")
	waitForPhase("resuming")
	desired.Store("stopped")
	select {
	case err := <-done:
		if !errors.Is(err, ErrStopRequested) {
			t.Fatalf("err = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("runner did not stop")
	}
}

func TestLoopCompletesWhenFiniteSchedulesAreExhausted(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "loop.jsonl")
	runner := NewRunner(Config{
		Target:       "demo:0.0",
		CaptureLines: 20,
		IdleAfter:    Duration{Duration: time.Second},
		PollInterval: Duration{Duration: time.Hour},
		LogPath:      logPath,
		Actions:      []ActionConfig{{Name: "once", Type: "send_text", Text: "go"}},
	}, Options{DryRun: true})
	runner.capturePane = func(string, int) (string, error) { return "idle", nil }

	if err := runner.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"reason":"actions_exhausted"`) {
		t.Fatalf("log = %s", data)
	}
}

func TestLoopDoesNotStartFlowThatWouldExceedMaxActions(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "loop.jsonl")
	runner := NewRunner(Config{
		Target:       "demo:0.0",
		CaptureLines: 20,
		IdleAfter:    Duration{Duration: time.Second},
		PollInterval: Duration{Duration: time.Hour},
		MaxActions:   1,
		LogPath:      logPath,
		Flows: []FlowConfig{{
			Name: "two-steps",
			Steps: []ActionConfig{
				{Name: "one", Type: "send_text", Text: "one"},
				{Name: "two", Type: "send_text", Text: "two"},
			},
		}},
	}, Options{DryRun: true})
	runner.capturePane = func(string, int) (string, error) { return "idle", nil }

	if err := runner.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), `"type":"action"`) || !strings.Contains(string(data), `"reason":"max_actions"`) {
		t.Fatalf("log = %s", data)
	}
}

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

func TestLoopAcceptsAllowlistedCodexModelCapacityRetry(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "loop.jsonl")
	runner := NewRunner(Config{
		Target:                 "demo:0.0",
		CaptureLines:           80,
		IdleAfter:              Duration{Duration: time.Second},
		PollInterval:           Duration{Duration: time.Second},
		StopOnPermissionPrompt: true,
		LogPath:                logPath,
		Actions:                []ActionConfig{{Name: "nudge", Type: "send_text", Text: "go"}},
	}, Options{Once: true})
	runner.capturePane = func(target string, lines int) (string, error) {
		return "response, though it may be less capable of handling complex requests.\n› 1. Retry with a faster model\n  2. Keep waiting\n  3. Learn more\nPress enter to confirm or esc to go back\n", nil
	}
	var sentKeys []string
	runner.sendKeys = func(target string, keys []string) error {
		sentKeys = append(sentKeys, keys...)
		return nil
	}
	runner.sendText = func(string, string, bool) error {
		t.Fatal("scheduled action must not run against the capacity prompt")
		return nil
	}

	if err := runner.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(sentKeys, []string{"Enter"}) {
		t.Fatalf("sent keys = %#v, want Enter", sentKeys)
	}

	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	var got event
	if err := json.Unmarshal(data[:len(data)-1], &got); err != nil {
		t.Fatal(err)
	}
	if got.Type != "prompt_accept" || got.Status != "ok" || got.Reason != "codex_model_capacity_retry" {
		t.Fatalf("event = %#v", got)
	}
}

func TestLoopStillStopsOnDirectoryPermissionPrompt(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "loop.jsonl")
	runner := NewRunner(Config{
		Target:                 "demo:0.0",
		CaptureLines:           80,
		IdleAfter:              Duration{Duration: time.Second},
		PollInterval:           Duration{Duration: time.Second},
		StopOnPermissionPrompt: true,
		LogPath:                logPath,
		Actions:                []ActionConfig{{Name: "nudge", Type: "send_text", Text: "go"}},
	}, Options{})
	runner.capturePane = func(target string, lines int) (string, error) {
		return "Allow directory access\n/private/tmp/project\nDo you want to allow this?\n› 1. Yes\n  2. No\n", nil
	}
	runner.sendKeys = func(string, []string) error {
		t.Fatal("filesystem permission prompt must not be answered")
		return nil
	}
	runner.sendText = func(string, string, bool) error {
		t.Fatal("scheduled action must not run against a permission prompt")
		return nil
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
	if !ok || details["type"] != prompt.TypeDirectoryAccess {
		t.Fatalf("details = %#v", got.Details)
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
