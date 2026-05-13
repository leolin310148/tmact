package workflow

import (
	"strings"
	"testing"
	"time"

	"tmact/internal/agents"
	"tmact/internal/prompt"
)

func TestDiscussionObserveStopsOnGenericPrompt(t *testing.T) {
	runner := &Runner{
		cfg: Config{Discussion: DiscussionConfig{CaptureLines: 80}},
		bindings: []RoleBinding{{
			Role:  "qa",
			Agent: agents.AgentConfig{Name: "qa-agent", Target: "demo:0.0"},
		}},
		now: time.Now,
		capturePane: func(target string, lines int) (string, error) {
			return "Allow command?\n  1. Yes\n❯ 2. No\n", nil
		},
	}

	_, err := runner.observeRolePanes(t.TempDir()+"/comments.jsonl", nil)
	if err == nil {
		t.Fatal("expected permission prompt error")
	}
	promptErr, ok := err.(PermissionPromptError)
	if !ok {
		t.Fatalf("error = %T %v", err, err)
	}
	if promptErr.Role != "qa" || promptErr.Prompt.Type != prompt.TypeCommandApproval {
		t.Fatalf("prompt error = %#v", promptErr)
	}
	if !strings.Contains(err.Error(), "command_approval") || !strings.Contains(err.Error(), "Allow command?") {
		t.Fatalf("error text = %q", err.Error())
	}
}

func TestImplementationObserveStopsOnPatchPrompt(t *testing.T) {
	runner := &ImplementationRunner{
		cfg: Config{Implementation: ImplementationConfig{CaptureLines: 80}},
		bindings: []RoleBinding{{
			Role:  "swe",
			Agent: agents.AgentConfig{Name: "swe-agent", Target: "demo:0.0"},
		}},
		now: time.Now,
		capturePane: func(target string, lines int) (string, error) {
			return "Apply this patch?\n  1. Yes\n❯ 2. No\n", nil
		},
	}

	_, err := runner.observeRolePanes(t.TempDir()+"/phase2-comments.jsonl", nil)
	if err == nil {
		t.Fatal("expected permission prompt error")
	}
	promptErr, ok := err.(PermissionPromptError)
	if !ok {
		t.Fatalf("error = %T %v", err, err)
	}
	if promptErr.Role != "swe" || promptErr.Prompt.Type != prompt.TypePatchApproval {
		t.Fatalf("prompt error = %#v", promptErr)
	}
	if !strings.Contains(err.Error(), "patch_approval") || !strings.Contains(err.Error(), "Apply this patch?") {
		t.Fatalf("error text = %q", err.Error())
	}
}
