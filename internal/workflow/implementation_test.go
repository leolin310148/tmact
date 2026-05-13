package workflow

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"tmact/internal/agents"
)

func TestLoadImplementationConfigDefaultsAndRequiredRoles(t *testing.T) {
	t.Chdir(t.TempDir())
	if err := os.MkdirAll(filepath.Join("openspec", "changes", "demo"), 0o755); err != nil {
		t.Fatal(err)
	}
	path := "implementation.yaml"
	data := []byte(`change: demo
agents_config: agents.yaml
roles:
  pm: pm-agent
  swe: swe-agent
  qa: qa-agent
`)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadImplementationConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(cfg.Implementation.StageOrder, ",") != "swe_apply,qa_verify,pm_archive" {
		t.Fatalf("stage order = %#v", cfg.Implementation.StageOrder)
	}
	if cfg.Implementation.MaxTurns != 12 || cfg.Implementation.CaptureLines != 180 {
		t.Fatalf("defaults not applied: %#v", cfg.Implementation)
	}
	if cfg.Implementation.RequirePhase1Agreed == nil || !*cfg.Implementation.RequirePhase1Agreed {
		t.Fatalf("require_phase1_agreed default not applied")
	}
}

func TestLoadImplementationConfigRejectsMissingCommand(t *testing.T) {
	t.Chdir(t.TempDir())
	if err := os.MkdirAll(filepath.Join("openspec", "changes", "demo"), 0o755); err != nil {
		t.Fatal(err)
	}
	path := "implementation.yaml"
	data := []byte(`change: demo
agents_config: agents.yaml
roles:
  pm: pm-agent
  swe: swe-agent
  qa: qa-agent
implementation:
  apply_instructions:
    args: ["instructions", "apply"]
`)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadImplementationConfig(path); err == nil {
		t.Fatal("expected missing apply command to fail")
	}
}

func TestParseImplementationCommentMarker(t *testing.T) {
	now := time.Date(2026, 5, 13, 12, 0, 0, 0, time.UTC)
	line := `TMAct-OpenSpec-Phase2: role=qa stage=verify kind=pass change_hash=sha256:abc blocking=false body="tests passed"`
	comment, ok, err := ParseImplementationCommentLine(line, now)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("marker not found")
	}
	if comment.Role != "qa" || comment.Stage != "verify" || comment.Kind != "pass" || comment.ChangeHash != "sha256:abc" || comment.Blocking {
		t.Fatalf("comment = %#v", comment)
	}
	if comment.Body != "tests passed" || comment.ID == "" {
		t.Fatalf("comment details = %#v", comment)
	}
}

func TestEvaluateImplementationGateOrdersStagesAndBlocksArchive(t *testing.T) {
	stages := []string{"swe_apply", "qa_verify", "pm_archive"}
	validation := &ValidationResult{ChangeHash: "sha256:a", Passed: true}

	result := EvaluateImplementationGate(stages, "sha256:a", validation, nil)
	if result.Passed || result.PendingStage != "swe_apply" || result.PendingRole != "swe" {
		t.Fatalf("expected swe apply pending: %#v", result)
	}

	comments := []ImplementationComment{
		phase2("swe", "apply", "complete", "sha256:a"),
	}
	result = EvaluateImplementationGate(stages, "sha256:a", validation, comments)
	if result.Passed || result.PendingStage != "qa_verify" || result.PendingRole != "qa" {
		t.Fatalf("expected qa verify pending: %#v", result)
	}

	comments = append(comments, phase2("qa", "verify", "fail", "sha256:a"))
	result = EvaluateImplementationGate(stages, "sha256:a", validation, comments)
	if result.Passed || result.PendingStage != "" || !hasReason(result.Reasons, "qa_failed") {
		t.Fatalf("qa failure should block archive: %#v", result)
	}
}

