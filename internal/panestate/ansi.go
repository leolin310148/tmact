package panestate

import (
	"regexp"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

type styledRune struct {
	value rune
	dim   bool
}

const inputPlaceholder = "[input-placeholder]"

// AnnotateDimSuggestion replaces the current generated input suggestion with
// an explicit placeholder. Claude and Codex render generated suggestions dim,
// while operator-entered drafts are non-dim. Requiring the styled and plain
// input text to match also avoids hiding a draft if the pane changes between
// the two captures.
func AnnotateDimSuggestion(raw, ansi string) (string, bool) {
	input, ok := currentStyledInput(ansi)
	if !ok || len(input) == 0 || !allInputDim(input) {
		return raw, false
	}

	styledText := styledInputText(input)
	lines := strings.Split(raw, "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		prefix, plainText, ok := splitInputLine(lines[i])
		if !ok {
			continue
		}
		if plainText != styledText {
			return raw, false
		}
		lines[i] = prefix + " " + inputPlaceholder
		return strings.Join(lines, "\n"), true
	}
	return raw, false
}

// ClassifyANSI refines the plain-text classification with the live input
// line's terminal attributes. Claude and Codex render generated suggestions
// dim, while text typed by the operator is non-dim. Keeping those states
// separate prevents automation from clearing an unsent draft.
func ClassifyANSI(raw, ansi string) Result {
	result := Classify(raw)
	if result.Asking || result.State == StateWaitingQuota || hasSignal(result.Signals, "usage_limit_unrecognized") || !strings.Contains(ansi, "\x1b[") {
		return result
	}
	if hasActiveInterruptIndicator(raw) {
		result.State = StateWorking
		result.Signals = appendSignal(result.Signals, "working_text")
		return result
	}
	input, ok := currentStyledInput(ansi)
	if !ok {
		return result
	}
	if len(input) == 0 {
		result.State = StateWaitingInput
		result.Signals = appendSignal(result.Signals, "empty_input")
		return result
	}
	if allInputDim(input) {
		result.State = StateWaitingInput
		result.Signals = appendSignal(result.Signals, "dim_suggestion")
		return result
	}
	result.State = StateDraftInput
	result.Signals = appendSignal(result.Signals, "draft_input")
	return result
}

func allInputDim(input []styledRune) bool {
	for _, char := range input {
		if !unicode.IsSpace(char.value) && !char.dim {
			return false
		}
	}
	return true
}

func styledInputText(input []styledRune) string {
	runes := make([]rune, len(input))
	for i, char := range input {
		runes[i] = char.value
	}
	return string(runes)
}

func splitInputLine(line string) (prefix, input string, ok bool) {
	runes := []rune(line)
	start := 0
	for start < len(runes) && unicode.IsSpace(runes[start]) {
		start++
	}
	if start == len(runes) || (runes[start] != '❯' && runes[start] != '›') {
		return "", "", false
	}
	prefix = string(runes[:start+1])
	start++
	for start < len(runes) && unicode.IsSpace(runes[start]) {
		start++
	}
	end := len(runes)
	for end > start && unicode.IsSpace(runes[end-1]) {
		end--
	}
	return prefix, string(runes[start:end]), true
}

func hasSignal(signals []string, want string) bool {
	for _, signal := range signals {
		if signal == want {
			return true
		}
	}
	return false
}

func hasActiveInterruptIndicator(raw string) bool {
	lines := CleanedLines(raw)
	promptIndex := -1
	for i := len(lines) - 1; i >= 0; i-- {
		if looksLikeAgentPrompt(lines[i]) {
			promptIndex = i
			break
		}
	}
	if promptIndex < 0 {
		return false
	}
	for i := promptIndex - 1; i >= 0 && promptIndex-i <= 3; i-- {
		line := lines[i]
		if isAgentChromeLine(line) {
			continue
		}
		lower := strings.ToLower(line)
		if strings.Contains(lower, "esc to interrupt") || strings.Contains(lower, "ctrl-c to interrupt") {
			return true
		}
		return claudeSpinnerPattern.MatchString(strings.TrimSpace(line))
	}
	return false
}

// claudeSpinnerPattern matches Claude's live activity line — spinner glyph,
// gerund, ellipsis, elapsed timer — which narrow or split panes render
// without the "esc to interrupt" tail, e.g. "✻ Frosting… (2m 51s)".
var claudeSpinnerPattern = regexp.MustCompile(`^[✻✽✶✳✢✺·∗*+] ?[A-Za-z][A-Za-z' -]{1,40}(…|\.\.\.)\s*\((?:\d+h\s*)?(?:\d+m\s*)?\d+s\b`)

func currentStyledInput(raw string) ([]styledRune, bool) {
	lines := strings.Split(raw, "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		chars := parseStyledLine(lines[i])
		start := 0
		for start < len(chars) && unicode.IsSpace(chars[start].value) {
			start++
		}
		if start == len(chars) || (chars[start].value != '❯' && chars[start].value != '›') {
			continue
		}
		start++
		for start < len(chars) && unicode.IsSpace(chars[start].value) {
			start++
		}
		end := len(chars)
		for end > start && unicode.IsSpace(chars[end-1].value) {
			end--
		}
		return chars[start:end], true
	}
	return nil, false
}

func parseStyledLine(line string) []styledRune {
	dim := false
	var chars []styledRune
	for len(line) > 0 {
		if strings.HasPrefix(line, "\x1b[") {
			end := 2
			for end < len(line) && (line[end] < '@' || line[end] > '~') {
				end++
			}
			if end < len(line) {
				if line[end] == 'm' {
					dim = applySGRDim(dim, line[2:end])
				}
				line = line[end+1:]
				continue
			}
		}
		if strings.HasPrefix(line, "\x1b]") {
			if end := strings.IndexByte(line, '\a'); end >= 0 {
				line = line[end+1:]
				continue
			}
			if end := strings.Index(line, "\x1b\\"); end >= 0 {
				line = line[end+2:]
				continue
			}
		}
		r, size := utf8.DecodeRuneInString(line)
		chars = append(chars, styledRune{value: r, dim: dim})
		line = line[size:]
	}
	return chars
}

func applySGRDim(dim bool, params string) bool {
	if params == "" {
		return false
	}
	values := strings.Split(params, ";")
	for i := 0; i < len(values); i++ {
		code, err := strconv.Atoi(values[i])
		if err != nil {
			continue
		}
		if code == 38 || code == 48 || code == 58 {
			if i+1 < len(values) {
				switch values[i+1] {
				case "2":
					i += 4
				case "5":
					i += 2
				}
			}
			continue
		}
		switch code {
		case 0, 22:
			dim = false
		case 2:
			dim = true
		}
	}
	return dim
}
