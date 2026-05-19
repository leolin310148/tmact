package prompt

import (
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"
)

type DirectoryAccess struct {
	Title          string   `json:"title"`
	Path           string   `json:"path,omitempty"`
	Paths          []string `json:"paths,omitempty"`
	Question       string   `json:"question,omitempty"`
	SelectedOption *Option  `json:"selected_option,omitempty"`
	Options        []Option `json:"options,omitempty"`
}

const (
	TypeCommandApproval     = "command_approval"
	TypeDirectoryAccess     = "directory_access"
	TypeGenericConfirmation = "generic_confirmation"
	TypePatchApproval       = "patch_approval"
	TypeTrustFolder         = "trust_folder"
	TypeWaitingApproval     = "waiting_approval"
)

type Prompt struct {
	Type           string   `json:"type"`
	Title          string   `json:"title,omitempty"`
	Path           string   `json:"path,omitempty"`
	Paths          []string `json:"paths,omitempty"`
	Question       string   `json:"question,omitempty"`
	SelectedOption *Option  `json:"selected_option,omitempty"`
	Options        []Option `json:"options,omitempty"`
	Confidence     string   `json:"confidence,omitempty"`
}

type Option struct {
	Number   int    `json:"number"`
	Label    string `json:"label"`
	Selected bool   `json:"selected"`
}

// optionPattern matches a numbered menu option, optionally led by a selection
// cursor. Claude renders the cursor as "❯", Codex as "›" — both mean the row
// is the current choice.
var optionPattern = regexp.MustCompile(`^([❯›]\s*)?([0-9]+)\.\s+(.+)$`)

func Detect(raw string) *Prompt {
	if detected := DetectDirectoryAccess(raw); detected != nil {
		return PromptFromDirectoryAccess(detected)
	}
	return detectGenericPrompt(raw)
}

func PromptFromDirectoryAccess(detected *DirectoryAccess) *Prompt {
	if detected == nil {
		return nil
	}
	return &Prompt{
		Type:           TypeDirectoryAccess,
		Title:          detected.Title,
		Path:           detected.Path,
		Paths:          append([]string{}, detected.Paths...),
		Question:       detected.Question,
		SelectedOption: cloneOption(detected.SelectedOption),
		Options:        append([]Option{}, detected.Options...),
		Confidence:     "high",
	}
}

func DirectoryAccessFromPrompt(detected *Prompt) *DirectoryAccess {
	if detected == nil || detected.Type != TypeDirectoryAccess {
		return nil
	}
	return &DirectoryAccess{
		Title:          detected.Title,
		Path:           detected.Path,
		Paths:          append([]string{}, detected.Paths...),
		Question:       detected.Question,
		SelectedOption: cloneOption(detected.SelectedOption),
		Options:        append([]Option{}, detected.Options...),
	}
}

func DetectDirectoryAccess(raw string) *DirectoryAccess {
	var detected *DirectoryAccess

	for _, line := range strings.Split(raw, "\n") {
		text := CleanLine(line)
		if text == "" {
			continue
		}

		if strings.Contains(text, "Allow directory access") {
			detected = &DirectoryAccess{Title: "Allow directory access"}
			continue
		}
		if detected == nil {
			continue
		}

		if strings.Contains(text, "Do you want to allow this?") {
			detected.Question = "Do you want to allow this?"
			continue
		}

		if option := parseOption(text); option != nil {
			detected.Options = append(detected.Options, *option)
			if option.Selected {
				selected := *option
				detected.SelectedOption = &selected
			}
			continue
		}

		if paths := parsePaths(text); len(paths) > 0 {
			if detected.Path == "" {
				detected.Path = paths[0]
			}
			detected.Paths = append(detected.Paths, paths...)
		}
	}

	if detected == nil {
		return nil
	}
	if detected.Question == "" || len(detected.Options) == 0 {
		return nil
	}
	return detected
}

func detectGenericPrompt(raw string) *Prompt {
	lines := cleanedLines(raw)
	if len(lines) == 0 {
		return nil
	}
	recent := recentLines(lines, 24)
	for index, line := range recent {
		lower := strings.ToLower(line)
		promptType, title, ok := genericPromptHeader(lower)
		if !ok {
			continue
		}
		prompt := &Prompt{
			Type:       promptType,
			Title:      title,
			Question:   line,
			Confidence: "medium",
		}
		prompt.Options = collectOptions(recent[index+1:])
		if selected := selectedOption(prompt.Options); selected != nil {
			prompt.SelectedOption = selected
		}
		if len(prompt.Options) > 0 || promptType == TypeWaitingApproval {
			return prompt
		}
	}
	return nil
}

