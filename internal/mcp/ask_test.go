package mcp

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

type askRec struct {
	started   []AskRequest
	followUps []string
	polls     []time.Duration
}

func askDeps(t *testing.T, home string, rec *askRec, state AskState) Deps {
	deps := writeDeps(t, home, &writeRec{})
	deps.AskStart = func(_ context.Context, req AskRequest) (AskTicket, error) {
		rec.started = append(rec.started, req)
		return AskTicket{QuestionID: "q1", Session: req.Session, Pane: req.Session + ":0.0", Delivery: "pane"}, nil
	}
	deps.AskFollowUp = func(_ context.Context, id, prompt string) (AskTicket, error) {
		if id != "q1" {
			return AskTicket{}, errors.New("unknown question")
		}
		rec.followUps = append(rec.followUps, prompt)
		return AskTicket{QuestionID: id, Session: "s", Delivery: "mailbox"}, nil
	}
	deps.AskPoll = func(_ context.Context, id string, wait time.Duration) (AskState, error) {
		rec.polls = append(rec.polls, wait)
		state.QuestionID = id
		return state, nil
	}
	return deps
}

func TestAskToolsListed(t *testing.T) {
	cs := connectRoots(t, askDeps(t, t.TempDir(), &askRec{}, AskState{}), testRoots)
	res, err := cs.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	ann := map[string]*sdk.ToolAnnotations{}
	for _, tool := range res.Tools {
		ann[tool.Name] = tool.Annotations
	}
	for _, name := range []string{"ask_agent", "ask_followup"} {
		if a := ann[name]; a == nil || a.ReadOnlyHint || a.DestructiveHint == nil || !*a.DestructiveHint {
			t.Fatalf("%s annotations = %+v", name, a)
		}
	}
	if a := ann["get_answer"]; a == nil || !a.ReadOnlyHint {
		t.Fatalf("get_answer annotations = %+v", a)
	}
	// Without roots the server stays read-only, ask tools included.
	cs = connectRoots(t, askDeps(t, t.TempDir(), &askRec{}, AskState{}), nil)
	res, _ = cs.ListTools(context.Background(), nil)
	for _, tool := range res.Tools {
		if strings.HasPrefix(tool.Name, "ask_") {
			t.Fatalf("read-only server lists %s", tool.Name)
		}
	}
}

func TestAskAgentResolvesDir(t *testing.T) {
	home := tempHome(t)
	rec := &askRec{}
	cs := connectRoots(t, askDeps(t, home, rec, AskState{}), testRoots)
	var out askOut
	res := call(t, cs, "ask_agent", map[string]any{"session": "rev", "dir": "~/w/repo", "agent": "claude", "prompt": "review"}, &out)
	if res.IsError {
		t.Fatalf("error: %v", res.Content[0].(*sdk.TextContent).Text)
	}
	want, _ := filepath.EvalSymlinks(filepath.Join(home, "w", "repo"))
	if got := rec.started[0]; got.Dir != want || got.Agent != "claude" || got.Prompt != "review" {
		t.Fatalf("started = %+v", got)
	}
	if out.QuestionID != "q1" || out.Pane != "rev:0.0" {
		t.Fatalf("out = %+v", out)
	}
	for name, args := range map[string]map[string]any{
		"escape":  {"dir": "~/w/escape"},
		"outside": {"dir": "~/etc"},
		"session": {"session": "a.b"},
		"peer":    {"peer": "mini"}, // ask_agent has no peer field: schema rejects it
	} {
		in := map[string]any{"session": "rev", "dir": "~/w/repo", "agent": "claude", "prompt": "p"}
		for k, v := range args {
			in[k] = v
		}
		if res := call(t, cs, "ask_agent", in, nil); !res.IsError {
			t.Fatalf("%s: accepted", name)
		}
	}
	if len(rec.started) != 1 {
		t.Fatalf("rejected asks reached the store: %d", len(rec.started))
	}
}

func TestGetAnswerWrapsReplyAndCapsWait(t *testing.T) {
	rec := &askRec{}
	cs := connectRoots(t, askDeps(t, t.TempDir(), rec, AskState{Session: "rev", Status: AskAnswered, ReplyKind: "question", ReplyText: "which branch?"}), testRoots)
	var out answerOut
	call(t, cs, "get_answer", map[string]any{"question_id": "q1", "wait_seconds": 999}, &out)
	if rec.polls[0] != maxAnswerWait {
		t.Fatalf("wait = %v", rec.polls[0])
	}
	if !strings.Contains(out.ObservedReply, captureOpen) || !strings.Contains(out.ObservedReply, "which branch?") {
		t.Fatalf("reply not wrapped: %q", out.ObservedReply)
	}
	if !strings.Contains(out.Next, "ask_followup") {
		t.Fatalf("next = %q", out.Next)
	}
}

func TestAskFollowup(t *testing.T) {
	rec := &askRec{}
	cs := connectRoots(t, askDeps(t, t.TempDir(), rec, AskState{}), testRoots)
	var out askOut
	call(t, cs, "ask_followup", map[string]any{"question_id": "q1", "prompt": "main"}, &out)
	if len(rec.followUps) != 1 || out.Delivery != "mailbox" {
		t.Fatalf("rec = %+v out = %+v", rec, out)
	}
	if res := call(t, cs, "ask_followup", map[string]any{"question_id": "nope", "prompt": "x"}, nil); !res.IsError {
		t.Fatal("unknown question accepted")
	}
}
