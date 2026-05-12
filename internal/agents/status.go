package agents

import (
	"bytes"
	"os/exec"
	"strings"
	"time"

	"tmact/internal/panestate"
	"tmact/internal/prompt"
	"tmact/internal/tmux"
)

const (
	StateBlocked           = panestate.StateBlocked
	StateIdle              = panestate.StateIdle
	StateUnknown           = panestate.StateUnknown
	StateWaitingPermission = panestate.StateWaitingPermission
	StateWaitingInput      = panestate.StateWaitingInput
	StateWorking           = panestate.StateWorking
)

type Report struct {
	Timestamp string        `json:"ts"`
	Agents    []AgentStatus `json:"agents"`
}

type AgentStatus struct {
	Name     string                  `json:"name"`
	Target   string                  `json:"target"`
	Type     string                  `json:"type,omitempty"`
	Role     string                  `json:"role,omitempty"`
	Repo     string                  `json:"repo,omitempty"`
	State    string                  `json:"state"`
	LastLine string                  `json:"last_line,omitempty"`
	Prompt   *prompt.DirectoryAccess `json:"prompt,omitempty"`
	Git      *GitStatus              `json:"git,omitempty"`
	Error    string                  `json:"error,omitempty"`
}

type GitStatus struct {
	Branch string `json:"branch,omitempty"`
	Dirty  bool   `json:"dirty"`
	Short  string `json:"short,omitempty"`
	Error  string `json:"error,omitempty"`
}

func Collect(cfg Config) Report {
	report := Report{
		Timestamp: time.Now().Format(time.RFC3339),
		Agents:    make([]AgentStatus, 0, len(cfg.Agents)),
	}
	for _, agent := range cfg.Agents {
		report.Agents = append(report.Agents, collectAgent(agent))
	}
	return report
}

func collectAgent(agent AgentConfig) AgentStatus {
	status := AgentStatus{
		Name:   agent.Name,
		Target: agent.Target,
		Type:   agent.Type,
		Role:   agent.Role,
		Repo:   agent.Repo,
		State:  StateUnknown,
	}

	raw, err := tmux.CapturePane(agent.Target, agent.CaptureLines)
	if err != nil {
		status.State = StateBlocked
		status.Error = err.Error()
	} else {
		status.LastLine = LastMeaningfulLine(raw)
		status.State, status.Prompt = ClassifyPane(raw)
	}

	if agent.Repo != "" {
		status.Git = InspectGit(agent.Repo)
	}
	return status
}

func ClassifyPane(raw string) (string, *prompt.DirectoryAccess) {
	result := panestate.Classify(raw)
	if result.State == panestate.StateWaitingInput {
		return StateIdle, result.Prompt
	}
	return result.State, result.Prompt
}

func LastMeaningfulLine(raw string) string {
	return panestate.LastMeaningfulLine(raw)
}

func InspectGit(repo string) *GitStatus {
	cmd := exec.Command("git", "-C", repo, "status", "--short", "--branch")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	output, err := cmd.Output()
	if err != nil {
		message := strings.TrimSpace(stderr.String())
		if message == "" {
			message = err.Error()
		}
		return &GitStatus{Error: message}
	}

	text := strings.TrimSpace(string(output))
	status := &GitStatus{Short: text}
	if text == "" {
		return status
	}

	lines := strings.Split(text, "\n")
	if strings.HasPrefix(lines[0], "## ") {
		status.Branch = parseBranchLine(lines[0])
		lines = lines[1:]
	}
	status.Dirty = len(lines) > 0
	return status
}

func parseBranchLine(line string) string {
	branch := strings.TrimPrefix(line, "## ")
	if idx := strings.Index(branch, "..."); idx >= 0 {
		branch = branch[:idx]
	}
	return strings.TrimSpace(branch)
}
