package main

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
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
