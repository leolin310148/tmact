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
	if len(loaded.Config.Stages) != 24 {
		t.Fatalf("stages=%d", len(loaded.Config.Stages))
	}
	verificationArgv, ok := loaded.Variables["verification_argv"].([]string)
	if !ok || strings.Join(verificationArgv, ",") != "go,test,./..." || loaded.Variables["verification_cwd"] != "." {
		t.Fatalf("verification variables=%#v", loaded.Variables)
	}
	stages := map[string]workflow.StageConfig{}
	for _, stage := range loaded.Config.Stages {
		stages[stage.ID] = stage
		for outcome, disposition := range stage.Outcomes {
			if disposition == "retry" && len(stage.ProducesRevisions) == 0 {
				t.Fatalf("stage %s retries outcome %s without producing a revision", stage.ID, outcome)
			}
		}
	}
	for _, pair := range [][2]string{{"pm_review", "pm_revision"}, {"swe_review", "swe_revision"}, {"qa_review", "qa_revision"}, {"final_review", "final_revision"}} {
		review, revision := stages[pair[0]], stages[pair[1]]
		if review.Outcomes["request_changes"] != "success" {
			t.Fatalf("%s request_changes=%q", review.ID, review.Outcomes["request_changes"])
		}
		if revision.When == nil || revision.When.Stage == nil || revision.When.Stage.ID != review.ID || revision.When.Stage.Outcome != "request_changes" {
			t.Fatalf("%s remediation condition=%#v", revision.ID, revision.When)
		}
		if strings.Join(revision.ProducesRevisions, ",") != "spec" {
			t.Fatalf("%s produces=%v", revision.ID, revision.ProducesRevisions)
		}
	}
	if stages["final_confirmation"].Outcomes["request_changes"] != "blocked" {
		t.Fatalf("final confirmation outcomes=%v", stages["final_confirmation"].Outcomes)
	}
	if strings.Join(stages["apply"].Needs, ",") != "final_confirmation" {
		t.Fatalf("apply needs=%v", stages["apply"].Needs)
	}
	for _, id := range []string{"test_implementation", "test_repair"} {
		if stages[id].ArgvVariable != "verification_argv" || stages[id].Cwd != "{{ .vars.verification_cwd }}" {
			t.Fatalf("%s command config=%#v", id, stages[id])
		}
	}
	for _, id := range []string{"apply", "repair_implementation"} {
		if !strings.Contains(stages[id].Prompt, "Do not modify or archive any file under openspec/changes/") || !strings.Contains(stages[id].Prompt, "without editing tasks.md") {
			t.Fatalf("%s prompt does not protect approved OpenSpec artifacts: %q", id, stages[id].Prompt)
		}
	}
	if stages["qa_verify"].Outcomes["fail"] != "success" || stages["qa_confirmation"].Outcomes["fail"] != "blocked" {
		t.Fatalf("QA convergence outcomes verify=%v confirmation=%v", stages["qa_verify"].Outcomes, stages["qa_confirmation"].Outcomes)
	}
	if strings.Join(stages["repair_implementation"].ProducesRevisions, ",") != "source" {
		t.Fatalf("repair produces=%v", stages["repair_implementation"].ProducesRevisions)
	}
	archive := loaded.Config.Stages[len(loaded.Config.Stages)-1]
	if archive.ID != "archive" || strings.Join(archive.ProducesRevisions, ",") != "spec,source" {
		t.Fatalf("archive produces=%v", archive.ProducesRevisions)
	}
	overridden, err := workflow.Load(path, map[string]string{
		"change":           "demo",
		"verification_argv": `["make","test"]`,
		"verification_cwd":  "internal/web/frontend",
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(overridden.Variables["verification_argv"].([]string), ","); got != "make,test" {
		t.Fatalf("overridden verification_argv=%q", got)
	}
	if overridden.Variables["verification_cwd"] != "internal/web/frontend" {
		t.Fatalf("overridden verification_cwd=%v", overridden.Variables["verification_cwd"])
	}
}
func TestWorkflowExampleAndRemovedCommands(t *testing.T) {
	out, err := captureRun(t, "workflow", "example", "--profile", "openspec")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"version: 2", "produces_revisions", "archive_gate", "type: string_list", "argv_variable: verification_argv"} {
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
