package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/leolin310148/tmact/internal/runmeta"
	"github.com/leolin310148/tmact/internal/tmux"
)

func writeLoopConfig(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "managed-loop.yaml")
	data := "target: demo:0.0\nlog_path: loop.jsonl\nactions:\n  - type: send_text\n    text: continue\n"
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoopStartCreatesDetachedManagedRun(t *testing.T) {
	restore := stubCLIHooks(t)
	defer restore()

	configPath := writeLoopConfig(t)
	runDir := t.TempDir()
	var gotSession, gotWindow, gotCWD string
	var gotCommand []string
	listSessionTmuxPanes = func(string) ([]tmux.Pane, error) {
		return nil, errors.New("no such session")
	}
	tmactExecutable = func() (string, error) { return "/tmp/tmact test", nil }
	newTmuxSession = func(session, window, cwd string, command []string) error {
		gotSession, gotWindow, gotCWD = session, window, cwd
		gotCommand = append([]string(nil), command...)
		_, err := runmeta.Register(runDir, runmeta.RegisterOptions{
			Kind:       "loop",
			ConfigPath: configPath,
			Target:     "demo:0.0",
			Now:        time.Now(),
		})
		return err
	}
	newTmuxWindow = func(string, string, string, []string) error {
		t.Fatal("new window should not be used when the supervisor session is absent")
		return nil
	}

	out, err := captureRun(t, "loop", "start", "--config", configPath, "--run-dir", runDir, "--timeout", "1s")
	if err != nil {
		t.Fatal(err)
	}
	if gotSession != loopSupervisorSession || gotWindow != "managed-loop" || gotCWD == "" {
		t.Fatalf("launch session=%q window=%q cwd=%q", gotSession, gotWindow, gotCWD)
	}
	joined := strings.Join(gotCommand, " ")
	for _, want := range []string{"/tmp/tmact test", "loop run", "--config", configPath, "--run-dir", runDir} {
		if !strings.Contains(joined, want) {
			t.Fatalf("command missing %q: %#v", want, gotCommand)
		}
	}
	if !strings.Contains(out, "started loop") || !strings.Contains(out, loopSupervisorSession) {
		t.Fatalf("output = %s", out)
	}
}

func TestLoopStartIsIdempotentPerConfig(t *testing.T) {
	restore := stubCLIHooks(t)
	defer restore()

	configPath := writeLoopConfig(t)
	runDir := t.TempDir()
	run, err := runmeta.Register(runDir, runmeta.RegisterOptions{
		Kind:       "loop",
		ConfigPath: configPath,
		Target:     "demo:0.0",
		Now:        time.Now(),
	})
	if err != nil {
		t.Fatal(err)
	}
	newTmuxSession = func(string, string, string, []string) error {
		t.Fatal("idempotent start must not create a session")
		return nil
	}
	newTmuxWindow = func(string, string, string, []string) error {
		t.Fatal("idempotent start must not create a window")
		return nil
	}

	out, err := captureRun(t, "loop", "start", "--config", configPath, "--run-dir", runDir)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "already active") || !strings.Contains(out, run.ID) {
		t.Fatalf("output = %s", out)
	}
}

