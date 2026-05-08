package agents

import (
	"testing"

	"tmact/internal/prompt"
)

func TestInboxFromReportIncludesPermissionPrompt(t *testing.T) {
	report := Report{
		Timestamp: "now",
		Agents: []AgentStatus{
			{
				Name:   "sample",
				Target: "sample:0.0",
				State:  StateWaitingPermission,
				Prompt: &prompt.DirectoryAccess{
					Title:    "Allow directory access",
					Question: "Do you want to allow this?",
					Paths:    []string{"/tmp/sample"},
					SelectedOption: &prompt.Option{
						Number: 2,
						Label:  "Yes",
					},
				},
			},
		},
	}

	inbox := InboxFromReport(report)
	if len(inbox.Items) != 1 {
		t.Fatalf("items = %d", len(inbox.Items))
	}
	if inbox.Items[0].Kind != "permission_prompt" {
		t.Fatalf("kind = %q", inbox.Items[0].Kind)
	}
	if inbox.Items[0].Severity != "action_required" {
		t.Fatalf("severity = %q", inbox.Items[0].Severity)
	}
}

func TestInboxFromReportSkipsWorkingAgent(t *testing.T) {
	report := Report{
		Agents: []AgentStatus{
			{Name: "sample", Target: "sample:0.0", State: StateWorking},
		},
	}

	inbox := InboxFromReport(report)
	if len(inbox.Items) != 0 {
		t.Fatalf("items = %d", len(inbox.Items))
	}
}

func TestInboxFromReportIncludesPaneError(t *testing.T) {
	report := Report{
		Agents: []AgentStatus{
			{Name: "sample", Target: "missing:0.0", State: StateBlocked, Error: "tmux capture-pane failed"},
		},
	}

	inbox := InboxFromReport(report)
	if len(inbox.Items) != 1 {
		t.Fatalf("items = %d", len(inbox.Items))
	}
	if inbox.Items[0].Kind != "pane_error" {
		t.Fatalf("kind = %q", inbox.Items[0].Kind)
	}
	if inbox.Items[0].Severity != "critical" {
		t.Fatalf("severity = %q", inbox.Items[0].Severity)
	}
}

func TestInboxFromReportIncludesRepoError(t *testing.T) {
	report := Report{
		Agents: []AgentStatus{
			{
				Name:   "sample",
				Target: "sample:0.0",
				State:  StateIdle,
				Git:    &GitStatus{Error: "not a git repository"},
			},
		},
	}

	inbox := InboxFromReport(report)
	if len(inbox.Items) != 1 {
		t.Fatalf("items = %d", len(inbox.Items))
	}
	if inbox.Items[0].Kind != "repo_error" {
		t.Fatalf("kind = %q", inbox.Items[0].Kind)
	}
}
