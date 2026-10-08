package panestatus

import (
	"strings"
	"testing"
)

// joinedIdleHash is the original idle hash: drop ignored lines, rejoin, hash.
func joinedIdleHash(m *idleMatcher, raw string) string {
	var kept []string
	for _, line := range strings.Split(raw, "\n") {
		ignored := false
		for _, pattern := range m.patterns {
			if pattern.MatchString(line) {
				ignored = true
				break
			}
		}
		if !ignored {
			kept = append(kept, line)
		}
	}
	return hashText(strings.Join(kept, "\n"))
}

func TestIdleHashMatchesJoinedHash(t *testing.T) {
	matcher, err := compileIdlePatterns([]string{`^spinner`})
	if err != nil {
		t.Fatal(err)
	}
	i := inspector{ignore: matcher}
	cases := []string{
		"",
		"\n",
		"plain line",
		"Context 42% used",
		"Context 42% used\nCost: $1.20",
		"output\nContext 42% used\n> prompt",
		"Context 42% used\noutput\n",
		"output\n\n\nmore\n",
		"spinner frame\nToken usage: 12k/200k\nkept",
		"kept\nspinner frame",
	}
	for _, raw := range cases {
		want := joinedIdleHash(matcher, raw)
		// Twice: the second pass answers from the line memo.
		for pass := 0; pass < 2; pass++ {
			if got := i.idleHash(raw); got != want {
				t.Fatalf("pass %d idleHash(%q) = %s, want %s", pass, raw, got, want)
			}
		}
	}
}

func TestIdleHashWithoutPatternsHashesRaw(t *testing.T) {
	i := inspector{}
	if got, want := i.idleHash("a\nb"), hashText("a\nb"); got != want {
		t.Fatalf("idleHash = %s, want %s", got, want)
	}
}

func TestIdleMatcherMemoStaysBounded(t *testing.T) {
	matcher := &idleMatcher{ignored: map[string]bool{}}
	for n := 0; n < idleLineMemoLimit*2; n++ {
		matcher.ignores(strings.Repeat("x", n%50) + string(rune('a'+n%26)) + strings.Repeat("y", n/50))
	}
	if len(matcher.ignored) > idleLineMemoLimit {
		t.Fatalf("memo grew to %d entries, limit %d", len(matcher.ignored), idleLineMemoLimit)
	}
}
