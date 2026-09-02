package main

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/leolin310148/tmact/internal/askreply"
	"github.com/leolin310148/tmact/internal/dispatch"
)

var questionIDInPrompt = regexp.MustCompile(`q_[a-z2-7]{26}`)

func TestAskDispatchesProtocolAndReturnsExplicitReply(t *testing.T) {
	defer stubCLIHooks(t)()
	dir := t.TempDir()
	storeDir := t.TempDir()

	dispatchRun = func(opts dispatch.Options) (dispatch.Report, error) {
		if !opts.Execute || opts.Wait || opts.NoClear {
			t.Fatalf("dispatch opts = %#v", opts)
		}
		if !strings.Contains(opts.Prompt, "tmact reply protocol (required)") ||
			!strings.Contains(opts.Prompt, "do not merely mention the command") ||
			!strings.Contains(opts.Prompt, "--wait") {
			t.Fatalf("prompt missing reply protocol: %s", opts.Prompt)
		}
		id := questionIDInPrompt.FindString(opts.Prompt)
		if id == "" {
			t.Fatalf("prompt missing question id: %s", opts.Prompt)
		}
		store, err := askreply.New(storeDir)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := store.Reply(id, "root cause found\nfix verified"); err != nil {
			t.Fatal(err)
		}
		return dispatch.Report{
			Session: opts.Session, Target: "%9", Dir: opts.Dir,
			Agent: opts.Agent, Prompt: opts.Prompt, Execute: true,
			Steps: []dispatch.Step{{Name: "send-prompt", Status: dispatch.StatusOK}},
		}, nil
	}

	out, err := captureRun(t, "ask", "answerer", "--dir", dir, "--agent", "codex", "--prompt", "investigate", "--store-dir", storeDir, "--execute", "--json")
	if err != nil {
		t.Fatal(err)
	}
	var report askReport
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatal(err)
	}
	if report.Status != "answered" || report.Reply == nil || report.Reply.Text != "root cause found\nfix verified" {
		t.Fatalf("report = %#v", report)
	}
	if report.QuestionID == "" || report.Reply.QuestionID != report.QuestionID {
		t.Fatalf("question mapping = %#v", report)
	}
	if report.Prompt != "investigate" || report.Delivery != askreply.DeliveryPane {
		t.Fatalf("original prompt/delivery = %#v", report)
	}
	if report.AnswererWaiting || report.Closed {
		t.Fatalf("an ordinary answer must leave the question open: %#v", report)
	}
	if !strings.Contains(report.FollowUpCommand, "tmact ask --thread "+report.QuestionID) {
		t.Fatalf("follow-up command = %q", report.FollowUpCommand)
	}

	store, err := askreply.New(storeDir)
	if err != nil {
		t.Fatal(err)
	}
	thread, err := store.Load(report.QuestionID)
	if err != nil {
		t.Fatal(err)
	}
	if thread.Closed != nil || thread.LastTarget() != "%9" {
		t.Fatalf("thread = %#v", thread)
	}
}

func TestAskDryRunCreatesNoMailbox(t *testing.T) {
	defer stubCLIHooks(t)()
	storeDir := filepath.Join(t.TempDir(), "mailbox")
	dispatchRun = func(opts dispatch.Options) (dispatch.Report, error) {
		if opts.Execute {
			t.Fatal("dry-run unexpectedly executed")
		}
		if !strings.Contains(opts.Prompt, "<question-id>") {
			t.Fatalf("dry-run prompt = %s", opts.Prompt)
		}
		return dispatch.Report{
			Session: opts.Session, Dir: opts.Dir, Agent: opts.Agent, Prompt: opts.Prompt,
			Steps: []dispatch.Step{{Name: "send-prompt", Status: dispatch.StatusPlanned}},
		}, nil
	}
	out, err := captureRun(t, "ask", "answerer", "--dir", t.TempDir(), "--agent", "claude", "--prompt", "review", "--store-dir", storeDir)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "dry-run: ask <question-id>") {
		t.Fatalf("output = %s", out)
	}
	if matches, _ := filepath.Glob(filepath.Join(storeDir, "*")); len(matches) != 0 {
		t.Fatalf("dry-run created mailbox entries: %v", matches)
	}
}

