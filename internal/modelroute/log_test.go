package modelroute

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestRoutingLogRoundTripPrivacyAndCorrelation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "private", "routing.jsonl")
	t.Setenv("TMACT_ROUTING_LOG", path)
	id := NewDispatchID()
	entry := LogEntry{Event: "decision", Trace: Trace{DispatchID: id}, Input: &Input{Task: "private wrapper", Files: []TaskFile{{Path: "task.md", Status: "attached", Content: "private file", SHA256: "hash"}}}, Response: json.RawMessage(`{"private":"response"}`), Selection: &Selection{Difficulty: "advanced", Files: []TaskFile{{Content: "private file"}}}}
	if err := AppendLog(entry); err != nil {
		t.Fatal(err)
	}
	if err := AppendLog(LogEntry{Event: "dispatch", Trace: Trace{DispatchID: id}, Outcome: &Outcome{Status: "dispatched"}}); err != nil {
		t.Fatal(err)
	}
	if info, err := os.Stat(path); err != nil || info.Mode().Perm() != 0600 {
		t.Fatalf("permissions=%v err=%v", info, err)
	}
	entries, err := ReadLog(id, 50, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 || entries[0].Event != "dispatch" || entries[1].Selection.Difficulty != "advanced" {
		t.Fatal(entries)
	}
	redacted, _ := json.Marshal(entries)
	if strings.Contains(string(redacted), "private wrapper") || strings.Contains(string(redacted), "private file") || strings.Contains(string(redacted), "response") {
		t.Fatalf("private content leaked: %s", redacted)
	}
	entries, err = ReadLog(id, 50, true)
	if err != nil {
		t.Fatal(err)
	}
	if entries[1].Input.Files[0].Content != "private file" || entries[1].Input.Task != "private wrapper" {
		t.Fatal("snapshots not retained")
	}
	if got, err := ReadLog("different", 50, true); err != nil || len(got) != 0 {
		t.Fatalf("filter=%v err=%v", got, err)
	}
	if got, err := ReadLog(id, 1, false); err != nil || len(got) != 1 || got[0].Event != "dispatch" {
		t.Fatalf("limit=%v err=%v", got, err)
	}
}

func TestRoutingLogConcurrentAppendAndSymlinkRefusal(t *testing.T) {
	path := filepath.Join(t.TempDir(), "routing.jsonl")
	t.Setenv("TMACT_ROUTING_LOG", path)
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := AppendLog(LogEntry{Event: "dispatch", Trace: Trace{DispatchID: NewDispatchID()}}); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	entries, err := ReadLog("", 50, true)
	if err != nil || len(entries) != 20 {
		t.Fatalf("records=%d err=%v", len(entries), err)
	}
	target := filepath.Join(t.TempDir(), "target")
	if err := os.WriteFile(target, []byte("preserve"), 0600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TMACT_ROUTING_LOG", link)
	if err := AppendLog(LogEntry{}); err == nil {
		t.Fatal("log symlink followed")
	}
	data, _ := os.ReadFile(target)
	if string(data) != "preserve" {
		t.Fatal("symlink target modified")
	}
}

func TestChooseLogsActualAttachedInputAndIndependentDifficulty(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "routing.jsonl")
	inputPath := filepath.Join(dir, "input.json")
	t.Setenv("TMACT_ROUTING_LOG", logPath)
	t.Setenv("DECIDE_CLI_CONFIG", "none")
	t.Setenv("ROUTING_TEST_INPUT", inputPath)
	script := `#!/bin/sh
cat > "$ROUTING_TEST_INPUT"
printf '%s\n' '{"model":"fake-decide","answers":{"route":{"choice":"profile_1"},"difficulty":{"choice":"advanced"}},"confidences":{"route":0.1,"difficulty":0.3}}'
`
	if err := os.WriteFile(filepath.Join(dir, "decide"), []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	if err := os.WriteFile(filepath.Join(dir, "task.md"), []byte("Investigate cross-process data loss and crash recovery."), 0600); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	in, err := Prepare("Read task.md and do it.", "codex", "", "", quotaSnapshot(now, 1, 2, 3), now)
	if err != nil {
		t.Fatal(err)
	}
	in.Trace = Trace{DispatchID: NewDispatchID(), Command: "dispatch-work", Execute: false}
	AttachPromptFiles(&in, dir)
	selection, err := Choose(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	if selection.Difficulty != "advanced" || selection.SelectedTier != "standard" || selection.Confidence != .1 {
		t.Fatalf("independent metadata=%+v", selection)
	}
	if len(selection.Files) != 1 || selection.Files[0].Content != "" {
		t.Fatal("report must contain only file metadata")
	}
	b, _ := os.ReadFile(inputPath)
	var supplied Input
	if err := json.Unmarshal(b, &supplied); err != nil {
		t.Fatal(err)
	}
	if supplied.Files[0].Content != in.Files[0].Content || supplied.Task != in.Task {
		t.Fatal("decide did not receive full file context")
	}
	entries, err := ReadLog(in.Trace.DispatchID, 50, true)
	if err != nil || len(entries) != 1 {
		t.Fatalf("entries=%v err=%v", entries, err)
	}
	if entries[0].Input.Files[0].Content != in.Files[0].Content || entries[0].PolicyVersion == "" || entries[0].PolicySHA256 == "" || !strings.Contains(string(entries[0].Questions), "difficulty") {
		t.Fatal("incomplete decision audit record")
	}
}

func TestLogWriteFailureDoesNotBlockValidSelection(t *testing.T) {
	dir := t.TempDir()
	blocked := filepath.Join(dir, "not-a-directory")
	if err := os.WriteFile(blocked, []byte("preserve"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TMACT_ROUTING_LOG", filepath.Join(blocked, "routing.jsonl"))
	t.Setenv("DECIDE_CLI_CONFIG", "none")
	script := `#!/bin/sh
cat >/dev/null
printf '%s\n' '{"answers":{"route":{"choice":"profile_0"}},"confidences":{"route":0.1}}'
`
	if err := os.WriteFile(filepath.Join(dir, "decide"), []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	selection, err := Choose(context.Background(), Input{Candidates: []Candidate{{Agent: "codex", Model: "gpt-6.1-sol", Effort: "medium"}}})
	if err != nil || selection.Model != "gpt-6.1-sol" {
		t.Fatalf("selection=%+v err=%v", selection, err)
	}
}
