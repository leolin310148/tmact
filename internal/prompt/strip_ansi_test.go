package prompt

import (
	"math/rand"
	"strings"
	"testing"
)

// stripANSIReference is the original copy-every-byte implementation; the fast
// path must stay output-identical to it.
func stripANSIReference(text string) string {
	var b strings.Builder
	for i := 0; i < len(text); i++ {
		c := text[i]
		if c != 0x1b {
			b.WriteByte(c)
			continue
		}
		i++
		if i >= len(text) {
			break
		}
		switch text[i] {
		case '[':
			for i+1 < len(text) {
				i++
				if text[i] >= 0x40 && text[i] <= 0x7e {
					break
				}
			}
		case ']':
			for i+1 < len(text) {
				i++
				if text[i] == 0x07 {
					break
				}
				if text[i] == 0x1b && i+1 < len(text) && text[i+1] == '\\' {
					i++
					break
				}
			}
		case '(', ')', '*', '+', '-', '.', '/', '#':
			if i+1 < len(text) {
				i++
			}
		}
	}
	return b.String()
}

func TestStripANSIMatchesReference(t *testing.T) {
	cases := []string{
		"",
		"plain line › with unicode │",
		"\x1b[0;1m›\x1b[0m \x1b[2mWrite tests\x1b[0m",
		"prefix \x1b]8;;http://x\x07link\x1b]8;;\x1b\\ tail",
		"\x1b(Bcharset",
		"trailing esc \x1b",
		"trailing csi \x1b[",
	}
	pieces := []string{"a", "│", " ", "\x1b", "[", "]", "m", "0", ";", "\x07", "\\", "(", "B", "›"}
	rng := rand.New(rand.NewSource(1))
	for n := 0; n < 5000; n++ {
		var b strings.Builder
		for k := rng.Intn(20); k > 0; k-- {
			b.WriteString(pieces[rng.Intn(len(pieces))])
		}
		cases = append(cases, b.String())
	}
	for _, c := range cases {
		if got, want := stripANSI(c), stripANSIReference(c); got != want {
			t.Fatalf("stripANSI(%q) = %q, want %q", c, got, want)
		}
	}
}

func TestStripANSIPlainTextDoesNotAllocate(t *testing.T) {
	line := strings.Repeat("plain captured line ", 8)
	if allocs := testing.AllocsPerRun(100, func() { _ = stripANSI(line) }); allocs != 0 {
		t.Fatalf("allocs = %v, want 0 for text without escapes", allocs)
	}
}
