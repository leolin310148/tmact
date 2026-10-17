package main

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/leolin310148/tmact/internal/askreply"
	"github.com/leolin310148/tmact/internal/dispatch"
	"github.com/leolin310148/tmact/internal/mcp"
)

// mcpAskTimeout is how long an MCP question stays answerable. ChatGPT polls
// with get_answer instead of blocking, so every poll also pushes it out.
const mcpAskTimeout = 2 * time.Hour

// mcpAskDeps adapts the ask/reply store for the MCP tools. It reuses the CLI's
// startAsk and deliverAskFollowUp so delivery rules (mailbox waiter vs pane)
// stay identical; only the blocking wait is replaced by a bounded poll that
// never closes the question.
func mcpAskDeps(readyTimeout time.Duration) (
	start func(context.Context, mcp.AskRequest) (mcp.AskTicket, error),
	followUp func(context.Context, string, string) (mcp.AskTicket, error),
	poll func(context.Context, string, time.Duration) (mcp.AskState, error),
) {
	flags := func() askFlags {
		return askFlags{
			readyTimeout: readyTimeout,
			readySettle:  dispatch.DefaultReadySettleDelay,
			timeout:      mcpAskTimeout,
			execute:      true,
		}
	}
	start = func(ctx context.Context, req mcp.AskRequest) (mcp.AskTicket, error) {
		store, err := askreply.New("")
		if err != nil {
			return mcp.AskTicket{}, err
		}
		f := flags()
		f.session, f.dir, f.agent, f.model, f.prompt, f.noClear = req.Session, req.Dir, req.Agent, req.Model, req.Prompt, req.ContinueConversation
		report, err := startAsk(ctx, store, f)
		if err != nil {
			return mcp.AskTicket{}, err
		}
		return askTicket(report), nil
	}
	followUp = func(ctx context.Context, id, prompt string) (mcp.AskTicket, error) {
		store, err := askreply.New("")
		if err != nil {
			return mcp.AskTicket{}, err
		}
		f := flags()
		f.thread, f.prompt = id, prompt
		report, _, err := deliverAskFollowUp(ctx, store, f)
		if err != nil {
			return mcp.AskTicket{}, err
		}
		return askTicket(report), nil
	}
	poll = func(ctx context.Context, id string, wait time.Duration) (mcp.AskState, error) {
		store, err := askreply.New("")
		if err != nil {
			return mcp.AskState{}, err
		}
		return pollAskAnswer(ctx, store, id, wait)
	}
	return start, followUp, poll
}

func askTicket(r askReport) mcp.AskTicket {
	t := mcp.AskTicket{QuestionID: r.QuestionID, Session: r.Session, Delivery: r.Delivery}
	if r.Dispatch != nil {
		t.Pane = r.Dispatch.Target
		if t.Session == "" {
			t.Session = r.Dispatch.Session
		}
	}
	return t
}

// pollAskAnswer returns the first answerer message the asker has not been
// handed yet, waiting up to wait for one. Unlike AwaitAnswer it leaves the
// question open when the wait elapses.
func pollAskAnswer(ctx context.Context, store *askreply.Store, id string, wait time.Duration) (mcp.AskState, error) {
	deadline := tmactNow().Add(wait)
	for {
		thread, err := store.Load(id)
		if err != nil {
			return mcp.AskState{}, err
		}
		st := mcp.AskState{QuestionID: id, Session: thread.Request.Session, Status: mcp.AskPending}
		afterSeq, _, err := store.AnswerReadSeq(id)
		if err != nil {
			return mcp.AskState{}, err
		}
		for _, m := range thread.Messages {
			if m.From != askreply.RoleAnswerer || m.Seq <= afterSeq {
				continue
			}
			if err := store.MarkAnswerRead(id, m.Seq); err != nil {
				return mcp.AskState{}, err
			}
			st.Status, st.ReplyKind, st.ReplyText, st.RepliedAt = mcp.AskAnswered, m.Kind, m.Text, m.CreatedAt
			return st, nil
		}
		if thread.Closed != nil {
			st.Status, st.ClosedReason = mcp.AskClosed, fmt.Sprintf("%s by %s", thread.Closed.Reason, thread.Closed.By)
			return st, nil
		}
		if lastAnswerKind(thread) == askreply.KindFinal {
			st.Status, st.ClosedReason = mcp.AskClosed, "final answer already delivered"
			return st, nil
		}
		if !tmactNow().Before(thread.ExpiresAt) {
			st.Status = mcp.AskExpired
			return st, nil
		}
		if !tmactNow().Before(deadline) {
			// Someone is still asking: keep the question answerable.
			if err := store.Extend(id, tmactNow().Add(mcpAskTimeout)); err != nil && !errors.Is(err, askreply.ErrAlreadyFinalized) {
				return mcp.AskState{}, err
			}
			return st, nil
		}
		select {
		case <-ctx.Done():
			return mcp.AskState{}, ctx.Err()
		case <-time.After(500 * time.Millisecond):
		}
	}
}

func lastAnswerKind(thread askreply.Thread) string {
	for i := len(thread.Messages) - 1; i >= 0; i-- {
		if thread.Messages[i].From == askreply.RoleAnswerer {
			return thread.Messages[i].Kind
		}
	}
	return ""
}
