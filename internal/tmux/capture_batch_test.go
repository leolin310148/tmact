package tmux

import (
	"slices"
	"strings"
	"testing"
)

// fakeBatchRunner emulates tmux running a ;-separated command list: each
// capture-pane prints its pane's text, each display-message prints its
// message, and the first unknown pane aborts the rest of the list.
func fakeBatchRunner(panes map[string]string, calls *[][]string) func([]string) string {
	return func(args []string) string {
		*calls = append(*calls, args)
		var out strings.Builder
		var cmd []string
		flush := func() bool {
			defer func() { cmd = nil }()
			switch cmd[0] {
			case "capture-pane":
				text, ok := panes[cmd[2]]
				if !ok {
					return false
				}
				out.WriteString(text)
			case "display-message":
				out.WriteString(cmd[2] + "\n")
			}
			return true
		}
		for _, arg := range args {
			if arg == ";" {
				if !flush() {
					return out.String()
				}
				continue
			}
			cmd = append(cmd, arg)
		}
		if len(cmd) > 0 {
			flush()
		}
		return out.String()
	}
}

func TestCaptureBatchArgsChainsCapturesWithMarkers(t *testing.T) {
	args := captureBatchArgs([]CaptureSpec{
		{PaneID: "%1", Lines: 40, JoinWrapped: true},
		{PaneID: "%2", Lines: 40, Escapes: true},
	}, "n")
	want := []string{
		"capture-pane", "-t", "%1", "-p", "-S", "-40", "-J", ";", "display-message", "-p", "tmact-capture-n-0",
		";", "capture-pane", "-t", "%2", "-p", "-S", "-40", "-e", ";", "display-message", "-p", "tmact-capture-n-1",
	}
	if !slices.Equal(args, want) {
		t.Fatalf("args =\n%q\nwant\n%q", args, want)
	}
}

func TestCapturePanesBatchSplitsOutputPerPane(t *testing.T) {
	var calls [][]string
	panes := map[string]string{
		"%1": "one\ntwo\n",
		"%2": "\n\n",
		"%3": "three\n",
	}
	got := capturePanesBatch([]CaptureSpec{{PaneID: "%1"}, {PaneID: "%2"}, {PaneID: "%3"}}, fakeBatchRunner(panes, &calls))
	if len(calls) != 1 {
		t.Fatalf("tmux invocations = %d, want 1", len(calls))
	}
	for i, id := range []string{"%1", "%2", "%3"} {
		if got[i] != panes[id] {
			t.Fatalf("capture %d = %q, want %q", i, got[i], panes[id])
		}
	}
}

func TestCapturePanesBatchSkipsFailedPaneAndRetriesRest(t *testing.T) {
	var calls [][]string
	panes := map[string]string{"%1": "one\n", "%3": "three\n", "%4": "four\n"}
	specs := []CaptureSpec{{PaneID: "%1"}, {PaneID: "%gone"}, {PaneID: "%3"}, {PaneID: "%4"}}
	got := capturePanesBatch(specs, fakeBatchRunner(panes, &calls))
	if len(calls) != 2 {
		t.Fatalf("tmux invocations = %d, want 2", len(calls))
	}
	if _, ok := got[1]; ok {
		t.Fatal("failed pane should be missing so the caller falls back")
	}
	if got[0] != "one\n" || got[2] != "three\n" || got[3] != "four\n" {
		t.Fatalf("got %v", got)
	}
}

func TestCapturePanesBatchStopsWhenNothingComesBack(t *testing.T) {
	var calls [][]string
	got := capturePanesBatch([]CaptureSpec{{PaneID: "%1"}, {PaneID: "%2"}, {PaneID: "%3"}}, fakeBatchRunner(nil, &calls))
	if len(calls) != 1 || len(got) != 0 {
		t.Fatalf("calls=%d got=%v, want one call and no results", len(calls), got)
	}
}

func TestParseCaptureBatchIgnoresMarkerLookalikesInContent(t *testing.T) {
	out := "tmact-capture-x-0 inside\nreal\ntmact-capture-n-0\n"
	got := parseCaptureBatch(out, "n", 1)
	if len(got) != 1 || got[0] != "tmact-capture-x-0 inside\nreal\n" {
		t.Fatalf("got %q", got)
	}
}
