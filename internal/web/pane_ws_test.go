package web

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
)

/* ---- input validation ---- */

func TestKeyAllowed(t *testing.T) {
	allowed := []string{"Enter", "BSpace", "BTab", "Escape", "Up", "PageDown", "C-c", "C-z", "Space"}
	denied := []string{"", "rm", "C-C", "C-1", "M-x", "Enter; rm", "-X", "ArrowUp"}
	for _, k := range allowed {
		if !keyAllowed(k) {
			t.Errorf("keyAllowed(%q) = false, want true", k)
		}
	}
	for _, k := range denied {
		if keyAllowed(k) {
			t.Errorf("keyAllowed(%q) = true, want false", k)
		}
	}
}

func TestApplyInputText(t *testing.T) {
	var gotTarget, gotText string
	var gotEnter bool
	s := &Server{SendText: func(target, text string, enter bool) error {
		gotTarget, gotText, gotEnter = target, text, enter
		return nil
	}}
	if err := s.applyInput("%7", inputMsg{T: "text", S: "hi"}); err != nil {
		t.Fatal(err)
	}
	if gotTarget != "%7" || gotText != "hi" || gotEnter {
		t.Fatalf("SendText got (%q, %q, %v)", gotTarget, gotText, gotEnter)
	}
}

func TestApplyInputSendUsesEnter(t *testing.T) {
	var gotEnter bool
	s := &Server{SendText: func(_, _ string, enter bool) error {
		gotEnter = enter
		return nil
	}}
	if err := s.applyInput("%7", inputMsg{T: "send", S: "a prompt"}); err != nil {
		t.Fatal(err)
	}
	if !gotEnter {
		t.Fatal(`"send" message must paste with Enter`)
	}
}

func TestApplyInputKeyAllowed(t *testing.T) {
	var gotKey string
	s := &Server{SendKey: func(_, key string) error {
		gotKey = key
		return nil
	}}
	if err := s.applyInput("%7", inputMsg{T: "key", K: "C-c"}); err != nil {
		t.Fatal(err)
	}
	if gotKey != "C-c" {
		t.Fatalf("SendKey got %q, want C-c", gotKey)
	}
}

func TestApplyInputKeyRejected(t *testing.T) {
	s := &Server{SendKey: func(_, _ string) error {
		t.Fatal("SendKey must not run for a disallowed key")
		return nil
	}}
	if err := s.applyInput("%7", inputMsg{T: "key", K: "rm -rf"}); err == nil {
		t.Fatal("expected error for disallowed key")
	}
}

func TestApplyInputClearPane(t *testing.T) {
	var gotTarget string
	s := &Server{ClearPane: func(target string) error {
		gotTarget = target
		return nil
	}}
	if err := s.applyInput("%7", inputMsg{T: "clear"}); err != nil {
		t.Fatal(err)
	}
	if gotTarget != "%7" {
		t.Fatalf("ClearPane got %q, want %%7", gotTarget)
	}
}

func TestApplyInputUnknownType(t *testing.T) {
	if err := (&Server{}).applyInput("%7", inputMsg{T: "bogus"}); err == nil {
		t.Fatal("expected error for unknown message type")
	}
}

/* ---- /ws/pane integration ---- */

func wsURL(httpURL string) string {
	return "ws" + strings.TrimPrefix(httpURL, "http")
}

func dialPane(t *testing.T, srv *httptest.Server, pane string) (*websocket.Conn, context.Context) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	t.Cleanup(cancel)
	c, _, err := websocket.Dial(ctx, wsURL(srv.URL)+"/ws/pane?pane="+pane, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(func() { c.CloseNow() })
	return c, ctx
}

func TestPaneWSStreamsContent(t *testing.T) {
	srv := httptest.NewServer((&Server{
		CapturePane: func(string, int) (string, error) { return "pane body", nil },
	}).Handler())
	defer srv.Close()

	c, ctx := dialPane(t, srv, "%2511")
	var m outMsg
	if err := wsjson.Read(ctx, c, &m); err != nil {
		t.Fatal(err)
	}
	if m.T != "patch" || m.From != 0 || strings.Join(m.Lines, "\n") != "pane body" {
		t.Fatalf("got %+v, want patch from=0 lines=[pane body]", m)
	}
}