func TestEvaluateImplementationGateRequiresValidationBeforeArchive(t *testing.T) {
	stages := []string{"swe_apply", "qa_verify", "pm_archive"}
	comments := []ImplementationComment{
		phase2("swe", "apply", "complete", "sha256:a"),
		phase2("qa", "verify", "pass", "sha256:a"),
	}
	result := EvaluateImplementationGate(stages, "sha256:a", &ValidationResult{ChangeHash: "sha256:a", Passed: false}, comments)
	if result.Passed || result.PendingStage != "" || !hasReason(result.Reasons, "validation_not_passed") {
		t.Fatalf("failed validation should block archive prompt: %#v", result)
	}

	result = EvaluateImplementationGate(stages, "sha256:a", &ValidationResult{ChangeHash: "sha256:a", Passed: true}, comments)
	if result.Passed || result.PendingStage != "pm_archive" || result.PendingRole != "pm" {
		t.Fatalf("expected pm archive pending: %#v", result)
	}

	comments = append(comments, phase2("pm", "archive", "complete", "sha256:a"))
	result = EvaluateImplementationGate(stages, "sha256:a", &ValidationResult{ChangeHash: "sha256:a", Passed: true}, comments)
	if !result.Passed {
		t.Fatalf("archive complete should pass: %#v", result)
	}
}

func TestImplementationPreconditionRequiresAgreedPhase1Hash(t *testing.T) {
	t.Chdir(t.TempDir())
	changeDir := filepath.Join("openspec", "changes", "demo")
	if err := os.MkdirAll(filepath.Join(changeDir, "specs", "x"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(changeDir, "proposal.md"), []byte("# Proposal\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(changeDir, "specs", "x", "spec.md"), []byte("## ADDED Requirements\n\n### Requirement: X\n\n#### Scenario: Y\n\n- GIVEN x\n- WHEN y\n- THEN z\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	hash, _, err := HashChangeDir(changeDir)
	if err != nil {
		t.Fatal(err)
	}
	cfg := Config{
		Change: "demo",
		Roles:  map[string]string{"swe": "swe-agent", "qa": "qa-agent", "pm": "pm-agent"},
		Implementation: ImplementationConfig{
			StageOrder:          []string{"swe_apply", "qa_verify", "pm_archive"},
			RequirePhase1Agreed: boolPtr(true),
		},
	}
	agentCfg := agents.Config{Agents: []agents.AgentConfig{
		{Name: "swe-agent", Target: "demo:swe.0"},
		{Name: "qa-agent", Target: "demo:qa.0"},
		{Name: "pm-agent", Target: "demo:pm.0"},
	}}
	runner, err := NewImplementationRunner(cfg, agentCfg, Options{DryRun: true})
	if err != nil {
		t.Fatal(err)
	}
	_, current, _, reason, err := runner.checkPreconditions(changeDir)
	if err != nil {
		t.Fatal(err)
	}
	if current != hash || reason != "phase1_not_agreed" {
		t.Fatalf("precondition current=%q reason=%q", current, reason)
	}

	if err := WriteState(StatePath(changeDir), State{Change: "demo", Outcome: "agreed", ChangeHash: hash}); err != nil {
		t.Fatal(err)
	}
	accepted, current, _, reason, err := runner.checkPreconditions(changeDir)
	if err != nil {
		t.Fatal(err)
	}
	if accepted != hash || current != hash || reason != "" {
		t.Fatalf("precondition accepted=%q current=%q reason=%q", accepted, current, reason)
	}

	if err := os.WriteFile(filepath.Join(changeDir, "proposal.md"), []byte("# Proposal changed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	accepted, current, _, reason, err = runner.checkPreconditions(changeDir)
	if err != nil {
		t.Fatal(err)
	}
	if accepted != hash || current == hash || reason != "accepted_hash_mismatch" {
		t.Fatalf("precondition mismatch accepted=%q current=%q reason=%q", accepted, current, reason)
	}
}

func phase2(role string, stage string, kind string, hash string) ImplementationComment {
	return ImplementationComment{ID: "p2-" + role + "-" + stage + "-" + kind, Role: role, Stage: stage, Kind: kind, ChangeHash: hash, Timestamp: time.Now()}
}

func boolPtr(value bool) *bool {
	return &value
}