func genericPromptHeader(lower string) (string, string, bool) {
	text := strings.TrimSpace(lower)
	switch {
	case strings.HasPrefix(text, "waiting for approval"):
		return TypeWaitingApproval, "Waiting for approval", true
	case strings.HasPrefix(text, "waiting for confirmation"):
		return TypeWaitingApproval, "Waiting for confirmation", true
	case strings.HasPrefix(text, "allow command?"):
		return TypeCommandApproval, "Allow command?", true
	case strings.HasPrefix(text, "allow this command?"):
		return TypeCommandApproval, "Allow this command?", true
	case strings.HasPrefix(text, "apply this patch?"):
		return TypePatchApproval, "Apply this patch?", true
	case strings.HasPrefix(text, "do you want to proceed?"):
		return TypeGenericConfirmation, "Do you want to proceed?", true
	case strings.Contains(text, "do you trust the files in this folder?"):
		return TypeTrustFolder, "Do you trust the files in this folder?", true
	case strings.Contains(text, "confirm folder trust"):
		return TypeTrustFolder, "Confirm folder trust", true
	default:
		return "", "", false
	}
}

func collectOptions(lines []string) []Option {
	options := []Option{}
	for _, line := range lines {
		if option := parseOption(line); option != nil {
			options = append(options, *option)
		}
	}
	return options
}

func selectedOption(options []Option) *Option {
	for _, option := range options {
		if option.Selected {
			selected := option
			return &selected
		}
	}
	return nil
}

func cleanedLines(raw string) []string {
	var lines []string
	for _, line := range strings.Split(raw, "\n") {
		text := CleanLine(line)
		if text != "" {
			lines = append(lines, text)
		}
	}
	return lines
}

func recentLines(lines []string, max int) []string {
	if len(lines) <= max {
		return lines
	}
	return lines[len(lines)-max:]
}

func cloneOption(option *Option) *Option {
	if option == nil {
		return nil
	}
	cloned := *option
	return &cloned
}

func parseOption(text string) *Option {
	matches := optionPattern.FindStringSubmatch(text)
	if matches == nil {
		return nil
	}

	number, err := strconv.Atoi(matches[2])
	if err != nil {
		return nil
	}
	label := strings.TrimSpace(trimBoxRight(matches[3]))
	cursor := strings.TrimSpace(matches[1])
	return &Option{
		Number:   number,
		Label:    label,
		Selected: cursor == "❯" || cursor == "›",
	}
}

func parsePaths(text string) []string {
	var paths []string
	for _, field := range strings.Split(text, ",") {
		path := strings.TrimSpace(field)
		if looksLikePath(path) {
			paths = append(paths, path)
		}
	}
	return paths
}

func looksLikePath(text string) bool {
	if !strings.Contains(text, "/") {
		return false
	}
	return strings.HasPrefix(text, "/") ||
		strings.HasPrefix(text, "./") ||
		strings.HasPrefix(text, "../") ||
		strings.HasPrefix(text, "~/")
}

func CleanLine(line string) string {
	text := stripANSI(line)
	text = strings.TrimSpace(text)
	for text != "" {
		next := strings.TrimSpace(trimBoxLeft(trimBoxRight(text)))
		if next == text {
			break
		}
		text = next
	}
	return strings.TrimSpace(text)
}

func trimBoxLeft(text string) string {
	for text != "" {
		r, size := utf8.DecodeRuneInString(text)
		if !isBoxBorder(r) {
			return text
		}
		text = strings.TrimSpace(text[size:])
	}
	return text
}

func trimBoxRight(text string) string {
	for text != "" {
		r, size := utf8.DecodeLastRuneInString(text)
		if !isBoxBorder(r) {
			return text
		}
		text = strings.TrimSpace(text[:len(text)-size])
	}
	return text
}

func isBoxBorder(r rune) bool {
	switch r {
	case '│', '┃', '║', '╭', '╮', '╰', '╯', '─', '━', '═', '┌', '┐', '└', '┘':
		return true
	default:
		return false
	}
}

func stripANSI(text string) string {
	var b strings.Builder
	inEscape := false
	for i := 0; i < len(text); i++ {
		c := text[i]
		if inEscape {
			if (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') {
				inEscape = false
			}
			continue
		}
		if c == 0x1b {
			inEscape = true
			continue
		}
		b.WriteByte(c)
	}
	return b.String()
}