func TestPaneWSContentCarriesDetectedQuestion(t *testing.T) {
	menu := "Which approach?\n❯ 1. Use a library\n  2. Hand-roll it\n"
	srv := httptest.NewServer((&Server{
		CapturePane: func(string, int) (string, error) { return menu, nil },
	}).Handler())
	defer srv.Close()

	c, ctx := dialPane(t, srv, "%2511")
	var m outMsg
	if err := wsjson.Read(ctx, c, &m); err != nil {
		t.Fatal(err)
	}
	if m.T != "patch" || m.Q == nil {
		t.Fatalf("got %+v, want a patch message with a question", m)
	}
	if len(m.Q.Choices) != 2 {
		t.Fatalf("question choices = %d, want 2", len(m.Q.Choices))
	}
}

func TestPaneWSContentOmitsQuestionWhenNone(t *testing.T) {
	srv := httptest.NewServer((&Server{
		CapturePane: func(string, int) (string, error) { return "plain output, no menu", nil },
	}).Handler())
	defer srv.Close()

	c, ctx := dialPane(t, srv, "%2511")
	var m outMsg
	if err := wsjson.Read(ctx, c, &m); err != nil {
		t.Fatal(err)
	}
	if m.T != "patch" || m.Q != nil {
		t.Fatalf("got %+v, want a patch message with no question", m)
	}
}

func TestPaneWSPatchOmitsCommonPrefix(t *testing.T) {
	captures := []string{
		"line 1\nline 2\nline 3",
		"line 1\nline 2\nline 4\nline 5",
	}
	idx := 0
	srv := httptest.NewServer((&Server{
		CapturePane: func(string, int) (string, error) {
			if idx >= len(captures) {
				return captures[len(captures)-1], nil
			}
			out := captures[idx]
			idx++
			return out, nil
		},
	}).Handler())
	defer srv.Close()

	c, ctx := dialPane(t, srv, "%2511")
	var first outMsg
	if err := wsjson.Read(ctx, c, &first); err != nil {
		t.Fatal(err)
	}
	if first.From != 0 || len(first.Lines) != 3 {
		t.Fatalf("first patch %+v, want from=0 with 3 lines", first)
	}
	var second outMsg
	if err := wsjson.Read(ctx, c, &second); err != nil {
		t.Fatal(err)
	}
	if second.From != 2 || strings.Join(second.Lines, "|") != "line 4|line 5" {
		t.Fatalf("second patch %+v, want from=2 lines=[line 4 line 5]", second)
	}
}

func TestPaneWSAppliesTextInput(t *testing.T) {
	type call struct {
		target, text string
		enter        bool
	}
	calls := make(chan call, 4)
	srv := httptest.NewServer((&Server{
		CapturePane: func(string, int) (string, error) { return "", nil },
		SendText: func(target, text string, enter bool) error {
			calls <- call{target, text, enter}
			return nil
		},
	}).Handler())
	defer srv.Close()

	c, ctx := dialPane(t, srv, "%2511")
	if err := wsjson.Write(ctx, c, inputMsg{T: "text", S: "hello"}); err != nil {
		t.Fatal(err)
	}
	select {
	case got := <-calls:
		if got.target != "%11" || got.text != "hello" || got.enter {
			t.Fatalf("SendText got %+v", got)
		}
	case <-ctx.Done():
		t.Fatal("SendText was not called")
	}
}

func TestPaneWSRejectsDisallowedKey(t *testing.T) {
	srv := httptest.NewServer((&Server{
		CapturePane: func(string, int) (string, error) { return "", nil },
		SendKey: func(string, string) error {
			t.Error("SendKey must not run for a disallowed key")
			return nil
		},
	}).Handler())
	defer srv.Close()

	c, ctx := dialPane(t, srv, "%2511")
	if err := wsjson.Write(ctx, c, inputMsg{T: "key", K: "Dangerous"}); err != nil {
		t.Fatal(err)
	}
	var m outMsg
	if err := wsjson.Read(ctx, c, &m); err != nil {
		t.Fatal(err)
	}
	if m.T != "error" {
		t.Fatalf("got %+v, want an error message", m)
	}
}

func TestPaneWSRejectsBadPaneID(t *testing.T) {
	srv := httptest.NewServer((&Server{}).Handler())
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	c, _, err := websocket.Dial(ctx, wsURL(srv.URL)+"/ws/pane?pane=not-a-pane", nil)
	if err == nil {
		c.CloseNow()
		t.Fatal("expected dial to fail for an invalid pane id")
	}
}
