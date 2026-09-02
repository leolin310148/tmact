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

func newTestStore(t *testing.T) *Store {
	t.Helper()
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	store.PollInterval = time.Millisecond
	return store
}

func askerPrompt(text string) Message {
	return Message{From: RoleAsker, Kind: KindPrompt, Text: text, Delivery: DeliveryMailbox}
}

func TestReplyUnblocksAwaitAnswerAndPreservesMultilineText(t *testing.T) {
	store := newTestStore(t)
	store.Random = bytes.NewReader(make([]byte, 16))
	request, err := store.Create("answerer", "/repo", "codex", "%3", time.Minute)
	if err != nil {
		t.Fatal(err)
	}

	result := make(chan Message, 1)
	waitErr := make(chan error, 1)
	go func() {
		reply, err := store.AwaitAnswer(context.Background(), request.ID, 0)
		result <- reply
		waitErr <- err
	}()

	text := "investigation complete\nall tests pass"
	written, err := store.Reply(request.ID, text)
	if err != nil {
		t.Fatal(err)
	}
	if written.Text != text || written.Seq != 1 || written.Kind != KindAnswer || written.From != RoleAnswerer {
		t.Fatalf("written reply = %#v", written)
	}
	select {
	case reply := <-result:
		if err := <-waitErr; err != nil {
			t.Fatal(err)
		}
		if reply.Text != text || reply.QuestionID != request.ID || reply.Seq != 1 {
			t.Fatalf("reply = %#v", reply)
		}
	case <-time.After(time.Second):
		t.Fatal("wait did not receive reply")
	}
	thread, err := store.Load(request.ID)
	if err != nil {
		t.Fatal(err)
	}
	if thread.Closed != nil {
		t.Fatalf("an ordinary answer must leave the thread open: %#v", thread.Closed)
	}
}

