package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/leolin310148/tmact/internal/workflow"
)

func TestWorkflowOpenSpecProfileIsStrictlyValid(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "workflow.yaml")
	if err := os.WriteFile(path, []byte(workflowOpenSpecProfileYAML), 0o644); err != nil {
		t.Fatal(err)
	}
	loaded, err := workflow.Load(path, map[string]string{"change": "demo"})
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Config.Stages) != 11 {
		t.Fatalf("stages=%d", len(loaded.Config.Stages))
	}
}
func TestWorkflowExampleAndRemovedCommands(t *testing.T) {
	out, err := captureRun(t, "workflow", "example", "--profile", "openspec")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"version: 2", "produces_revisions", "archive_gate"} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q", want)
		}
	}
	if err := run([]string{"workflow", "implement"}); err == nil || !strings.Contains(err.Error(), "unknown workflow subcommand") {
		t.Fatalf("error=%v", err)
	}
}

func TestWorkflowValidatePlanAndExecuteBoundary(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "workflow.yaml")
	body := `version: 2
workspace: {root: .}
variables:
  name: {type: string, required: true}
revisions:
  files: {files: {paths: [.]}}
defaults: {timeout: 5s}
stages:
  - id: touch
    type: command
    argv: [/usr/bin/touch, "{{ .vars.name }}"]
    produces_revisions: [files]
`
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	out, err := captureRun(t, "workflow", "validate", "--config", path, "--var", "name=result.txt", "--json")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, `"valid": true`) {
		t.Fatalf("out=%s", out)
	}
	if _, err := captureRun(t, "workflow", "run", "--config", path, "--var", "name=result.txt", "--once"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "result.txt")); !os.IsNotExist(err) {
		t.Fatalf("dry plan created file: %v", err)
	}
	if _, err := captureRun(t, "workflow", "run", "--config", path, "--var", "name=result.txt", "--once", "--execute"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "result.txt")); err != nil {
		t.Fatal(err)
	}
	status, err := captureRun(t, "workflow", "status", "--config", path, "--json")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(status, `"status": "succeeded"`) {
		t.Fatalf("status=%s", status)
	}
}

func TestWorkflowStopFinalizesRunnerWithoutLockDespiteStalePID(t *testing.T) {
	restore := stubCLIHooks(t)
	defer restore()
	dir := t.TempDir()
	path := filepath.Join(dir, "workflow.yaml")
	if err := os.WriteFile(path, []byte(`version: 2
workspace: {root: .}
stages:
  - {id: wait, type: human, outcomes: {done: success}}
`), 0o644); err != nil {
		t.Fatal(err)
	}
	loaded, err := workflow.Load(path, nil)
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(dir, "runs")
	engine, err := workflow.NewEngine(loaded, root, true)
	if err != nil {
		t.Fatal(err)
	}
	if err := engine.Store.Update(func(state *workflow.State) error {
		state.PID = 424242
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	out, err := captureRun(t, "workflow", "stop", "--id", engine.Store.RunID, "--store-dir", root, "--wait")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "runner not active") {
		t.Fatalf("out=%q", out)
	}
	state, err := engine.Store.Read()
	if err != nil {
		t.Fatal(err)
	}
	if state.Status != "stopped" || state.Desired != "stopped" || state.PID != 0 {
		t.Fatalf("state=%#v", state)
	}
}

func TestWorkflowStopDoesNotFinalizeWhileRunnerLockIsHeld(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "workflow.yaml")
	if err := os.WriteFile(path, []byte(`version: 2
workspace: {root: .}
stages:
  - {id: wait, type: human, outcomes: {done: success}}
`), 0o644); err != nil {
		t.Fatal(err)
	}
	loaded, err := workflow.Load(path, nil)
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(dir, "runs")
	engine, err := workflow.NewEngine(loaded, root, true)
	if err != nil {
		t.Fatal(err)
	}
	release, err := engine.Store.AcquireRunnerLock()
	if err != nil {
		t.Fatal(err)
	}
	defer release()

	_, err = captureRun(t, "workflow", "stop", "--id", engine.Store.RunID, "--store-dir", root, "--wait", "--timeout", "0s")
	if err == nil || !strings.Contains(err.Error(), "timed out waiting") {
		t.Fatalf("error=%v", err)
	}
	state, err := engine.Store.Read()
	if err != nil {
		t.Fatal(err)
	}
	if state.Status == "stopped" || state.Desired != "stopped" {
		t.Fatalf("state=%#v", state)
	}
}

func TestWorkflowStartUsesRunnerLockAsActiveSignal(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "workflow.yaml")
	if err := os.WriteFile(path, []byte(`version: 2
workspace: {root: .}
stages:
  - {id: wait, type: human, outcomes: {done: success}}
`), 0o644); err != nil {
		t.Fatal(err)
	}
	loaded, err := workflow.Load(path, nil)
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(dir, workflow.DefaultStoreDir)
	engine, err := workflow.NewEngine(loaded, root, true)
	if err != nil {
		t.Fatal(err)
	}
	release, err := engine.Store.AcquireRunnerLock()
	if err != nil {
		t.Fatal(err)
	}
	defer release()

	out, err := captureRun(t, "workflow", "start", "--config", path, "--execute")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "workflow already active") {
		t.Fatalf("out=%q", out)
	}
}

func TestWorkflowStartRejectsPendingStopRequest(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "workflow.yaml")
	if err := os.WriteFile(path, []byte(`version: 2
workspace: {root: .}
stages:
  - {id: wait, type: human, outcomes: {done: success}}
`), 0o644); err != nil {
		t.Fatal(err)
	}
	loaded, err := workflow.Load(path, nil)
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(dir, workflow.DefaultStoreDir)
	engine, err := workflow.NewEngine(loaded, root, true)
	if err != nil {
		t.Fatal(err)
	}
	if err := engine.Store.Update(func(state *workflow.State) error {
		state.Desired = "stopped"
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	_, err = captureRun(t, "workflow", "start", "--config", path, "--execute")
	if err == nil || !strings.Contains(err.Error(), "stop request") {
		t.Fatalf("error=%v", err)
	}
}