func TestAskTimeoutPrintsQuestionBeforeReturningError(t *testing.T) {
	defer stubCLIHooks(t)()
	storeDir := t.TempDir()
	dispatchRun = func(opts dispatch.Options) (dispatch.Report, error) {
		return dispatch.Report{Session: opts.Session, Dir: opts.Dir, Agent: opts.Agent, Prompt: opts.Prompt, Execute: true}, nil
	}
	out, err := captureRun(t, "ask", "answerer", "--dir", t.TempDir(), "--agent", "codex", "--prompt", "investigate", "--timeout", "1ms", "--store-dir", storeDir, "--execute", "--json")
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("error = %v", err)
	}
	var report askReport
	if jsonErr := json.Unmarshal([]byte(out), &report); jsonErr != nil {
		t.Fatal(jsonErr)
	}
	if report.Status != dispatch.StatusFailed || report.QuestionID == "" {
		t.Fatalf("report = %#v", report)
	}
}

// seedThread creates a question that already carries a pane-delivered prompt
// into %7 and one ordinary answer, mirroring a completed first round.
func seedThread(t *testing.T, storeDir string) (*askreply.Store, askreply.Request) {
	t.Helper()
	store, err := askreply.New(storeDir)
	if err != nil {
		t.Fatal(err)
	}
	store.PollInterval = time.Millisecond
	request, err := store.Create("answerer", "/repo", "codex", "", askreply.DefaultTimeout)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Post(request.ID, askreply.Message{
		From: askreply.RoleAsker, Kind: askreply.KindPrompt, Delivery: askreply.DeliveryPane, Target: "%7",
	}, time.Time{}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Reply(request.ID, "first answer"); err != nil {
		t.Fatal(err)
	}
	return store, request
}

func waitForWaiter(t *testing.T, store *askreply.Store, id string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		has, err := store.HasWaiter(id)
		if err != nil {
			t.Fatal(err)
		}
		if has {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("answerer waiter never registered")
}

func TestAskThreadDeliversThroughMailboxWhenAnswererWaits(t *testing.T) {
	defer stubCLIHooks(t)()
	tmactSleep = func(time.Duration) {}
	storeDir := t.TempDir()
	store, request := seedThread(t, storeDir)
	dispatchRun = func(opts dispatch.Options) (dispatch.Report, error) {
		t.Fatalf("mailbox delivery must not dispatch into the pane: %#v", opts)
		return dispatch.Report{}, nil
	}

	answererErr := make(chan error, 1)
	go func() {
		followUp, err := store.AwaitPrompt(context.Background(), request.ID, 2)
		if err != nil {
			answererErr <- err
			return
		}
		if followUp.Text != "also check the tests" {
			answererErr <- errors.New("unexpected follow-up: " + followUp.Text)
			return
		}
		_, err = store.Post(request.ID, askreply.Message{
			From: askreply.RoleAnswerer, Kind: askreply.KindQuestion,
			Text: "which branch?", Delivery: askreply.DeliveryMailbox,
		}, time.Time{})
		answererErr <- err
	}()
	waitForWaiter(t, store, request.ID)

	out, err := captureRun(t, "ask", "--thread", request.ID, "--prompt", "also check the tests", "--store-dir", storeDir, "--timeout", "5s", "--execute", "--json")
	if err != nil {
		t.Fatal(err)
	}
	if err := <-answererErr; err != nil {
		t.Fatal(err)
	}
	var report askReport
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatal(err)
	}
	if report.Status != "answered" || report.Delivery != askreply.DeliveryMailbox || report.Dispatch != nil {
		t.Fatalf("report = %#v", report)
	}
	if report.Reply == nil || report.Reply.Text != "which branch?" || !report.AnswererWaiting || report.Closed {
		t.Fatalf("reply = %#v", report)
	}
	if report.Session != "answerer" || report.QuestionID != request.ID {
		t.Fatalf("thread routing = %#v", report)
	}
}

func TestAskThreadDispatchesIntoRecordedPaneWithoutClear(t *testing.T) {
	defer stubCLIHooks(t)()
	storeDir := t.TempDir()
	store, request := seedThread(t, storeDir)
	dispatched := false
	dispatchRun = func(opts dispatch.Options) (dispatch.Report, error) {
		dispatched = true
		if !opts.Execute || !opts.NoClear || opts.Target != "%7" || opts.Session != "answerer" || opts.Dir != "/repo" || opts.Agent != "codex" {
			t.Fatalf("dispatch opts = %#v", opts)
		}
		if !strings.Contains(opts.Prompt, "follow-up on question ID "+request.ID) || !strings.Contains(opts.Prompt, "tmact reply "+request.ID) {
			t.Fatalf("follow-up prompt = %s", opts.Prompt)
		}
		if _, err := store.Post(request.ID, askreply.Message{
			From: askreply.RoleAnswerer, Kind: askreply.KindFinal, Text: "all done", Delivery: askreply.DeliveryMailbox,
		}, time.Time{}); err != nil {
			t.Fatal(err)
		}
		if err := store.Close(request.ID, askreply.RoleAnswerer, "final reply"); err != nil {
			t.Fatal(err)
		}
		return dispatch.Report{
			Session: opts.Session, Target: "%7", Dir: opts.Dir, Agent: opts.Agent, Prompt: opts.Prompt, Execute: true,
			Steps: []dispatch.Step{{Name: "send-prompt", Status: dispatch.StatusOK}},
		}, nil
	}

	out, err := captureRun(t, "ask", "--thread", request.ID, "--prompt", "wrap up", "--store-dir", storeDir, "--timeout", "5s", "--execute", "--json")
	if err != nil {
		t.Fatal(err)
	}
	if !dispatched {
		t.Fatal("follow-up without a waiter must dispatch into the pane")
	}
	var report askReport
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatal(err)
	}
	if report.Status != "answered" || report.Delivery != askreply.DeliveryPane || report.Dispatch == nil {
		t.Fatalf("report = %#v", report)
	}
	if report.Reply == nil || report.Reply.Kind != askreply.KindFinal || !report.Closed || report.AnswererWaiting {
		t.Fatalf("reply = %#v", report)
	}

	if _, err := captureRun(t, "ask", "--thread", request.ID, "--prompt", "one more", "--store-dir", storeDir, "--execute"); err == nil || !strings.Contains(err.Error(), "is closed") {
		t.Fatalf("follow-up on closed question = %v", err)
	}
}

