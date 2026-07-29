package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestFeedbackAddRecordsPrivateJSONL(t *testing.T) {
	defer stubCLIHooks(t)()
	home := t.TempDir()
	t.Setenv("HOME", home)
	fixedTime := time.Date(2026, 7, 29, 4, 5, 6, 789, time.FixedZone("test", 8*60*60))
	tmactNow = func() time.Time { return fixedTime }
	oldVersion := version
	version = "v1.2.3"
	defer func() { version = oldVersion }()

	out, err := captureRun(
		t,
		"feedback",
		"--category", "feature",
		"want", "peer", "wait",
		"--command", "wait",
	)
	if err != nil {
		t.Fatal(err)
	}

	path := filepath.Join(home, ".tmact", feedbackFileName)
	if !strings.Contains(out, path) {
		t.Fatalf("output = %q, want path %q", out, path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Count(string(data), "\n"); got != 1 {
		t.Fatalf("feedback file has %d lines: %q", got, data)
	}
	var entry feedbackEntry
	if err := json.Unmarshal(data, &entry); err != nil {
		t.Fatal(err)
	}
	if !entry.Time.Equal(fixedTime) {
		t.Fatalf("time = %s, want %s", entry.Time, fixedTime)
	}
	if entry.Version != "v1.2.3" || entry.Category != "feature" || entry.Command != "wait" || entry.Message != "want peer wait" {
		t.Fatalf("entry = %#v", entry)
	}

	fileInfo, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := fileInfo.Mode().Perm(); got != 0o600 {
		t.Fatalf("feedback mode = %o, want 600", got)
	}
	dirInfo, err := os.Stat(filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}
	if got := dirInfo.Mode().Perm(); got != 0o700 {
		t.Fatalf("feedback dir mode = %o, want 700", got)
	}
}

func TestFeedbackListReturnsNewestEntriesOldestFirst(t *testing.T) {
	defer stubCLIHooks(t)()
	home := t.TempDir()
	t.Setenv("HOME", home)
	nextTime := time.Date(2026, 7, 29, 1, 0, 0, 0, time.UTC)
	tmactNow = func() time.Time {
		current := nextTime
		nextTime = nextTime.Add(time.Minute)
		return current
	}

	for _, message := range []string{"first", "second", "third"} {
		if _, err := captureRun(t, "feedback", message); err != nil {
			t.Fatal(err)
		}
	}
	out, err := captureRun(t, "feedback", "list", "--limit", "2", "--json")
	if err != nil {
		t.Fatal(err)
	}
	var entries []feedbackEntry
	if err := json.Unmarshal([]byte(out), &entries); err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 || entries[0].Message != "second" || entries[1].Message != "third" {
		t.Fatalf("entries = %#v", entries)
	}
	if entries[0].Category != defaultFeedbackCategory {
		t.Fatalf("default category = %q, want %q", entries[0].Category, defaultFeedbackCategory)
	}
}

func TestFeedbackExplicitAddAllowsReservedMessages(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	if _, err := captureRun(t, "feedback", "add", "list", "output", "is", "unclear", "--command=feedback"); err != nil {
		t.Fatal(err)
	}
	if _, err := captureRun(t, "feedback", "add", "help", "output", "is", "unclear", "--command=feedback"); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(home, ".tmact", feedbackFileName))
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 2 {
		t.Fatalf("feedback lines = %q", lines)
	}
	var entries []feedbackEntry
	for _, line := range lines {
		var entry feedbackEntry
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			t.Fatal(err)
		}
		entries = append(entries, entry)
	}
	if entries[0].Message != "list output is unclear" || entries[1].Message != "help output is unclear" {
		t.Fatalf("entries = %#v", entries)
	}
	for _, entry := range entries {
		if entry.Command != "feedback" {
			t.Fatalf("entry = %#v", entry)
		}
	}
}

func TestFeedbackAddHelpFlagShowsHelp(t *testing.T) {
	out, err := captureRun(t, "feedback", "add", "--help")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "tmact feedback MESSAGE") {
		t.Fatalf("help output = %q", out)
	}
}

func TestFeedbackPathDoesNotCreateFile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	path := filepath.Join(home, ".tmact", feedbackFileName)

	out, err := captureRun(t, "feedback", "path")
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(out) != path {
		t.Fatalf("path output = %q, want %q", out, path)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("path command created feedback file or returned unexpected error: %v", err)
	}

	out, err = captureRun(t, "feedback", "list", "--json")
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(out) != "[]" {
		t.Fatalf("empty JSON list = %q, want []", out)
	}
}

func TestFeedbackValidatesInput(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "missing message", args: []string{"feedback", "--category", "bug"}, want: "message is required"},
		{name: "bad category", args: []string{"feedback", "message", "--category", "other"}, want: "must be one of"},
		{name: "missing category value", args: []string{"feedback", "message", "--category"}, want: "requires a value"},
		{name: "unknown flag", args: []string{"feedback", "message", "--upload"}, want: "unknown feedback flag"},
		{name: "invalid limit", args: []string{"feedback", "list", "--limit", "0"}, want: "greater than zero"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := captureRun(t, tt.args...)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want containing %q", err, tt.want)
			}
		})
	}
}

func TestFeedbackHelpAndCatalog(t *testing.T) {
	out, err := captureRun(t, "feedback", "--help")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"tmact feedback MESSAGE", "ux, bug, feature, docs, or perf", "~/.tmact/feedback.jsonl", "never uploaded"} {
		if !strings.Contains(out, want) {
			t.Fatalf("help output missing %q: %s", want, out)
		}
	}

	help, ok := commandHelpFor("feedback")
	if !ok {
		t.Fatal("feedback missing from command catalog")
	}
	if len(help.Examples) < 3 || len(help.Safety) == 0 || len(help.Notes) < 2 {
		t.Fatalf("feedback help is too sparse: %#v", help)
	}
}
