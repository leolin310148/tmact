package workflow

import (
	"testing"
	"time"
)

func TestEvaluateGateRequiresAllRolesCurrentHashAndValidation(t *testing.T) {
	roles := []string{"pm", "swe", "qa", "reviewer"}
	comments := []Comment{
		accepted("pm", "sha256:a"),
		accepted("swe", "sha256:a"),
		accepted("qa", "sha256:a"),
		accepted("reviewer", "sha256:a"),
	}
	result := EvaluateGate(roles, "sha256:a", &ValidationResult{ChangeHash: "sha256:a", Passed: true}, comments)
	if !result.Passed {
		t.Fatalf("gate should pass: %#v", result)
	}

	result = EvaluateGate(roles, "sha256:b", &ValidationResult{ChangeHash: "sha256:b", Passed: true}, comments)
	if result.Passed || len(result.PendingRoles) != 4 {
		t.Fatalf("stale gate should fail with all pending roles: %#v", result)
	}

	result = EvaluateGate(roles, "sha256:a", &ValidationResult{ChangeHash: "sha256:a", Passed: false}, comments)
	if result.Passed || !hasReason(result.Reasons, "validation_not_passed") {
		t.Fatalf("failed validation should block gate: %#v", result)
	}
}

func TestEvaluateGateBlocksUntilDecisionResolvesComment(t *testing.T) {
	roles := []string{"pm", "swe", "qa", "reviewer"}
	blocker := Comment{ID: "c-block", Role: "qa", Kind: "request_changes", ChangeHash: "sha256:a", Blocking: true, Timestamp: time.Now()}
	comments := []Comment{
		accepted("pm", "sha256:a"),
		accepted("swe", "sha256:a"),
		accepted("qa", "sha256:a"),
		accepted("reviewer", "sha256:a"),
		blocker,
	}
	result := EvaluateGate(roles, "sha256:a", &ValidationResult{ChangeHash: "sha256:a", Passed: true}, comments)
	if result.Passed || !hasReason(result.Reasons, "blocking_comments") {
		t.Fatalf("blocking comment should fail gate: %#v", result)
	}

	comments = append(comments,
		Comment{ID: "c-decision", Role: "reviewer", Kind: "decision", ChangeHash: "sha256:a", ReplyTo: "c-block"},
		accepted("qa", "sha256:a"),
	)
	result = EvaluateGate(roles, "sha256:a", &ValidationResult{ChangeHash: "sha256:a", Passed: true}, comments)
	if !result.Passed {
		t.Fatalf("decision should resolve blocker: %#v", result)
	}
}

func accepted(role string, hash string) Comment {
	return Comment{ID: "c-" + role, Role: role, Kind: "accept", ChangeHash: hash, OpenSpecValid: true, Timestamp: time.Now()}
}

func hasReason(reasons []string, want string) bool {
	for _, reason := range reasons {
		if reason == want {
			return true
		}
	}
	return false
}
