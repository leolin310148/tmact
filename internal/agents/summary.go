package agents

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"tmact/internal/panestate"
	"tmact/internal/prompt"
	"tmact/internal/tmux"
)

type SummaryReport struct {
	Timestamp string         `json:"ts"`
	Summaries []AgentSummary `json:"summaries"`
}

type AgentSummary struct {
	Name       string                  `json:"name"`
	Target     string                  `json:"target"`
	Type       string                  `json:"type,omitempty"`
	Role       string                  `json:"role,omitempty"`
	Repo       string                  `json:"repo,omitempty"`
	State      string                  `json:"state"`
	LastLines  []string                `json:"last_lines,omitempty"`
	Prompt     *prompt.DirectoryAccess `json:"prompt,omitempty"`
	Git        *GitSummary             `json:"git,omitempty"`
	Error      string                  `json:"error,omitempty"`
	NextAction string                  `json:"next_action,omitempty"`
}

type GitSummary struct {
	Branch        string      `json:"branch,omitempty"`
	Dirty         bool        `json:"dirty"`
	Short         string      `json:"short,omitempty"`
	ChangedFiles  []string    `json:"changed_files,omitempty"`
	RecentCommits []GitCommit `json:"recent_commits,omitempty"`
	Error         string      `json:"error,omitempty"`
}

type GitCommit struct {
	Hash    string `json:"hash"`
	Subject string `json:"subject"`
}

func Summarize(cfg Config, agentName string, lineCount int, commitLimit int) (SummaryReport, error) {
	if lineCount <= 0 {
		lineCount = 12
	}
	if commitLimit <= 0 {
		commitLimit = 5
	}

	agents, err := selectedAgents(cfg, agentName)
	if err != nil {
		return SummaryReport{}, err
	}

	report := SummaryReport{
		Timestamp: time.Now().Format(time.RFC3339),
		Summaries: make([]AgentSummary, 0, len(agents)),
	}
	for _, agent := range agents {
		report.Summaries = append(report.Summaries, summarizeAgent(agent, lineCount, commitLimit))
	}
	return report, nil
}

func selectedAgents(cfg Config, agentName string) ([]AgentConfig, error) {
	if agentName == "" {
		return cfg.Agents, nil
	}
	for _, agent := range cfg.Agents {
		if agent.Name == agentName {
			return []AgentConfig{agent}, nil
		}
	}
	return nil, fmt.Errorf("agent %q not found", agentName)
}

func summarizeAgent(agent AgentConfig, lineCount int, commitLimit int) AgentSummary {
	summary := AgentSummary{
		Name:   agent.Name,
		Target: agent.Target,
		Type:   agent.Type,
		Role:   agent.Role,
		Repo:   agent.Repo,
		State:  StateUnknown,
	}

	raw, err := tmux.CapturePane(agent.Target, agent.CaptureLines)
	if err != nil {
		summary.State = StateBlocked
		summary.Error = err.Error()
	} else {
		summary.State, summary.Prompt = ClassifyPane(raw)
		summary.LastLines = LastMeaningfulLines(raw, lineCount)
	}

	if agent.Repo != "" {
		summary.Git = InspectGitSummary(agent.Repo, commitLimit)
	}
	summary.NextAction = RecommendNextAction(summary)
	return summary
}

func LastMeaningfulLines(raw string, count int) []string {
	if count <= 0 {
		return nil
	}

	lines := panestate.CleanedLines(raw)
	if len(lines) == 0 {
		return nil
	}
	if len(lines) > count {
		lines = lines[len(lines)-count:]
	}
	for i := range lines {
		lines[i] = truncate(lines[i], 180)
	}
	return lines
}

func truncate(text string, max int) string {
	if len(text) <= max {
		return text
	}
	return text[:max] + "..."
}

func InspectGitSummary(repo string, commitLimit int) *GitSummary {
	status := InspectGit(repo)
	summary := &GitSummary{
		Branch: status.Branch,
		Dirty:  status.Dirty,
		Short:  status.Short,
		Error:  status.Error,
	}
	if status.Error != "" {
		return summary
	}

	summary.ChangedFiles = changedFiles(status.Short)
	summary.RecentCommits = recentCommits(repo, commitLimit)
	return summary
}

func changedFiles(shortStatus string) []string {
	var files []string
	for _, line := range strings.Split(shortStatus, "\n") {
		if strings.TrimSpace(line) == "" || strings.HasPrefix(line, "## ") {
			continue
		}
		if len(line) <= 3 {
			continue
		}
		files = append(files, strings.TrimSpace(line[3:]))
	}
	return files
}

func recentCommits(repo string, limit int) []GitCommit {
	if limit <= 0 {
		return nil
	}

	cmd := exec.Command("git", "-C", repo, "log", fmt.Sprintf("-%d", limit), "--pretty=format:%h%x00%s")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	output, err := cmd.Output()
	if err != nil {
		return nil
	}

	var commits []GitCommit
	for _, line := range strings.Split(strings.TrimSpace(string(output)), "\n") {
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "\x00", 2)
		if len(parts) != 2 {
			continue
		}
		commits = append(commits, GitCommit{
			Hash:    parts[0],
			Subject: parts[1],
		})
	}
	return commits
}

func RecommendNextAction(summary AgentSummary) string {
	if summary.Error != "" {
		return "inspect tmux target or registry configuration"
	}
	if summary.Git != nil && summary.Git.Error != "" {
		return "inspect repository path or git state"
	}
	switch summary.State {
	case StateWaitingPermission:
		return "review permission prompt"
	case StateBlocked:
		return "open pane and unblock agent"
	case StateIdle:
		if summary.Git != nil && summary.Git.Dirty {
			return "review dirty worktree and ask agent to test or commit"
		}
		return "assign next task or leave idle"
	case StateWorking:
		return "monitor progress"
	default:
		return "inspect recent pane output"
	}
}
