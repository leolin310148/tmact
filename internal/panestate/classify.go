package panestate

import (
	"strings"

	"github.com/leolin310148/tmact/internal/prompt"
)

const (
	StateBlocked           = "blocked"
	StateIdle              = "idle"
	StateUnknown           = "unknown"
	StateWaitingPermission = "waiting_permission"
	StateWaitingInput      = "waiting_input"
	StateWorking           = "working"
)

type Result struct {
	State             string
	Asking            bool
	Prompt            *prompt.DirectoryAccess
	InteractivePrompt *prompt.Prompt
	LastLine          string
	Signals           []string
}

func Classify(raw string) Result {
	result := Result{
		State:    StateUnknown,
		LastLine: LastMeaningfulLine(raw),
	}
	if detected := prompt.Detect(raw); detected != nil {
		result.State = StateWaitingPermission
		result.Asking = true
		result.InteractivePrompt = detected
		if detected.Type == prompt.TypeDirectoryAccess {
			result.Prompt = prompt.DetectDirectoryAccess(raw)
		}
		result.Signals = appendSignal(result.Signals, promptSignal(detected.Type))
		return result
	}

	lines := CleanedLines(raw)
	if len(lines) == 0 {
		return result
	}

	last := strings.ToLower(lastInteractiveLine(lines))
	if looksLikeAgentPrompt(last) || looksLikeShellPrompt(last) {
		result.State = StateWaitingInput
		result.Signals = appendSignal(result.Signals, "waiting_input_text")
		return result
	}
	if containsAny(last,
		"waiting for input",
		"enter your prompt",
		"type a message",
		"what would you like",
	) {
		result.State = StateWaitingInput
		result.Signals = appendSignal(result.Signals, "waiting_input_text")
		return result
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
		if containsAny(lower,
			"working",
			"thinking",
			"running",
			"executing",
			"esc to interrupt",
			"ctrl-c to interrupt",
		) {
			result.State = StateWorking
			result.Signals = appendSignal(result.Signals, "working_text")
			return result
		}
		if containsAny(lower,
			"permission denied",
			"merge conflict",
		) {
			result.State = StateBlocked
			result.Signals = appendSignal(result.Signals, "blocked_text")
			return result
		}
	}

	return result
}

func promptSignal(promptType string) string {
	switch promptType {
	case prompt.TypeDirectoryAccess:
		return "permission_prompt"
	case prompt.TypeTrustFolder:
		return "trust_prompt"
	case prompt.TypeCommandApproval:
		return "command_approval_prompt"
	case prompt.TypePatchApproval:
		return "patch_approval_prompt"
	default:
		return "asking_prompt"
	}
}

func LastMeaningfulLine(raw string) string {
	lines := CleanedLines(raw)
	if len(lines) == 0 {
		return ""
	}
	return truncate(lines[len(lines)-1], 180)
}

func CleanedLines(raw string) []string {
	var lines []string
	for _, line := range strings.Split(raw, "\n") {
		cleaned := prompt.CleanLine(line)
		if cleaned != "" {
			lines = append(lines, cleaned)
		}
	}
	return lines
}

func isKnownIdleOutputLine(text string) bool {
	return containsAny(text,
		"nothing to commit, working tree clean",
		"working tree clean",
	)
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
	if looksLikeClaudeIdleFooter(text) || looksLikeClaudeStatusBar(text) {
		return true
	}
	return containsAny(text,
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
	)
}

func looksLikeClaudeIdleFooter(text string) bool {
	return strings.Contains(text, "auto mode on (shift+tab to cycle)") && strings.Contains(text, "for agents")
}

func looksLikeClaudeStatusBar(text string) bool {
	if !strings.Contains(text, " | ") {
		return false
	}
	return containsAny(text, "ctx:", " context)", "opus", "sonnet", "haiku")
}

func looksLikeAgentPrompt(text string) bool {
	text = strings.TrimSpace(text)
	return strings.HasPrefix(text, "❯") || strings.HasPrefix(text, "›")
}

func looksLikeShellPrompt(text string) bool {
	text = strings.TrimSpace(text)
	return strings.HasSuffix(text, "$") || strings.HasSuffix(text, "%") || strings.HasSuffix(text, ">")
}

func containsAny(text string, needles ...string) bool {
	for _, needle := range needles {
		if strings.Contains(text, needle) {
			return true
		}
	}
	return false
}

func appendSignal(signals []string, signal string) []string {
	for _, existing := range signals {
		if existing == signal {
			return signals
		}
	}
	return append(signals, signal)
}

func truncate(text string, max int) string {
	if len(text) <= max {
		return text
	}
	return text[:max] + "..."
}