func TestAskThreadDryRunPlansPaneDeliveryWithoutMailboxWrites(t *testing.T) {
	defer stubCLIHooks(t)()
	storeDir := t.TempDir()
	store, request := seedThread(t, storeDir)
	dispatchRun = func(opts dispatch.Options) (dispatch.Report, error) {
		if opts.Execute || !opts.NoClear {
			t.Fatalf("dry-run opts = %#v", opts)
		}
		return dispatch.Report{
			Session: opts.Session, Dir: opts.Dir, Agent: opts.Agent, Prompt: opts.Prompt,
			Steps: []dispatch.Step{{Name: "send-prompt", Status: dispatch.StatusPlanned}},
		}, nil
	}
	out, err := captureRun(t, "ask", "--thread", request.ID, "--prompt", "more", "--store-dir", storeDir)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "dry-run: ask "+request.ID) || !strings.Contains(out, "delivery: pane") {
		t.Fatalf("output = %s", out)
	}
	thread, err := store.Load(request.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(thread.Messages) != 2 {
		t.Fatalf("dry-run wrote messages: %#v", thread.Messages)
	}
}

func TestAskThreadRejectsLaunchFlags(t *testing.T) {
	defer stubCLIHooks(t)()
	storeDir := t.TempDir()
	_, request := seedThread(t, storeDir)
	cases := [][]string{
		{"ask", "answerer", "--thread", request.ID, "--prompt", "x", "--store-dir", storeDir},
		{"ask", "--thread", request.ID, "--dir", "/repo", "--prompt", "x", "--store-dir", storeDir},
		{"ask", "--thread", request.ID, "--agent", "codex", "--prompt", "x", "--store-dir", storeDir},
		{"ask", "--thread", request.ID, "--trust-folder", "--prompt", "x", "--store-dir", storeDir},
		{"ask", "--thread", request.ID, "--store-dir", storeDir},
		{"ask", "--close", "--store-dir", storeDir},
	}
	for _, args := range cases {
		if _, err := captureRun(t, args...); err == nil {
			t.Fatalf("args %v unexpectedly succeeded", args)
		}
	}
}

