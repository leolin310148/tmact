package prompt

import (
	"slices"
	"strings"
	"testing"
)

func TestRecentCleanedLinesMatchesFullClean(t *testing.T) {
	long := make([]string, 0, 120)
	for n := 0; n < 120; n++ {
		switch n % 4 {
		case 0:
			long = append(long, "")
		case 1:
			long = append(long, "\x1b[2m│  dim boxed line  │\x1b[0m")
		default:
			long = append(long, "line "+strings.Repeat("x", n%7))
		}
	}
	cases := []string{
		"",
		"\n",
		"\n\n\n",
		"only",
		"a\nb\n",
		"\x1b[31m❯ 1. Yes\x1b[0m\n  2. No\n",
		"│   │\n╰───╯\nkept",
		strings.Join(long, "\n"),
		strings.Join(long, "\n") + "\n",
	}
	for _, raw := range cases {
		for _, max := range []int{1, 3, 24, 40} {
			want := recentLines(cleanedLines(raw), max)
			got := recentCleanedLines(raw, max)
			if len(want) == 0 && len(got) == 0 {
				continue
			}
			if !slices.Equal(got, want) {
				t.Fatalf("recentCleanedLines(%q, %d) = %q, want %q", raw, max, got, want)
			}
		}
	}
}
