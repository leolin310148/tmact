package main

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/leolin310148/tmact/internal/tmux"
)

func TestCapturePrintsOnlyCapturedText(t *testing.T) {
	resetCLIHooks := stubCLIHooks(t)
	defer resetCLIHooks()

	captureTmuxPaneInfo = func(target string) (tmux.CapturePaneInfo, error) {
		if target != "work:2.1" {
			t.Fatalf("metadata target = %q", target)
		}
		return tmux.CapturePaneInfo{Target: "work:2.1", PaneID: "%42", HistorySize: 80}, nil
	}
	captureTmuxPane = func(target string, lines int) (string, error) {
		if target != "%42" || lines != 40 {
			t.Fatalf("capture target = %q, lines = %d", target, lines)
		}
		return "first\n\nsecond\n", nil
	}

	out, err := captureRun(t, "capture", "--target", "work:2.1", "--lines", "40")
	if err != nil {
		t.Fatal(err)
	}
	if out != "first\n\nsecond\n" {
		t.Fatalf("output = %q", out)
	}
}

func TestCaptureNonEmptyOmitsBlankRows(t *testing.T) {
	resetCLIHooks := stubCLIHooks(t)
	defer resetCLIHooks()

	captureTmuxPaneInfo = func(string) (tmux.CapturePaneInfo, error) {
		return tmux.CapturePaneInfo{Target: "work:0.0", PaneID: "%7"}, nil
	}
	captureTmuxPane = func(string, int) (string, error) {
		return "first\n   \nsecond\n\n", nil
	}

	out, err := captureRun(t, "-t", "%7", "capture", "--non-empty")
	if err != nil {
		t.Fatal(err)
	}
	if out != "first\nsecond\n" {
		t.Fatalf("output = %q", out)
	}
}

func TestCaptureJSONIncludesCanonicalPaneAndTruncation(t *testing.T) {
	resetCLIHooks := stubCLIHooks(t)
	defer resetCLIHooks()

	captureTmuxPaneInfo = func(string) (tmux.CapturePaneInfo, error) {
		return tmux.CapturePaneInfo{Target: "work:3.2", PaneID: "%19", HistorySize: 121}, nil
	}
	captureTmuxPane = func(target string, lines int) (string, error) {
		if target != "%19" || lines != 120 {
			t.Fatalf("capture target = %q, lines = %d", target, lines)
		}
		return "done\n", nil
	}

	out, err := captureRun(t, "capture", "--target", "work:3.2", "--json")
	if err != nil {
		t.Fatal(err)
	}
	var report captureReport
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatal(err)
	}
	if report.Selector != "work:3.2" || report.Target != "work:3.2" || report.PaneID != "%19" {
		t.Fatalf("report target metadata = %#v", report)
	}
	if report.RequestedLines != 120 || report.Text != "done\n" || report.HistorySize != 121 || !report.Truncated {
		t.Fatalf("report capture metadata = %#v", report)
	}
	if report.Cursor == "" || !report.FullSnapshot || report.Reset || report.ResetReason != "" {
		t.Fatalf("report cursor metadata = %#v", report)
	}
}

func TestCaptureAfterReturnsIncrementalJSON(t *testing.T) {
	resetCLIHooks := stubCLIHooks(t)
	defer resetCLIHooks()

	historySize := 20
	captureTmuxPaneInfo = func(string) (tmux.CapturePaneInfo, error) {
		return tmux.CapturePaneInfo{Target: "work:0.0", PaneID: "%7", HistorySize: historySize}, nil
	}
	text := "one\ntwo\n"
	captureTmuxPane = func(string, int) (string, error) { return text, nil }

	initialOut, err := captureRun(t, "capture", "--target", "%7", "--json")
	if err != nil {
		t.Fatal(err)
	}
	var initial captureReport
	if err := json.Unmarshal([]byte(initialOut), &initial); err != nil {
		t.Fatal(err)
	}

	historySize = 21
	text = "one\ntwo\nthree\n"
	incrementalOut, err := captureRun(t, "capture", "--target", "%7", "--after", initial.Cursor, "--json")
	if err != nil {
		t.Fatal(err)
	}
	var incremental captureReport
	if err := json.Unmarshal([]byte(incrementalOut), &incremental); err != nil {
		t.Fatal(err)
	}
	if incremental.Text != "three\n" || incremental.FullSnapshot || incremental.Reset || incremental.Cursor == "" || incremental.Cursor == initial.Cursor {
		t.Fatalf("incremental report = %#v", incremental)
	}
}

func TestCaptureRejectsPeerBeforeLocalTmux(t *testing.T) {
	resetCLIHooks := stubCLIHooks(t)
	defer resetCLIHooks()

	captureTmuxPaneInfo = func(string) (tmux.CapturePaneInfo, error) {
		t.Fatal("peer capture must not inspect local tmux")
		return tmux.CapturePaneInfo{}, nil
	}
	captureTmuxPane = func(string, int) (string, error) {
		t.Fatal("peer capture must not capture local tmux")
		return "", nil
	}

	_, err := captureRun(t, "capture", "--target", "mini@%7")
	if err == nil || !strings.Contains(err.Error(), "does not support peer targets") {
		t.Fatalf("err = %v", err)
	}
}

func TestCaptureValidation(t *testing.T) {
	tests := [][]string{
		{"capture"},
		{"capture", "--target", "work"},
		{"capture", "--target", "work:0"},
		{"capture", "--target", "%7", "--lines", "0"},
		{"capture", "--target", "%7", "--after", "cursor"},
		{"capture", "--target", "%7", "--after", "", "--json"},
		{"-t", "%7", "capture", "--target", "%8"},
		{"capture", "--target", "%7", "extra"},
	}
	for _, args := range tests {
		if _, err := captureRun(t, args...); err == nil {
			t.Fatalf("expected error for %v", args)
		}
	}
}

func TestOmitBlankRows(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{input: "", want: ""},
		{input: "\n \t\n", want: ""},
		{input: "a\n\n b \n", want: "a\n b \n"},
		{input: "a\n\nb", want: "a\nb"},
	}
	for _, tt := range tests {
		if got := omitBlankRows(tt.input); got != tt.want {
			t.Fatalf("omitBlankRows(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}
