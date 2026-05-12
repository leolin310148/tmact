package main

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"tmact/internal/runmeta"
	"tmact/internal/tmux"
)

func TestStateGetPrintsTextAndJSON(t *testing.T) {
	path := writeCLIStatus(t, "state: planning\nfeature: demo\n")

	text, err := captureRun(t, "state", "get", "--path", path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(text, "state: planning") || !strings.Contains(text, "feature: demo") {
		t.Fatalf("text output = %q", text)
	}

	out, err := captureRun(t, "state", "get", "--path", path, "--json")
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]interface{}
	if err := json.Unmarshal([]byte(out), &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded["state"] != "planning" || decoded["feature"] != "demo" {
		t.Fatalf("json output = %#v", decoded)
	}
}

func TestStateSetCreatesStatusFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "status.yaml")

	out, err := captureRun(t, "state", "set", "--path", path, "--state", "implementation", "--owner", "swe", "--cycle", "3", "--blocker", "waiting")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "state: implementation") {
		t.Fatalf("output = %q", out)
	}

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"state: implementation", "owner: swe", "cycle: 3", "waiting"} {
		if !strings.Contains(string(content), want) {
			t.Fatalf("content missing %q: %s", want, content)
		}
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(path), "events.jsonl")); err != nil {
		t.Fatal(err)
	}
}

func TestStateTransitionRequiresCurrentState(t *testing.T) {
	path := writeCLIStatus(t, "state: review\n")

	if _, err := captureRun(t, "state", "transition", "--path", path, "--from", "review", "--to", "fixing"); err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(content), "state: fixing") {
		t.Fatalf("content = %s", content)
	}

	if _, err := captureRun(t, "state", "transition", "--path", path, "--from", "review", "--to", "done"); err == nil {
		t.Fatal("expected transition error")
	}
}

func TestStateEventAppendsEvent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "status.yaml")

	out, err := captureRun(t, "state", "event", "--path", path, "--kind", "note", "--agent", "planner", "--message", "handoff written", "--json")
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]interface{}
	if err := json.Unmarshal([]byte(out), &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded["kind"] != "note" || decoded["agent"] != "planner" {
		t.Fatalf("json output = %#v", decoded)
	}
	content, err := os.ReadFile(filepath.Join(filepath.Dir(path), "events.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(content), `"message":"handoff written"`) {
		t.Fatalf("events = %s", content)
	}
}

func TestStateCommandValidation(t *testing.T) {
	path := writeCLIStatus(t, "state: review\n")
	tests := [][]string{
		{"state", "get"},
		{"state", "set", "--path", path},
		{"state", "set", "--path", path, "--state", "done", "--cycle", "-1"},
		{"state", "transition", "--path", path, "--from", "planning", "--to", "done"},
	}

	for _, args := range tests {
		if _, err := captureRun(t, args...); err == nil {
			t.Fatalf("expected error for %v", args)
		}
	}
}

func TestListPrintsAndCachesNumberedTargets(t *testing.T) {
	t.Chdir(t.TempDir())
	resetCLIHooks := stubCLIHooks(t)
	defer resetCLIHooks()

	listAllTmuxPanes = func() ([]tmux.Pane, error) {
		return []tmux.Pane{{
			Session:        "IDLL",
			WindowIndex:    1,
			WindowName:     "roadmap-codex",
			PaneIndex:      0,
			PaneID:         "%42",
			CurrentCommand: "codex",
			CurrentPath:    "/repo",
			Active:         true,
		}}, nil
	}
	tmactNow = func() time.Time { return time.Date(2026, 5, 11, 9, 30, 0, 0, time.UTC) }

	out, err := captureRun(t, "ls")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"#", "target", "0", "%42", "IDLL", "1:roadmap-codex", "codex", "/repo"} {
		if !strings.Contains(out, want) {
			t.Fatalf("ls output missing %q: %s", want, out)
		}
	}

	data, err := os.ReadFile(filepath.Join(".cache", "tmact-targets.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"target": "%42"`) {
		t.Fatalf("cache = %s", data)
	}
}

func TestSendDryRunResolvesNumberedTarget(t *testing.T) {
	t.Chdir(t.TempDir())
	resetCLIHooks := stubCLIHooks(t)
	defer resetCLIHooks()

	tmactNow = func() time.Time { return time.Date(2026, 5, 11, 9, 30, 0, 0, time.UTC) }
	if err := writeTargetCache(targetCache{
		GeneratedAt: tmactNow(),
		Panes: []listPaneRow{{
			Index:  0,
			Target: "%42",
		}},
	}); err != nil {
		t.Fatal(err)
	}
	listTargetTmuxPanes = func(target string) ([]tmux.Pane, error) {
		if target != "%42" {
			t.Fatalf("target = %q", target)
		}
		return []tmux.Pane{{PaneID: "%42"}}, nil
	}
	pasteTmuxText = func(string, string, bool) error {
		t.Fatal("dry-run should not paste")
		return nil
	}

	out, err := captureRun(t, "-t", "0", "send", "--command", "go test ./...")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "dry-run: would send command to %42: go test ./...") {
		t.Fatalf("output = %q", out)
	}
}