func TestAskCloseEndsQuestionForAnswerer(t *testing.T) {
	defer stubCLIHooks(t)()
	storeDir := t.TempDir()
	store, request := seedThread(t, storeDir)
	out, err := captureRun(t, "ask", "--thread", request.ID, "--close", "--store-dir", storeDir, "--json")
	if err != nil {
		t.Fatal(err)
	}
	var report askReport
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatal(err)
	}
	if report.Status != askStatusClosed || !report.Closed || report.QuestionID != request.ID {
		t.Fatalf("report = %#v", report)
	}
	if _, err := store.Reply(request.ID, "late"); !errors.Is(err, askreply.ErrAlreadyFinalized) {
		t.Fatalf("reply after close = %v", err)
	}
}

func TestReplyCommandWritesAnswerAndKeepsQuestionOpen(t *testing.T) {
	defer stubCLIHooks(t)()
	storeDir := t.TempDir()
	store, err := askreply.New(storeDir)
	if err != nil {
		t.Fatal(err)
	}
	request, err := store.Create("answerer", "/repo", "codex", "", askreply.DefaultTimeout)
	if err != nil {
		t.Fatal(err)
	}

	out, err := captureRun(t, "reply", request.ID, "--text", "done", "--store-dir", storeDir, "--json")
	if err != nil {
		t.Fatal(err)
	}
	var report replyReport
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatal(err)
	}
	if report.Status != "replied" || report.Reply.QuestionID != request.ID || report.Reply.Text != "done" || report.Reply.Kind != askreply.KindAnswer || report.Closed {
		t.Fatalf("report = %#v", report)
	}
	out, err = captureRun(t, "reply", request.ID, "--text", "again", "--store-dir", storeDir)
	if err != nil {
		t.Fatalf("second reply on an open thread = %v", err)
	}
	if !strings.Contains(out, "(seq 2)") {
		t.Fatalf("output = %s", out)
	}
}

func TestReplyFinalClosesQuestion(t *testing.T) {
	defer stubCLIHooks(t)()
	storeDir := t.TempDir()
	store, request := seedThread(t, storeDir)
	out, err := captureRun(t, "reply", request.ID, "--text", "done", "--final", "--store-dir", storeDir)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "question closed") {
		t.Fatalf("output = %s", out)
	}
	if _, err := captureRun(t, "reply", request.ID, "--text", "again", "--store-dir", storeDir); !errors.Is(err, askreply.ErrAlreadyFinalized) {
		t.Fatalf("duplicate error = %v", err)
	}
	thread, err := store.Load(request.ID)
	if err != nil {
		t.Fatal(err)
	}
	if thread.Closed == nil || thread.Closed.By != askreply.RoleAnswerer {
		t.Fatalf("closed = %#v", thread.Closed)
	}
}

