package main

import (
	"context"
	"testing"
	"time"

	"github.com/leolin310148/tmact/internal/askreply"
	"github.com/leolin310148/tmact/internal/mcp"
)

func TestPollAskAnswerKeepsQuestionOpen(t *testing.T) {
	store, err := askreply.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	req, err := store.Create("rev", t.TempDir(), "claude", "", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	st, err := pollAskAnswer(ctx, store, req.ID, 0)
	if err != nil || st.Status != mcp.AskPending {
		t.Fatalf("empty poll = %+v, %v", st, err)
	}
	thread, _ := store.Load(req.ID)
	if thread.Closed != nil {
		t.Fatalf("a pending poll closed the question: %+v", thread.Closed)
	}

	post := func(kind, text string) {
		t.Helper()
		if _, err := store.Post(req.ID, askreply.Message{From: askreply.RoleAnswerer, Kind: kind, Text: text, Delivery: askreply.DeliveryMailbox}, time.Time{}); err != nil {
			t.Fatal(err)
		}
	}
	post(askreply.KindAnswer, "interim")
	post(askreply.KindFinal, "done")

	// Each poll hands over the next unread reply exactly once, in order.
	for _, want := range []struct{ kind, text string }{{askreply.KindAnswer, "interim"}, {askreply.KindFinal, "done"}} {
		st, err := pollAskAnswer(ctx, store, req.ID, 0)
		if err != nil || st.Status != mcp.AskAnswered || st.ReplyKind != want.kind || st.ReplyText != want.text {
			t.Fatalf("poll = %+v, %v; want %s %q", st, err, want.kind, want.text)
		}
	}
	st, err = pollAskAnswer(ctx, store, req.ID, 0)
	if err != nil || st.Status != mcp.AskClosed {
		t.Fatalf("after final = %+v, %v", st, err)
	}
}