func TestSendExecuteCommandCanClearLine(t *testing.T) {
	resetCLIHooks := stubCLIHooks(t)
	defer resetCLIHooks()

	var sentKeys [][]string
	var pastedTarget, pastedText string
	var pastedEnter bool
	sendTmuxKeys = func(_ string, keys []string) error {
		sentKeys = append(sentKeys, append([]string(nil), keys...))
		return nil
	}
	pasteTmuxText = func(target string, text string, enter bool) error {
		pastedTarget = target
		pastedText = text
		pastedEnter = enter
		return nil
	}

	out, err := captureRun(t, "-t", "%42", "send", "--clear-line", "--command", "go test ./...", "--execute")
	if err != nil {
		t.Fatal(err)
	}
	if len(sentKeys) != 1 || strings.Join(sentKeys[0], ",") != "C-u" {
		t.Fatalf("sent keys = %#v", sentKeys)
	}
	if pastedTarget != "%42" || pastedText != "go test ./..." || !pastedEnter {
		t.Fatalf("pasted target=%q text=%q enter=%t", pastedTarget, pastedText, pastedEnter)
	}
	if !strings.Contains(out, "clear line and send command to %42: go test ./...") {
		t.Fatalf("output = %q", out)
	}
}

func TestSendValidation(t *testing.T) {
	tests := [][]string{
		{"send", "--command", "go test ./..."},
		{"-t", "%42", "send"},
		{"-t", "%42", "send", "--text", "hi", "--command", "go test ./..."},
		{"-t", "%42", "send", "--key", "Enter", "--enter"},
		{"-t", "%42", "send", "--keys", "C-u,"},
	}
	for _, args := range tests {
		if _, err := captureRun(t, args...); err == nil {
			t.Fatalf("expected error for %v", args)
		}
	}
}

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

func captureRun(t *testing.T, args ...string) (string, error) {
	t.Helper()

	oldStdout := os.Stdout
	read, write, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = write
	err = run(args)
	if closeErr := write.Close(); closeErr != nil && err == nil {
		err = closeErr
	}
	os.Stdout = oldStdout

	output, readErr := io.ReadAll(read)
	if readErr != nil && err == nil {
		err = readErr
	}
	return string(output), err
}

func writeCLIStatus(t *testing.T, content string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "status.yaml")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func stubCLIHooks(t *testing.T) func() {
	t.Helper()

	oldListAllTmuxPanes := listAllTmuxPanes
	oldListTargetTmuxPanes := listTargetTmuxPanes
	oldPasteTmuxText := pasteTmuxText
	oldSendTmuxKeys := sendTmuxKeys
	oldTmactNow := tmactNow

	return func() {
		listAllTmuxPanes = oldListAllTmuxPanes
		listTargetTmuxPanes = oldListTargetTmuxPanes
		pasteTmuxText = oldPasteTmuxText
		sendTmuxKeys = oldSendTmuxKeys
		tmactNow = oldTmactNow
	}
}
