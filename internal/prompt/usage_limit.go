package prompt

import (
	"regexp"
	"strconv"
	"strings"
	"time"
)

const (
	UsageLimitProviderClaude = "claude"
	UsageLimitProviderCodex  = "codex"
)

// UsageLimit describes an exact, allowlisted provider quota screen. Codex
// limits are passive terminal output; Claude limits require selecting the
// already-highlighted wait option.
type UsageLimit struct {
	Provider          string
	ResetAt           time.Time
	Timezone          string
	WaitSelection     bool
	CurrentScreenText string
}

var codexUsageLimitPattern = regexp.MustCompile(`(?i)^■ You've hit your usage limit\. Visit https://chatgpt\.com/codex/settings/usage to purchase more credits or try again at ([A-Z][a-z]{2}) ([0-9]{1,2})(st|nd|rd|th), ([0-9]{4}) ([0-9]{1,2}):([0-9]{2}) (AM|PM)\.$`)

// DetectUsageLimit recognizes only the exact current Claude or Codex limit
// screen and parses its reset instant. observedAt supplies the local timezone
// for provider timestamps that are rendered without an explicit zone.
func DetectUsageLimit(raw string, detected *Prompt, observedAt time.Time) (*UsageLimit, bool) {
	if IsClaudeSessionLimitWait(raw, detected) {
		resetAt, ok := ClaudeSessionLimitResetAt(raw, detected, observedAt)
		if !ok {
			return nil, false
		}
		return &UsageLimit{
			Provider:      UsageLimitProviderClaude,
			ResetAt:       resetAt,
			Timezone:      resetAt.Location().String(),
			WaitSelection: true,
		}, true
	}

	text, ok := currentCodexUsageLimitText(raw)
	if !ok {
		return nil, false
	}
	match := codexUsageLimitPattern.FindStringSubmatch(text)
	if len(match) == 0 {
		return nil, false
	}
	resetAt, ok := parseCodexUsageLimitTime(match, observedAt.Location())
	if !ok {
		return nil, false
	}
	return &UsageLimit{
		Provider:          UsageLimitProviderCodex,
		ResetAt:           resetAt,
		Timezone:          resetAt.Location().String(),
		CurrentScreenText: text,
	}, true
}

// HasCurrentUsageLimitMarker reports a current provider limit marker even if
// the surrounding allowlisted format cannot be parsed. Callers use this to
// fail closed instead of treating format drift as an input-ready pane.
func HasCurrentUsageLimitMarker(raw string, detected *Prompt) bool {
	if detected != nil && strings.Contains(normalizePromptText(strings.Join(cleanedLines(raw), " ")), "you've hit your session limit") {
		return true
	}
	_, ok := currentCodexUsageLimitText(raw)
	return ok
}

func currentCodexUsageLimitText(raw string) (string, bool) {
	lines := cleanedLines(raw)
	for index := len(lines) - 1; index >= 0; index-- {
		if !looksLikeCodexUsageLimitStart(lines[index]) {
			continue
		}
		end := len(lines)
		for next := index + 1; next < len(lines); next++ {
			if strings.HasPrefix(lines[next], "›") {
				end = next
				tail := next + 1
				if tail < len(lines) && isCodexUsageFooter(lines[tail]) {
					tail++
				}
				if tail != len(lines) {
					return "", false
				}
				return strings.Join(lines[index:end], " "), true
			}
		}
		return strings.Join(lines[index:end], " "), true
	}
	return "", false
}

func looksLikeCodexUsageLimitStart(line string) bool {
	if !strings.HasPrefix(line, "■ ") {
		return false
	}
	lower := strings.ToLower(line)
	signals := 0
	for _, signal := range []string{"usage limit", "chatgpt.com/codex/settings/usage", "try again"} {
		if strings.Contains(lower, signal) {
			signals++
		}
	}
	return signals >= 2
}

func isCodexUsageFooter(line string) bool {
	return strings.Contains(line, " · Context ") && strings.HasSuffix(line, " used")
}

func parseCodexUsageLimitTime(match []string, location *time.Location) (time.Time, bool) {
	if location == nil {
		return time.Time{}, false
	}
	month, ok := parseEnglishMonth(match[1])
	if !ok {
		return time.Time{}, false
	}
	day, err := strconv.Atoi(match[2])
	if err != nil || ordinalSuffix(day) != strings.ToLower(match[3]) {
		return time.Time{}, false
	}
	year, err := strconv.Atoi(match[4])
	if err != nil || year < 2000 || year > 9999 {
		return time.Time{}, false
	}
	hour, err := strconv.Atoi(match[5])
	if err != nil || hour < 1 || hour > 12 {
		return time.Time{}, false
	}
	minute, err := strconv.Atoi(match[6])
	if err != nil || minute < 0 || minute > 59 {
		return time.Time{}, false
	}
	if strings.EqualFold(match[7], "AM") {
		if hour == 12 {
			hour = 0
		}
	} else if hour != 12 {
		hour += 12
	}
	resetAt := time.Date(year, month, day, hour, minute, 0, 0, location)
	if resetAt.Year() != year || resetAt.Month() != month || resetAt.Day() != day || resetAt.Hour() != hour || resetAt.Minute() != minute {
		return time.Time{}, false
	}
	for _, delta := range []time.Duration{-2 * time.Hour, -time.Hour, time.Hour, 2 * time.Hour} {
		other := resetAt.Add(delta).In(location)
		if other.Year() == year && other.Month() == month && other.Day() == day && other.Hour() == hour && other.Minute() == minute {
			return time.Time{}, false
		}
	}
	return resetAt, true
}

func parseEnglishMonth(value string) (time.Month, bool) {
	for month := time.January; month <= time.December; month++ {
		if strings.EqualFold(value, month.String()[:3]) {
			return month, true
		}
	}
	return 0, false
}

func ordinalSuffix(day int) string {
	if day < 1 || day > 31 {
		return ""
	}
	if day >= 11 && day <= 13 {
		return "th"
	}
	switch day % 10 {
	case 1:
		return "st"
	case 2:
		return "nd"
	case 3:
		return "rd"
	default:
		return "th"
	}
}
