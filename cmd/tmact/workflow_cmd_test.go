package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/leolin310148/tmact/internal/workflow"
)

func TestWorkflowStatusPrintsLocalState(t *testing.T) {
	t.Chdir(t.TempDir())
	changeDir := filepath.Join("openspec", "changes", "demo")
	if err := os.MkdirAll(changeDir, 0o755); err != nil {
		t.Fatal(err)
	}
	configPath := "workflow.yaml"
	if err := os.WriteFile(configPath, []byte(`change: demo
agents_config: agents.yaml
roles:
  pm: pm-agent
  swe: swe-agent
  qa: qa-agent
  reviewer: reviewer-agent
`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := workflow.WriteState(workflow.StatePath(changeDir), workflow.State{
		Change:      "demo",
		Status:      "running",
		Phase:       "review",
		Turn:        2,
		PendingRole: "qa",
		ChangeHash:  "sha256:abc",
		Gate:        workflow.GateResult{Reasons: []string{"missing_agreement"}},
	}); err != nil {
		t.Fatal(err)
	}

	out, err := captureRun(t, "workflow", "status", "--config", configPath, "--run-dir", filepath.Join(t.TempDir(), "runs"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"workflow_state: demo", "pending_role: qa", "change_hash: sha256:abc", "gate_reasons: missing_agreement"} {
		if !strings.Contains(out, want) {
			t.Fatalf("workflow status missing %q: %s", want, out)
		}
	}
}

func TestWorkflowStatusPrintsImplementationState(t *testing.T) {
	t.Chdir(t.TempDir())
	changeDir := filepath.Join("openspec", "changes", "demo")
	if err := os.MkdirAll(changeDir, 0o755); err != nil {
		t.Fatal(err)
	}
	configPath := "implementation.yaml"
	if err := os.WriteFile(configPath, []byte(`change: demo
agents_config: agents.yaml
roles:
  pm: pm-agent
  swe: swe-agent
  qa: qa-agent
`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := workflow.WriteImplementationState(workflow.Phase2StatePath(changeDir), workflow.ImplementationState{
		Change:             "demo",
		Status:             "running",
		Phase:              "implementation",
		Turn:               1,
		PendingStage:       "qa_verify",
		PendingRole:        "qa",
		AcceptedChangeHash: "sha256:abc",
		Gate:               workflow.ImplementationGateResult{Reasons: []string{"missing_verify"}},
	}); err != nil {
		t.Fatal(err)
	}

	out, err := captureRun(t, "workflow", "status", "--config", configPath, "--run-dir", filepath.Join(t.TempDir(), "runs"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"implementation_state: demo", "pending_stage: qa_verify", "accepted_change_hash: sha256:abc", "gate_reasons: missing_verify"} {
		if !strings.Contains(out, want) {
			t.Fatalf("workflow status missing %q: %s", want, out)
		}
	}
}

func TestWorkflowExamplePrintsCombinedYAML(t *testing.T) {
	out, err := captureRun(t, "workflow", "example")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"change: your-change-id", "prompt_dispatch:", "clear_before_prompt: true", "discussion:", "implementation:", "stage_order: [swe_apply, qa_verify, pm_archive]", `args: ["archive", "{{change}}", "--yes"]`} {
		if !strings.Contains(out, want) {
			t.Fatalf("workflow example missing %q: %s", want, out)
		}
	}
}

func TestWorkflowReportReviewCLIWritesComment(t *testing.T) {
	t.Chdir(t.TempDir())
	changeDir := filepath.Join("openspec", "changes", "demo")
	if err := os.MkdirAll(changeDir, 0o755); err != nil {
		t.Fatal(err)
	}
	configPath := "workflow.yaml"
	if err := os.WriteFile(configPath, []byte(`change: demo
agents_config: agents.yaml
roles:
  pm: pm-agent
  swe: swe-agent
  qa: qa-agent
  reviewer: reviewer-agent
`), 0o644); err != nil {
		t.Fatal(err)
	}
	tmactNow = func() time.Time { return time.Date(2026, 5, 13, 12, 0, 0, 0, time.UTC) }
	out, err := captureRun(t, "workflow", "report", "review", "--config", configPath, "--role", "qa", "--kind", "accept", "--change-hash", "sha256:abc", "--openspec-valid", "--body", "accepted")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "review_report: c-") {
		t.Fatalf("output = %s", out)
	}
	comments, err := workflow.LoadComments(workflow.CommentsPath(changeDir))
	if err != nil {
		t.Fatal(err)
	}
	if len(comments) != 1 || comments[0].Role != "qa" || !comments[0].OpenSpecValid {
		t.Fatalf("comments = %#v", comments)
	}
}

func TestWorkflowReportImplementationCLIWritesComment(t *testing.T) {
	t.Chdir(t.TempDir())
	changeDir := filepath.Join("openspec", "changes", "demo")
	if err := os.MkdirAll(changeDir, 0o755); err != nil {
		t.Fatal(err)
	}
	configPath := "workflow.yaml"
	if err := os.WriteFile(configPath, []byte(`change: demo
agents_config: agents.yaml
roles:
  pm: pm-agent
  swe: swe-agent
  qa: qa-agent
`), 0o644); err != nil {
		t.Fatal(err)
	}
	tmactNow = func() time.Time { return time.Date(2026, 5, 13, 12, 0, 0, 0, time.UTC) }
	out, err := captureRun(t, "workflow", "report", "implementation", "--config", configPath, "--role", "qa", "--stage", "verify", "--kind", "pass", "--change-hash", "sha256:abc", "--body", "tests passed")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "implementation_report: p2-") {
		t.Fatalf("output = %s", out)
	}
	comments, err := workflow.LoadImplementationComments(workflow.Phase2CommentsPath(changeDir))
	if err != nil {
		t.Fatal(err)
	}
	if len(comments) != 1 || comments[0].Role != "qa" || comments[0].Stage != "verify" || comments[0].Kind != "pass" {
		t.Fatalf("comments = %#v", comments)
	}
}
