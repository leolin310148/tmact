package web

import (
	"fmt"
	"math/rand"
	"slices"
	"strings"
	"testing"
)

func applyLinePatch(buf []string, drop, from int, tail []string) []string {
	out := append([]string(nil), buf[drop:drop+from]...)
	return append(out, tail...)
}

func numberedLines(start, end int) []string {
	lines := make([]string, 0, end-start)
	for i := start; i < end; i++ {
		lines = append(lines, fmt.Sprintf("line %d", i))
	}
	return lines
}

func TestLinePatchPlainPrefix(t *testing.T) {
	drop, from, tail := linePatch([]string{"a", "b", "c"}, []string{"a", "b", "d", "e"}, true)
	if drop != 0 || from != 2 || strings.Join(tail, "|") != "d|e" {
		t.Fatalf("got drop=%d from=%d tail=%v, want 0/2/[d e]", drop, from, tail)
	}
}

func TestLinePatchSlidingWindowDropsHead(t *testing.T) {
	// A full capture window scrolled by one line and the bottom row (the
	// agent's status line) changed.
	prev := append(numberedLines(0, 2040), "status 1")
	next := append(numberedLines(1, 2041), "status 2")

	drop, from, tail := linePatch(prev, next, true)
	if drop != 1 || from != 2039 || strings.Join(tail, "|") != "line 2040|status 2" {
		t.Fatalf("got drop=%d from=%d tail=%v, want drop=1 from=2039 two-line tail", drop, from, tail)
	}
	if got := applyLinePatch(prev, drop, from, tail); !slices.Equal(got, next) {
		t.Fatal("patched buffer does not equal next capture")
	}
}

func TestLinePatchWithoutShiftKeepsLegacyFullResend(t *testing.T) {
	prev := numberedLines(0, 100)
	next := numberedLines(1, 101)
	drop, from, tail := linePatch(prev, next, false)
	if drop != 0 || from != 0 || len(tail) != 100 {
		t.Fatalf("got drop=%d from=%d tail=%d lines, want legacy 0/0/100", drop, from, len(tail))
	}
}

func TestLinePatchEmptyInputs(t *testing.T) {
	if drop, from, tail := linePatch(nil, []string{"a"}, true); drop != 0 || from != 0 || len(tail) != 1 {
		t.Fatalf("nil prev: got %d/%d/%v", drop, from, tail)
	}
	if drop, from, tail := linePatch([]string{"a", "b"}, nil, true); drop != 0 || from != 0 || len(tail) != 0 {
		t.Fatalf("nil next: got %d/%d/%v", drop, from, tail)
	}
}

func TestLinePatchReconstructsRandomInputs(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	// A tiny alphabet makes repeated lines (blank rows, box borders) common,
	// which is where a wrong alignment would show up.
	alphabet := []string{"", "", "─", "a", "b", "c"}
	randLines := func(n int) []string {
		lines := make([]string, n)
		for i := range lines {
			lines[i] = alphabet[rng.Intn(len(alphabet))]
		}
		return lines
	}
	for i := 0; i < 2000; i++ {
		prev := randLines(rng.Intn(40))
		var next []string
		switch rng.Intn(3) {
		case 0:
			next = randLines(rng.Intn(40))
		case 1: // scroll by k, then rewrite some bottom rows
			k := 0
			if len(prev) > 0 {
				k = rng.Intn(len(prev))
			}
			next = append(append([]string(nil), prev[k:]...), randLines(rng.Intn(5))...)
		default: // edit in place
			next = append([]string(nil), prev...)
			if len(next) > 0 {
				next[rng.Intn(len(next))] = "x"
			}
		}
		drop, from, tail := linePatch(prev, next, true)
		if got := applyLinePatch(prev, drop, from, tail); !slices.Equal(got, next) {
			t.Fatalf("case %d: prev=%q next=%q drop=%d from=%d tail=%q rebuilt=%q", i, prev, next, drop, from, tail, got)
		}
		if _, plainFrom, _ := linePatch(prev, next, false); len(tail) > len(next)-plainFrom {
			t.Fatalf("case %d: shift patch sent %d lines, more than the plain %d", i, len(tail), len(next)-plainFrom)
		}
	}
}