func TestLoopRestartPreservesPreviousDryRunMode(t *testing.T) {
	restore := stubCLIHooks(t)
	defer restore()

	configPath := writeLoopConfig(t)
	runDir := t.TempDir()
	previous, err := runmeta.Register(runDir, runmeta.RegisterOptions{
		Kind:       "loop",
		ConfigPath: configPath,
		Target:     "demo:0.0",
		DryRun:     true,
		Now:        time.Now().Add(-time.Minute),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := runmeta.Finish(runDir, previous, "stopped", "requested", time.Now().Add(-time.Second)); err != nil {
		t.Fatal(err)
	}
	listSessionTmuxPanes = func(string) ([]tmux.Pane, error) { return nil, errors.New("missing") }
	tmactExecutable = func() (string, error) { return "/tmp/tmact", nil }
	var gotCommand []string
	newTmuxSession = func(_, _, _ string, command []string) error {
		gotCommand = append([]string(nil), command...)
		_, err := runmeta.Register(runDir, runmeta.RegisterOptions{
			Kind:       "loop",
			ConfigPath: configPath,
			Target:     "demo:0.0",
			DryRun:     true,
			Now:        time.Now(),
		})
		return err
	}

	if _, err := captureRun(t, "loop", "restart", "--config", configPath, "--run-dir", runDir, "--timeout", "1s"); err != nil {
		t.Fatal(err)
	}
	if !containsString(gotCommand, "--dry-run") {
		t.Fatalf("restart command did not preserve dry-run: %#v", gotCommand)
	}
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func TestLoopStopNoWaitWritesCooperativeControl(t *testing.T) {
	configPath := writeLoopConfig(t)
	runDir := t.TempDir()
	run, err := runmeta.Register(runDir, runmeta.RegisterOptions{
		Kind:       "loop",
		ConfigPath: configPath,
		Target:     "demo:0.0",
		Now:        time.Now(),
	})
	if err != nil {
		t.Fatal(err)
	}

	out, err := captureRun(t, "loop", "stop", "--id", run.ID, "--run-dir", runDir, "--no-wait")
	if err != nil {
		t.Fatal(err)
	}
	control, err := runmeta.ReadControl(runDir, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if control.DesiredState != runmeta.DesiredStopped || !strings.Contains(out, "requested stop") {
		t.Fatalf("control=%#v output=%q", control, out)
	}
}

func TestLoopStopAcceptsPositionalID(t *testing.T) {
	configPath := writeLoopConfig(t)
	runDir := t.TempDir()
	run, err := runmeta.Register(runDir, runmeta.RegisterOptions{
		Kind:       "loop",
		ConfigPath: configPath,
		Target:     "demo:0.0",
		Now:        time.Now(),
	})
	if err != nil {
		t.Fatal(err)
	}

	out, err := captureRun(t, "loop", "stop", run.ID, "--run-dir", runDir, "--no-wait")
	if err != nil {
		t.Fatal(err)
	}
	control, err := runmeta.ReadControl(runDir, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if control.DesiredState != runmeta.DesiredStopped || !strings.Contains(out, run.ID) {
		t.Fatalf("control=%#v output=%q", control, out)
	}
}

func TestLoopListShowsActiveIDsByDefault(t *testing.T) {
	runDir := t.TempDir()
	now := time.Now()
	active, err := runmeta.Register(runDir, runmeta.RegisterOptions{
		Kind:       "loop",
		ConfigPath: filepath.Join(t.TempDir(), "active.yaml"),
		Target:     "active:0.0",
		Now:        now,
	})
	if err != nil {
		t.Fatal(err)
	}
	stopped, err := runmeta.Register(runDir, runmeta.RegisterOptions{
		Kind:       "loop",
		ConfigPath: filepath.Join(t.TempDir(), "stopped.yaml"),
		Target:     "stopped:0.0",
		Now:        now.Add(-time.Minute),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := runmeta.Finish(runDir, stopped, "stopped", "requested", now); err != nil {
		t.Fatal(err)
	}

	out, err := captureRun(t, "loop", "list", "--run-dir", runDir)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "id") || !strings.Contains(out, active.ID) {
		t.Fatalf("active loop id missing from output: %s", out)
	}
	if strings.Contains(out, stopped.ID) {
		t.Fatalf("stopped loop should be hidden without --all: %s", out)
	}

	out, err = captureRun(t, "loop", "list", "--run-dir", runDir, "--all")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, active.ID) || !strings.Contains(out, stopped.ID) {
		t.Fatalf("--all output missing loop ids: %s", out)
	}
}

func TestLoopListJSONUsesEmptyArray(t *testing.T) {
	out, err := captureRun(t, "loop", "list", "--run-dir", t.TempDir(), "--json")
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(out) != "[]" {
		t.Fatalf("output = %q", out)
	}
}

func TestLoopValidateJSON(t *testing.T) {
	configPath := writeLoopConfig(t)
	out, err := captureRun(t, "loop", "validate", "--config", configPath, "--json")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"valid": true`, `"target": "demo:0.0"`, `"actions": 1`} {
		if !strings.Contains(out, want) {
			t.Fatalf("output missing %q: %s", want, out)
		}
	}
}