func TestThreadCarriesMultipleRoundsInBothDirections(t *testing.T) {
	store := newTestStore(t)
	request, err := store.Create("answerer", "/repo", "claude", "", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Post(request.ID, Message{From: RoleAsker, Kind: KindPrompt, Text: "secret task", Delivery: DeliveryPane, Target: "%7"}, time.Time{}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Reply(request.ID, "first answer"); err != nil {
		t.Fatal(err)
	}
	first, err := store.AwaitAnswer(context.Background(), request.ID, 0)
	if err != nil || first.Seq != 2 || first.Text != "first answer" {
		t.Fatalf("first = %#v, %v", first, err)
	}

	// Answerer waits for a follow-up; asker delivers it through the mailbox.
	followUp := make(chan Message, 1)
	waitErr := make(chan error, 1)
	go func() {
		message, err := store.AwaitPrompt(context.Background(), request.ID, 0)
		followUp <- message
		waitErr <- err
	}()
	deadline := time.Now()
	for {
		has, err := store.HasWaiter(request.ID)
		if err != nil {
			t.Fatal(err)
		}
		if has {
			break
		}
		if time.Since(deadline) > time.Second {
			t.Fatal("waiter never registered")
		}
		time.Sleep(time.Millisecond)
	}
	if _, err := store.Post(request.ID, askerPrompt("please also check the tests"), time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	select {
	case message := <-followUp:
		if err := <-waitErr; err != nil {
			t.Fatal(err)
		}
		if message.Seq != 3 || message.Text != "please also check the tests" || message.From != RoleAsker {
			t.Fatalf("follow-up = %#v", message)
		}
	case <-time.After(time.Second):
		t.Fatal("answerer did not receive follow-up")
	}

	// Answerer asks a clarifying question; the asker sees its kind.
	if _, err := store.Post(request.ID, Message{From: RoleAnswerer, Kind: KindQuestion, Text: "which branch?", Delivery: DeliveryMailbox}, time.Time{}); err != nil {
		t.Fatal(err)
	}
	second, err := store.AwaitAnswer(context.Background(), request.ID, first.Seq)
	if err != nil || second.Seq != 4 || second.Kind != KindQuestion {
		t.Fatalf("second = %#v, %v", second, err)
	}

	thread, err := store.Load(request.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got := thread.LastSeq(RoleAsker); got != 3 {
		t.Fatalf("asker last seq = %d", got)
	}
	if got := thread.LastTarget(); got != "%7" {
		t.Fatalf("last target = %q", got)
	}
	if thread.Messages[0].Text != "" {
		t.Fatalf("pane-delivered prompt persisted text: %#v", thread.Messages[0])
	}
	if !thread.ExpiresAt.After(request.ExpiresAt) {
		t.Fatalf("asker follow-up did not extend the deadline: %s vs %s", thread.ExpiresAt, request.ExpiresAt)
	}
}

func TestFinalReplyClosesThreadForBothSides(t *testing.T) {
	store := newTestStore(t)
	request, err := store.Create("answerer", "/repo", "claude", "", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Post(request.ID, Message{From: RoleAnswerer, Kind: KindFinal, Text: "done", Delivery: DeliveryMailbox}, time.Time{}); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(request.ID, RoleAnswerer, "final reply"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Reply(request.ID, "second"); !errors.Is(err, ErrAlreadyFinalized) {
		t.Fatalf("second reply error = %v", err)
	}
	if _, err := store.Post(request.ID, askerPrompt("more?"), time.Time{}); !errors.Is(err, ErrAlreadyFinalized) {
		t.Fatalf("asker follow-up error = %v", err)
	}
	if err := store.Close(request.ID, RoleAsker, "again"); !errors.Is(err, ErrAlreadyFinalized) {
		t.Fatalf("second close error = %v", err)
	}
	// The final message itself is still delivered to a waiting asker.
	message, err := store.AwaitAnswer(context.Background(), request.ID, 0)
	if err != nil || message.Kind != KindFinal {
		t.Fatalf("final = %#v, %v", message, err)
	}
	// An answerer waiting after the close is released immediately.
	if _, err := store.AwaitPrompt(context.Background(), request.ID, 1); !errors.Is(err, ErrClosed) {
		t.Fatalf("await prompt on closed thread = %v", err)
	}
}

func TestAwaitAnswerRetriesMessageWhileExclusiveWriterFinishes(t *testing.T) {
	store := newTestStore(t)
	request, err := store.Create("answerer", "/repo", "codex", "", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	path := store.messagePath(request.ID, 1)
	if err := os.WriteFile(path, []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}

	result := make(chan Message, 1)
	waitErr := make(chan error, 1)
	go func() {
		reply, err := store.AwaitAnswer(context.Background(), request.ID, 0)
		result <- reply
		waitErr <- err
	}()
	time.Sleep(5 * time.Millisecond)
	finished := messageRecord{Version: Version, Message: Message{
		QuestionID: request.ID, Seq: 1, From: RoleAnswerer, Kind: KindAnswer,
		Text: "complete", Delivery: DeliveryMailbox, CreatedAt: time.Now(),
	}}
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
		t.Fatal("wait did not retry incomplete message")
	}
}

func TestAwaitAnswerCancellationRejectsLateReply(t *testing.T) {
	store := newTestStore(t)
	request, err := store.Create("answerer", "/repo", "codex", "", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := store.AwaitAnswer(ctx, request.ID, 0); !errors.Is(err, context.Canceled) {
		t.Fatalf("wait error = %v", err)
	}
	if _, err := store.Reply(request.ID, "late"); !errors.Is(err, ErrAlreadyFinalized) {
		t.Fatalf("late reply error = %v", err)
	}
}

func TestAwaitPromptTimeoutKeepsThreadOpen(t *testing.T) {
	store := newTestStore(t)
	request, err := store.Create("answerer", "/repo", "codex", "", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	if _, err := store.AwaitPrompt(ctx, request.ID, 0); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("await prompt error = %v", err)
	}
	has, err := store.HasWaiter(request.ID)
	if err != nil || has {
		t.Fatalf("waiter after timeout = %v, %v", has, err)
	}
	if _, err := store.Reply(request.ID, "still open"); err != nil {
		t.Fatalf("reply after answerer timeout = %v", err)
	}
}

func TestHasWaiterIgnoresDeadOrExpiredWaiter(t *testing.T) {
	store := newTestStore(t)
	request, err := store.Create("answerer", "/repo", "codex", "", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(store.questionDir(request.ID), waiterFile)
	alive := true
	store.ProcessAlive = func(int) bool { return alive }

	if err := writePrivateJSON(path, waiterRecord{PID: 4242, Until: time.Now().Add(time.Minute)}); err != nil {
		t.Fatal(err)
	}
	if has, err := store.HasWaiter(request.ID); err != nil || !has {
		t.Fatalf("live waiter = %v, %v", has, err)
	}

	alive = false
	if has, err := store.HasWaiter(request.ID); err != nil || has {
		t.Fatalf("dead waiter = %v, %v", has, err)
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("stale waiter marker not removed: %v", err)
	}

	alive = true
	if err := writePrivateJSON(path, waiterRecord{PID: 4242, Until: time.Now().Add(-time.Second)}); err != nil {
		t.Fatal(err)
	}
	if has, err := store.HasWaiter(request.ID); err != nil || has {
		t.Fatalf("expired waiter = %v, %v", has, err)
	}
}

func TestExpiredQuestionCannotBeRepliedTo(t *testing.T) {
	store := newTestStore(t)
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

func TestAskerFollowUpExtendsExpiry(t *testing.T) {
	store := newTestStore(t)
	now := time.Date(2026, 7, 29, 12, 0, 0, 0, time.UTC)
	store.Now = func() time.Time { return now }
	request, err := store.Create("answerer", "/repo", "codex", "", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Post(request.ID, askerPrompt("more"), now.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	now = now.Add(2 * time.Second)
	if _, err := store.Reply(request.ID, "in time"); err != nil {
		t.Fatalf("reply within extended deadline = %v", err)
	}
}

func TestPostRejectsInvalidRolesAndKinds(t *testing.T) {
	store := newTestStore(t)
	request, err := store.Create("answerer", "/repo", "codex", "", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	cases := []Message{
		{From: "bystander", Kind: KindAnswer, Text: "x", Delivery: DeliveryMailbox},
		{From: RoleAsker, Kind: KindAnswer, Text: "x", Delivery: DeliveryMailbox},
		{From: RoleAnswerer, Kind: KindPrompt, Text: "x", Delivery: DeliveryMailbox},
		{From: RoleAnswerer, Kind: KindAnswer, Text: "x", Delivery: DeliveryPane},
		{From: RoleAnswerer, Kind: KindAnswer, Text: "   ", Delivery: DeliveryMailbox},
	}
	for _, message := range cases {
		if _, err := store.Post(request.ID, message, time.Time{}); err == nil {
			t.Fatalf("post accepted %#v", message)
		}
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
	if _, err := store.Post(request.ID, Message{From: RoleAsker, Kind: KindPrompt, Text: "secret prompt", Delivery: DeliveryPane, Target: "%8"}, time.Time{}); err != nil {
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
	for _, name := range []string{requestFile, messagePrefix + "000001" + messageSuffix} {
		path := filepath.Join(questionDir, name)
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if got := info.Mode().Perm(); got != 0o600 {
			t.Fatalf("%s mode = %o", name, got)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(data), "secret prompt") {
			t.Fatalf("%s unexpectedly persisted prompt: %s", name, data)
		}
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
	store := newTestStore(t)
	if _, err := store.Reply("../../other", "answer"); err == nil || !strings.Contains(err.Error(), "invalid question id") {
		t.Fatalf("error = %v", err)
	}
	if _, err := store.Load("../../other"); err == nil || !strings.Contains(err.Error(), "invalid question id") {
		t.Fatalf("load error = %v", err)
	}
}
