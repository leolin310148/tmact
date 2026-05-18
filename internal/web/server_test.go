package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
)

/* ---- HTTP endpoints ---- */

func TestSnapshotServesFileVerbatim(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "status.json")
	body := `{"version":1,"summary":{"sessions":2}}`
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	handler := (&Server{StatePath: path}).Handler()
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/snapshot", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if got := rec.Body.String(); got != body {
		t.Fatalf("body = %q, want %q", got, body)
	}
}

func TestSnapshotMissingFileReturns503(t *testing.T) {
	handler := (&Server{StatePath: filepath.Join(t.TempDir(), "absent.json")}).Handler()
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/snapshot", nil))

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", rec.Code)
	}
}

func TestIndexPageServed(t *testing.T) {
	handler := (&Server{}).Handler()
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "<title>tmact") {
		t.Fatal("index page body missing expected title")
	}
}

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
	if m.T != "content" || m.S != "pane body" {
		t.Fatalf("got %+v, want content/pane body", m)
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
