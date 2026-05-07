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

type Option struct {
	Number   int    `json:"number"`
	Label    string `json:"label"`
	Selected bool   `json:"selected"`
}

var optionPattern = regexp.MustCompile(`^(❯\s*)?([0-9]+)\.\s+(.+)$`)

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
	return &Option{
		Number:   number,
		Label:    label,
		Selected: strings.TrimSpace(matches[1]) == "❯",
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
