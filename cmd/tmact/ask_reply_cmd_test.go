package main

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/leolin310148/tmact/internal/askreply"
	"github.com/leolin310148/tmact/internal/dispatch"
)

var questionIDInPrompt = regexp.MustCompile(`q_[a-z2-7]{26}`)

func TestAskDispatchesProtocolAndReturnsExplicitReply(t *testing.T) {
	defer stubCLIHooks(t)()
	dir := t.TempDir()
	storeDir := t.TempDir()

	dispatchRun = func(opts dispatch.Options) (dispatch.Report, error) {
		if !opts.Execute || opts.Wait {
			t.Fatalf("dispatch opts = %#v", opts)
		}
		if !strings.Contains(opts.Prompt, "tmact reply protocol (required)") ||
			!strings.Contains(opts.Prompt, "do not merely mention the command") {
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
			Session: opts.Session, Target: "answerer:0.0", Dir: opts.Dir,
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
	if report.Prompt != "investigate" {
		t.Fatalf("original prompt = %q", report.Prompt)
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

func TestReplyCommandWritesOneShotAnswer(t *testing.T) {
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
	var reply askreply.Reply
	if err := json.Unmarshal([]byte(out), &reply); err != nil {
		t.Fatal(err)
	}
	if reply.QuestionID != request.ID || reply.Text != "done" {
		t.Fatalf("reply = %#v", reply)
	}
	if _, err := captureRun(t, "reply", request.ID, "--text", "again", "--store-dir", storeDir); !errors.Is(err, askreply.ErrAlreadyFinalized) {
		t.Fatalf("duplicate error = %v", err)
	}
}

func TestReplyRequiresExactlyOneBodySource(t *testing.T) {
	defer stubCLIHooks(t)()
	if _, err := captureRun(t, "reply", "q_abcdefghijklmnopqrstuvwxyz"); err == nil || !strings.Contains(err.Error(), "exactly one") {
		t.Fatalf("missing body error = %v", err)
	}
	if _, err := captureRun(t, "reply", "q_abcdefghijklmnopqrstuvwxyz", "--text", "x", "--file", "answer.txt"); err == nil || !strings.Contains(err.Error(), "exactly one") {
		t.Fatalf("two bodies error = %v", err)
	}
}

func TestReplyCommandShellQuotesCustomStore(t *testing.T) {
	command := buildReplyCommand("q_abcdefghijklmnopqrstuvwxyz", "/tmp/asker's $store", true)
	if !strings.Contains(command, `--store-dir '/tmp/asker'\''s $store'`) {
		t.Fatalf("command = %s", command)
	}
}
