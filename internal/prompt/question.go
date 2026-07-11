package prompt

import "strings"

// Question is a detected interactive menu in pane output: a prompt the agent
// is waiting on plus the numbered choices a user can pick. It is a looser,
// presentation-oriented detection than Detect — the web UI turns each Choice
// into a quick-answer button, so a tap relays the matching digit into the pane.
type Question struct {
	Prompt  string   `json:"prompt,omitempty"`
	Choices []Choice `json:"choices"`
}

// Choice is one selectable answer. Number is both shown on the button and the
// digit relayed into the pane when tapped; Label is the option's text, kept
// for context on the button.
type Choice struct {
	Number int    `json:"number"`
	Label  string `json:"label,omitempty"`
}

// DetectQuestion reports an interactive menu the pane is waiting on, or nil.
// It first reuses Detect for the known approval prompts, then falls back to a
// generic scan for a trailing numbered menu — the kind an agent renders for a
// custom multiple-choice question that Detect's header list does not cover.
func DetectQuestion(raw string) *Question {
	if detected := Detect(raw); detected != nil && len(detected.Options) > 0 {
		return &Question{
			Prompt:  questionText(detected),
			Choices: choicesFromOptions(detected.Options),
		}
	}
	return nil
}

func questionText(p *Prompt) string {
	if p.Question != "" {
		return p.Question
	}
	return p.Title
}

func choicesFromOptions(options []Option) []Choice {
	choices := make([]Choice, 0, len(options))
	for _, option := range options {
		choices = append(choices, Choice{Number: option.Number, Label: option.Label})
	}
	return choices
}

// detectTrailingChoicePrompt finds a numbered selection menu sitting at the bottom of
// the pane. It requires the trailing options to be numbered 1..N in order and
// at least one to carry the "❯" selection cursor — a marker an agent's prose
// numbered list never has, which keeps bulleted output from registering as a
// question.
func detectTrailingChoicePrompt(raw string) *Prompt {
	recent := recentLines(cleanedLines(raw), 40)
	// Codex's structured questions print a hint footer ("tab to add notes |
	// enter to submit answer", "←/→ to navigate questions | esc to interrupt")
	// below the menu. Strip those so the trailing-distance check below sees
	// the numbered options as the pane's bottom content.
	recent = trimMenuFooter(recent)

	type located struct {
		index  int
		option Option
	}
	var found []located
	for index, line := range recent {
		if option := parseOption(line); option != nil {
			found = append(found, located{index: index, option: *option})
		}
	}
	if len(found) < 2 {
		return nil
	}

	count := found[len(found)-1].option.Number
	if count < 2 || len(found) < count {
		return nil
	}
	menu := found[len(found)-count:]

	selected := false
	for offset, item := range menu {
		if item.option.Number != offset+1 {
			return nil
		}
		if item.option.Selected {
			selected = true
		}
	}
	if !selected {
		return nil
	}
	// The menu must be the pane's trailing content — an old menu scrolled up
	// in the buffer is no longer the question being asked. A live menu has at
	// most a hint line ("↑↓ to navigate") below its last option.
	if len(recent)-1-menu[len(menu)-1].index > 3 {
		return nil
	}

	options := make([]Option, 0, len(menu))
	for index, item := range menu {
		option := item.option
		end := len(recent)
		if index+1 < len(menu) {
			end = menu[index+1].index
		}
		option.Label = optionLabelWithContinuation(recent, item.index, end)
		options = append(options, option)
	}
	detected := &Prompt{
		Type:       TypeChoicePrompt,
		Question:   menuPromptText(recent, menu[0].index),
		Options:    options,
		Confidence: "medium",
	}
	if selected := selectedOption(detected.Options); selected != nil {
		detected.SelectedOption = selected
	}
	if isClaudeTrustFolderMenu(detected.Options) {
		detected.Type = TypeTrustFolder
		detected.Title = "Confirm folder trust"
	}
	return detected
}

func isClaudeTrustFolderMenu(options []Option) bool {
	if len(options) != 2 || options[0].Number != 1 || options[1].Number != 2 {
		return false
	}
	return normalizePromptText(options[0].Label) == "yes, i trust this folder" &&
		normalizePromptText(options[1].Label) == "no, exit"
}

func optionLabelWithContinuation(lines []string, start, end int) string {
	option := parseOption(lines[start])
	if option == nil {
		return ""
	}
	parts := []string{option.Label}
	for index := start + 1; index < end; index++ {
		line := strings.TrimSpace(lines[index])
		if line != "" {
			parts = append(parts, line)
		}
	}
	return strings.Join(parts, " ")
}

// trimMenuFooter strips trailing hint lines printed below an interactive menu.
// Codex and Claude render keyboard shortcuts like "tab to add notes",
// "enter to submit", "esc to interrupt", or arrow-key navigation hints — chrome
// that sits beneath the numbered options without scrolling them out of view.
func trimMenuFooter(lines []string) []string {
	for len(lines) > 0 && isMenuFooter(lines[len(lines)-1]) {
		lines = lines[:len(lines)-1]
	}
	return lines
}

func isMenuFooter(line string) bool {
	lower := strings.ToLower(line)
	switch {
	case strings.HasPrefix(lower, "tab to "),
		strings.HasPrefix(lower, "enter to "),
		strings.HasPrefix(lower, "esc to "),
		strings.HasPrefix(lower, "esc "):
		return true
	}
	for _, prefix := range []string{"←", "→", "↑", "↓"} {
		if strings.HasPrefix(line, prefix) {
			return true
		}
	}
	return false
}

// menuPromptText returns the line immediately above the first option — the
// question the menu answers — or "" when that line is itself an option or
// missing.
func menuPromptText(recent []string, firstOption int) string {
	if firstOption <= 0 {
		return ""
	}
	above := recent[firstOption-1]
	if parseOption(above) != nil {
		return ""
	}
	return above
}
