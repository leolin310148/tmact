package panestate

import (
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

type styledRune struct {
	value rune
	dim   bool
}

// ClassifyANSI refines the plain-text classification with the live input
// line's terminal attributes. Claude and Codex render generated suggestions
// dim, while text typed by the operator is non-dim. Keeping those states
// separate prevents automation from clearing an unsent draft.
func ClassifyANSI(raw, ansi string) Result {
	result := Classify(raw)
	if result.Asking || !strings.Contains(ansi, "\x1b[") {
		return result
	}
	input, ok := currentStyledInput(ansi)
	if !ok {
		return result
	}
	if len(input) == 0 {
		if hasRecentInterruptIndicator(raw) {
			result.State = StateWorking
			result.Signals = appendSignal(result.Signals, "working_text")
		} else if result.State != StateWorking {
			result.State = StateWaitingInput
			result.Signals = appendSignal(result.Signals, "empty_input")
		}
		return result
	}
	allDim := true
	for _, char := range input {
		if !unicode.IsSpace(char.value) && !char.dim {
			allDim = false
			break
		}
	}
	if allDim {
		result.State = StateWaitingInput
		result.Signals = appendSignal(result.Signals, "dim_suggestion")
		return result
	}
	result.State = StateDraftInput
	result.Signals = appendSignal(result.Signals, "draft_input")
	return result
}

func hasRecentInterruptIndicator(raw string) bool {
	lines := CleanedLines(raw)
	for _, line := range recentLines(lines, 20) {
		lower := strings.ToLower(line)
		if strings.Contains(lower, "esc to interrupt") || strings.Contains(lower, "ctrl-c to interrupt") {
			return true
		}
	}
	return false
}

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