func TestReplyWaitReturnsAskerFollowUp(t *testing.T) {
	defer stubCLIHooks(t)()
	storeDir := t.TempDir()
	store, request := seedThread(t, storeDir)
	askerErr := make(chan error, 1)
	go func() {
		deadline := time.Now().Add(2 * time.Second)
		for time.Now().Before(deadline) {
			has, err := store.HasWaiter(request.ID)
			if err != nil {
				askerErr <- err
				return
			}
			if has {
				_, err := store.Post(request.ID, askreply.Message{
					From: askreply.RoleAsker, Kind: askreply.KindPrompt,
					Text: "main branch", Delivery: askreply.DeliveryMailbox,
				}, time.Now().Add(time.Hour))
				askerErr <- err
				return
			}
			time.Sleep(time.Millisecond)
		}
		askerErr <- errors.New("waiter never appeared")
	}()

	out, err := captureRun(t, "reply", request.ID, "--text", "which branch?", "--wait", "--timeout", "5s", "--store-dir", storeDir, "--json")
	if err != nil {
		t.Fatal(err)
	}
	if err := <-askerErr; err != nil {
		t.Fatal(err)
	}
	var report replyReport
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatal(err)
	}
	if report.Status != "answered" || report.Reply.Kind != askreply.KindQuestion || report.FollowUp == nil || report.FollowUp.Text != "main branch" {
		t.Fatalf("report = %#v", report)
	}
	if has, err := store.HasWaiter(request.ID); err != nil || has {
		t.Fatalf("waiter after return = %v, %v", has, err)
	}
}

func TestReplyWaitTimeoutLeavesQuestionOpen(t *testing.T) {
	defer stubCLIHooks(t)()
	storeDir := t.TempDir()
	store, request := seedThread(t, storeDir)
	out, err := captureRun(t, "reply", request.ID, "--text", "which branch?", "--wait", "--timeout", "20ms", "--store-dir", storeDir, "--json")
	if err == nil || !strings.Contains(err.Error(), "no follow-up within") {
		t.Fatalf("error = %v", err)
	}
	var report replyReport
	if jsonErr := json.Unmarshal([]byte(out), &report); jsonErr != nil {
		t.Fatal(jsonErr)
	}
	if report.Status != dispatch.StatusFailed || report.Reply.Seq != 3 {
		t.Fatalf("report = %#v", report)
	}
	thread, err := store.Load(request.ID)
	if err != nil {
		t.Fatal(err)
	}
	if thread.Closed != nil {
		t.Fatalf("answerer timeout closed the thread: %#v", thread.Closed)
	}
}

func TestReplyRejectsConflictingFlags(t *testing.T) {
	defer stubCLIHooks(t)()
	if _, err := captureRun(t, "reply", "q_abcdefghijklmnopqrstuvwxyz"); err == nil || !strings.Contains(err.Error(), "exactly one") {
		t.Fatalf("missing body error = %v", err)
	}
	if _, err := captureRun(t, "reply", "q_abcdefghijklmnopqrstuvwxyz", "--text", "x", "--file", "answer.txt"); err == nil || !strings.Contains(err.Error(), "exactly one") {
		t.Fatalf("two bodies error = %v", err)
	}
	if _, err := captureRun(t, "reply", "q_abcdefghijklmnopqrstuvwxyz", "--text", "x", "--wait", "--final"); err == nil || !strings.Contains(err.Error(), "mutually exclusive") {
		t.Fatalf("wait+final error = %v", err)
	}
}

func TestReplyCommandShellQuotesCustomStore(t *testing.T) {
	command := buildReplyCommand("q_abcdefghijklmnopqrstuvwxyz", "/tmp/asker's $store", true)
	if !strings.Contains(command, `--store-dir '/tmp/asker'\''s $store'`) {
		t.Fatalf("command = %s", command)
	}
	_, followUp := buildProtocolCommands("q_abcdefghijklmnopqrstuvwxyz", "/tmp/asker's $store", true)
	if !strings.Contains(followUp, "tmact ask --thread q_abcdefghijklmnopqrstuvwxyz") || !strings.Contains(followUp, `--store-dir '/tmp/asker'\''s $store'`) {
		t.Fatalf("follow-up command = %s", followUp)
	}
}
