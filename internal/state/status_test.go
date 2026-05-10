package state

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLoadMinimalStatus(t *testing.T) {
	path := writeStatus(t, "state: planning\n")

	status, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if status.State() != "planning" {
		t.Fatalf("state = %q", status.State())
	}
}

func TestSetPreservesUnknownFields(t *testing.T) {
	path := writeStatus(t, `
feature: source-fidelity
state: planning
nested:
  keep: true
`)
	updatedAt := time.Date(2026, 5, 8, 12, 0, 0, 0, time.UTC)
	cycle := 2

	data, _, err := Set(path, Update{
		State:     "implementation",
		Owner:     "swe",
		Stage:     "build",
		Cycle:     &cycle,
		UpdatedAt: updatedAt,
	})
	if err != nil {
		t.Fatal(err)
	}
	if data["feature"] != "source-fidelity" {
		t.Fatalf("feature not preserved: %#v", data["feature"])
	}
	nested, ok := data["nested"].(map[string]interface{})
	if !ok || nested["keep"] != true {
		t.Fatalf("nested not preserved: %#v", data["nested"])
	}
	if data["state"] != "implementation" {
		t.Fatalf("state = %#v", data["state"])
	}
	if data["updated_at"] != "2026-05-08T12:00:00Z" {
		t.Fatalf("updated_at = %#v", data["updated_at"])
	}
}

func TestTransitionRejectsMismatchedFrom(t *testing.T) {
	path := writeStatus(t, "state: review\n")

	_, _, err := Transition(path, "planning", Update{State: "fixing"})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "does not match") {
		t.Fatalf("error = %v", err)
	}
}

func TestTransitionUpdatesState(t *testing.T) {
	path := writeStatus(t, "state: review\n")
	updatedAt := time.Date(2026, 5, 8, 13, 0, 0, 0, time.UTC)

	data, _, err := Transition(path, "review", Update{State: "fixing", UpdatedAt: updatedAt})
	if err != nil {
		t.Fatal(err)
	}
	if data["state"] != "fixing" {
		t.Fatalf("state = %#v", data["state"])
	}
	if data["updated_at"] != "2026-05-08T13:00:00Z" {
		t.Fatalf("updated_at = %#v", data["updated_at"])
	}
}

func TestSetAppendsJSONLEvent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "status.yaml")

	_, event, err := Set(path, Update{State: "planning"})
	if err != nil {
		t.Fatal(err)
	}
	eventsPath := filepath.Join(filepath.Dir(path), "events.jsonl")
	file, err := os.Open(eventsPath)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	if !scanner.Scan() {
		t.Fatal("expected event line")
	}
	var decoded Event
	if err := json.Unmarshal(scanner.Bytes(), &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.Kind != "set" || decoded.State != "planning" || decoded.Path != path {
		t.Fatalf("event = %#v, returned = %#v", decoded, event)
	}
	if scanner.Scan() {
		t.Fatal("expected one event line")
	}
}

func TestWriteCreatesFinalFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "status.yaml")
	file := New(path)
	if err := file.Apply(Update{State: "planning"}); err != nil {
		t.Fatal(err)
	}
	if err := file.Write(); err != nil {
		t.Fatal(err)
	}

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(content), "state: planning") {
		t.Fatalf("content = %s", content)
	}
}

func writeStatus(t *testing.T, content string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "status.yaml")
	if err := os.WriteFile(path, []byte(strings.TrimLeft(content, "\n")), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}
