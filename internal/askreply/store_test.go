package askreply

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestReplyUnblocksWaitAndPreservesMultilineText(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	store.Random = bytes.NewReader(make([]byte, 16))
	store.PollInterval = time.Millisecond
	request, err := store.Create("answerer", "/repo", "codex", "%3", time.Minute)
	if err != nil {
		t.Fatal(err)
	}

	result := make(chan Reply, 1)
	waitErr := make(chan error, 1)
	go func() {
		reply, err := store.Wait(context.Background(), request.ID)
		result <- reply
		waitErr <- err
	}()

	text := "investigation complete\nall tests pass"
	written, err := store.Reply(request.ID, text)
	if err != nil {
		t.Fatal(err)
	}
	if written.Text != text {
		t.Fatalf("written reply = %#v", written)
	}
	select {
	case reply := <-result:
		if err := <-waitErr; err != nil {
			t.Fatal(err)
		}
		if reply.Text != text || reply.QuestionID != request.ID {
			t.Fatalf("reply = %#v", reply)
		}
	case <-time.After(time.Second):
		t.Fatal("wait did not receive reply")
	}
}

func TestReplyIsOneShot(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	request, err := store.Create("answerer", "/repo", "claude", "", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Reply(request.ID, "first"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Reply(request.ID, "second"); !errors.Is(err, ErrAlreadyFinalized) {
		t.Fatalf("second reply error = %v", err)
	}
}

func TestWaitRetriesOutcomeWhileExclusiveWriterFinishes(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	store.PollInterval = time.Millisecond
	request, err := store.Create("answerer", "/repo", "codex", "", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(store.questionDir(request.ID), outcomeFile)
	if err := os.WriteFile(path, []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}

	result := make(chan Reply, 1)
	waitErr := make(chan error, 1)
	go func() {
		reply, err := store.Wait(context.Background(), request.ID)
		result <- reply
		waitErr <- err
	}()
	time.Sleep(5 * time.Millisecond)
	finished := outcome{
		Version: Version, Kind: "reply", QuestionID: request.ID,
		Text: "complete", CreatedAt: time.Now(),
	}
	data, err := json.Marshal(finished)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	select {
	case reply := <-result:
		if err := <-waitErr; err != nil {
			t.Fatal(err)
		}
		if reply.Text != "complete" {
			t.Fatalf("reply = %#v", reply)
		}
	case <-time.After(time.Second):
		t.Fatal("wait did not retry incomplete outcome")
	}
}

func TestWaitCancellationRejectsLateReply(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	store.PollInterval = time.Millisecond
	request, err := store.Create("answerer", "/repo", "codex", "", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := store.Wait(ctx, request.ID); !errors.Is(err, context.Canceled) {
		t.Fatalf("wait error = %v", err)
	}
	if _, err := store.Reply(request.ID, "late"); !errors.Is(err, ErrAlreadyFinalized) {
		t.Fatalf("late reply error = %v", err)
	}
}

func TestExpiredQuestionCannotBeRepliedTo(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 7, 29, 12, 0, 0, 0, time.UTC)
	store.Now = func() time.Time { return now }
	request, err := store.Create("answerer", "/repo", "codex", "", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	now = now.Add(2 * time.Second)
	if _, err := store.Reply(request.ID, "late"); !errors.Is(err, ErrExpired) {
		t.Fatalf("reply error = %v", err)
	}
}

func TestRequestPersistsOnlyMetadataWithPrivateModes(t *testing.T) {
	root := filepath.Join(t.TempDir(), "mailbox")
	store, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	request, err := store.Create("answerer", "/repo", "gemini", "%8", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	questionDir := filepath.Join(root, request.ID)
	dirInfo, err := os.Stat(questionDir)
	if err != nil {
		t.Fatal(err)
	}
	if got := dirInfo.Mode().Perm(); got != 0o700 {
		t.Fatalf("question dir mode = %o", got)
	}
	path := filepath.Join(questionDir, requestFile)
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("request mode = %o", got)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "secret prompt") {
		t.Fatalf("request unexpectedly persisted prompt: %s", data)
	}
}

func TestDefaultStoreUsesOwnedPrivateRuntimeDirectory(t *testing.T) {
	tempRoot := t.TempDir()
	t.Setenv("TMPDIR", tempRoot)
	store, err := New("")
	if err != nil {
		t.Fatal(err)
	}
	wantRoot := filepath.Join(tempRoot, "tmact-"+strconv.Itoa(os.Getuid()))
	if store.Dir != filepath.Join(wantRoot, "asks") {
		t.Fatalf("default store = %s", store.Dir)
	}
	request, err := store.Create("answerer", "/repo", "codex", "", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{wantRoot, store.Dir, store.questionDir(request.ID)} {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if got := info.Mode().Perm(); got != 0o700 {
			t.Fatalf("%s mode = %o", path, got)
		}
	}
}

func TestPrivateRuntimeDirectoryRejectsSymlink(t *testing.T) {
	parent := t.TempDir()
	realDir := filepath.Join(parent, "real")
	if err := os.Mkdir(realDir, 0o700); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(parent, "runtime")
	if err := os.Symlink(realDir, link); err != nil {
		t.Fatal(err)
	}
	if err := ensurePrivateRuntimeDir(filepath.Join(link, "asks")); err == nil || !strings.Contains(err.Error(), "not a real directory") {
		t.Fatalf("error = %v", err)
	}
}

func TestInvalidQuestionIDCannotEscapeStore(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Reply("../../other", "answer"); err == nil || !strings.Contains(err.Error(), "invalid question id") {
		t.Fatalf("error = %v", err)
	}
}
