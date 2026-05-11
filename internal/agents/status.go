package agents

import (
	"bytes"
	"os/exec"
	"strings"
	"time"

	"tmact/internal/prompt"
	"tmact/internal/tmux"
)

const (
	StateBlocked           = "blocked"
	StateIdle              = "idle"
	StateUnknown           = "unknown"
	StateWaitingPermission = "waiting_permission"
	StateWorking           = "working"
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
	if detected := prompt.DetectDirectoryAccess(raw); detected != nil {
		return StateWaitingPermission, detected
	}

	lines := cleanedLines(raw)
	if len(lines) == 0 {
		return StateUnknown, nil
	}

	last := strings.ToLower(lastInteractiveLine(lines))
	if looksLikeAgentPrompt(last) || looksLikeShellPrompt(last) {
		return StateIdle, nil
	}
	if containsAny(last, []string{
		"waiting for input",
		"enter your prompt",
		"type a message",
		"what would you like",
	}) {
		return StateIdle, nil
	}

	recent := recentLines(lines, 20)
	for _, line := range recent {
		if isAgentChromeLine(line) || looksLikeAgentPrompt(line) {
			continue
		}
		lower := strings.ToLower(line)
		if isKnownIdleOutputLine(lower) {
			continue
		}
		if containsAny(lower, []string{
			"working",
			"thinking",
			"running",
			"executing",
			"esc to interrupt",
			"ctrl-c to interrupt",
		}) {
			return StateWorking, nil
		}
		if containsAny(lower, []string{
			"waiting for approval",
			"waiting for confirmation",
			"permission denied",
			"merge conflict",
		}) {
			return StateBlocked, nil
		}
	}

	return StateUnknown, nil
}

func isKnownIdleOutputLine(text string) bool {
	return containsAny(text, []string{
		"nothing to commit, working tree clean",
		"working tree clean",
	})
}

func LastMeaningfulLine(raw string) string {
	lines := cleanedLines(raw)
	if len(lines) == 0 {
		return ""
	}
	return truncate(lines[len(lines)-1], 180)
}

func cleanedLines(raw string) []string {
	var lines []string
	for _, line := range strings.Split(raw, "\n") {
		cleaned := prompt.CleanLine(line)
		if cleaned != "" {
			lines = append(lines, cleaned)
		}
	}
	return lines
}

func containsAny(text string, needles []string) bool {
	for _, needle := range needles {
		if strings.Contains(text, needle) {
			return true
		}
	}
	return false
}

func recentLines(lines []string, max int) []string {
	if len(lines) <= max {
		return lines
	}
	return lines[len(lines)-max:]
}

func lastInteractiveLine(lines []string) string {
	for i := len(lines) - 1; i >= 0; i-- {
		line := lines[i]
		if isAgentChromeLine(line) {
			continue
		}
		return line
	}
	return ""
}

func isAgentChromeLine(text string) bool {
	text = strings.ToLower(strings.TrimSpace(text))
	if text == "" {
		return true
	}
	return containsAny(text, []string{
		"welcome back",
		"tips for getting",
		"what's new",
		"run /init",
		"/release-notes",
		"token usage:",
		"to continue this session",
		"codex app",
		"claude code",
		"openai codex",
		"model:",
		"directory:",
		"context ",
		"cost:",
	})
}

func looksLikeAgentPrompt(text string) bool {
	text = strings.TrimSpace(text)
	return strings.HasPrefix(text, "❯") || strings.HasPrefix(text, "›")
}

func looksLikeShellPrompt(text string) bool {
	text = strings.TrimSpace(text)
	return strings.HasSuffix(text, "$") || strings.HasSuffix(text, "%") || strings.HasSuffix(text, ">")
}

func truncate(text string, max int) string {
	if len(text) <= max {
		return text
	}
	return text[:max] + "..."
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
