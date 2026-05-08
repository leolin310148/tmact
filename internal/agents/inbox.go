package agents

type Inbox struct {
	Timestamp string      `json:"ts"`
	Items     []InboxItem `json:"items"`
}

type InboxItem struct {
	Agent    string                 `json:"agent"`
	Target   string                 `json:"target"`
	Repo     string                 `json:"repo,omitempty"`
	Kind     string                 `json:"kind"`
	State    string                 `json:"state"`
	Severity string                 `json:"severity"`
	Summary  string                 `json:"summary"`
	Details  map[string]interface{} `json:"details,omitempty"`
}

func InboxFromReport(report Report) Inbox {
	inbox := Inbox{
		Timestamp: report.Timestamp,
		Items:     []InboxItem{},
	}
	for _, agent := range report.Agents {
		if item, ok := inboxItemForAgent(agent); ok {
			inbox.Items = append(inbox.Items, item)
		}
	}
	return inbox
}

func inboxItemForAgent(agent AgentStatus) (InboxItem, bool) {
	base := InboxItem{
		Agent:  agent.Name,
		Target: agent.Target,
		Repo:   agent.Repo,
		State:  agent.State,
	}

	if agent.Error != "" {
		base.Kind = "pane_error"
		base.Severity = "critical"
		base.Summary = agent.Error
		return base, true
	}
	if agent.Git != nil && agent.Git.Error != "" {
		base.Kind = "repo_error"
		base.Severity = "warning"
		base.Summary = agent.Git.Error
		return base, true
	}

	switch agent.State {
	case StateWaitingPermission:
		base.Kind = "permission_prompt"
		base.Severity = "action_required"
		base.Summary = "permission prompt is waiting for a decision"
		if agent.Prompt != nil {
			base.Details = promptInboxDetails(agent)
		}
		return base, true
	case StateBlocked:
		base.Kind = "blocked"
		base.Severity = "action_required"
		base.Summary = agent.LastLine
		if base.Summary == "" {
			base.Summary = "agent appears blocked"
		}
		return base, true
	default:
		return InboxItem{}, false
	}
}

func promptInboxDetails(agent AgentStatus) map[string]interface{} {
	details := map[string]interface{}{
		"title": agent.Prompt.Title,
		"paths": agent.Prompt.Paths,
	}
	if agent.Prompt.Question != "" {
		details["question"] = agent.Prompt.Question
	}
	if agent.Prompt.SelectedOption != nil {
		details["selected_option"] = map[string]interface{}{
			"number": agent.Prompt.SelectedOption.Number,
			"label":  agent.Prompt.SelectedOption.Label,
		}
	}
	return details
}
