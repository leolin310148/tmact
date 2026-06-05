package web

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	"github.com/leolin310148/tmact/internal/statusd"
)

func TestPaneIDPatternAcceptsFederatedID(t *testing.T) {
	accepted := []string{"%0", "%2511", "peer-a@%0", "my-host.tail@%99", "z_w@%1"}
	rejected := []string{"", "%", "%a", "@%0", "peer-a@", "peer-a@@%0", "peer-a@n0t", "peer-a@%-1", "x@y@%0", "not-a-pane"}
	for _, p := range accepted {
		if !paneIDPattern.MatchString(p) {
			t.Errorf("paneIDPattern rejected %q (should accept)", p)
		}
	}
	for _, p := range rejected {
		if paneIDPattern.MatchString(p) {
			t.Errorf("paneIDPattern accepted %q (should reject)", p)
		}
	}
}

func TestPaneWSProxyUnknownPeer(t *testing.T) {
	srv := httptest.NewServer((&Server{}).Handler())
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	_, resp, err := websocket.Dial(ctx, wsURL(srv.URL)+"/ws/pane?pane=nope@%250", nil)
	if err == nil {
		t.Fatal("expected dial error for unknown peer")
	}
	if resp == nil || resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected HTTP 404, got resp=%v", resp)
	}
}

func TestRemotePaneWSPullsDiffAndPostsInput(t *testing.T) {
	gotPane := make(chan string, 1)
	gotInput := make(chan inputMsg, 1)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/pane/diff":
			gotPane <- r.URL.Query().Get("pane")
			writeJSON(w, http.StatusOK, paneDiffMsg{T: "patch", From: 0, Lines: []string{"hello from peer"}, Cursor: "c1"})
		case "/api/pane/input":
			defer r.Body.Close()
			var m inputMsg
			if err := json.NewDecoder(r.Body).Decode(&m); err != nil {
				t.Errorf("decode input: %v", err)
				writeJSONError(w, http.StatusBadRequest, "bad json")
				return
			}
			gotInput <- m
			writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
		default:
			http.NotFound(w, r)
		}
	}))
	defer upstream.Close()

	local := httptest.NewServer((&Server{
		Peers: []statusd.Peer{{Name: "remote", URL: upstream.URL}},
	}).Handler())
	defer local.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	c, _, err := websocket.Dial(ctx, wsURL(local.URL)+"/ws/pane?pane=remote@%257", nil)
	if err != nil {
		t.Fatalf("dial local: %v", err)
	}
	defer c.CloseNow()

	select {
	case p := <-gotPane:
		if p != "%7" {
			t.Fatalf("upstream received pane=%q, want %%7", p)
		}
	case <-ctx.Done():
		t.Fatal("upstream never got a diff request")
	}

	var greet outMsg
	if err := wsjson.Read(ctx, c, &greet); err != nil {
		t.Fatalf("read greeting: %v", err)
	}
	if greet.T != "patch" || strings.Join(greet.Lines, "") != "hello from peer" {
		t.Fatalf("greeting = %+v, want patch with peer hello", greet)
	}

	if err := wsjson.Write(ctx, c, inputMsg{T: "text", S: "ping"}); err != nil {
		t.Fatalf("client write: %v", err)
	}
	select {
	case got := <-gotInput:
		if got.T != "text" || got.S != "ping" {
			t.Fatalf("input = %+v, want text/ping", got)
		}
	case <-ctx.Done():
		t.Fatal("upstream never got input")
	}
}

func TestRemotePaneWSNoDuplicatePatchOnUnchanged(t *testing.T) {
	var calls int
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/pane/diff" {
			http.NotFound(w, r)
			return
		}
		calls++
		if calls == 1 {
			writeJSON(w, http.StatusOK, paneDiffMsg{T: "patch", From: 0, Lines: []string{"same"}, Cursor: "c1"})
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer upstream.Close()

	local := httptest.NewServer((&Server{
		Peers: []statusd.Peer{{Name: "remote", URL: upstream.URL}},
	}).Handler())
	defer local.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	c, _, err := websocket.Dial(ctx, wsURL(local.URL)+"/ws/pane?pane=remote@%257", nil)
	if err != nil {
		t.Fatalf("dial local: %v", err)
	}
	defer c.CloseNow()

	var first outMsg
	if err := wsjson.Read(ctx, c, &first); err != nil {
		t.Fatalf("read first: %v", err)
	}
	if strings.Join(first.Lines, "") != "same" {
		t.Fatalf("first = %+v, want same", first)
	}

	shortCtx, shortCancel := context.WithTimeout(context.Background(), 350*time.Millisecond)
	defer shortCancel()
	var dup outMsg
	if err := wsjson.Read(shortCtx, c, &dup); err == nil {
		t.Fatalf("read duplicate patch %+v, want no message", dup)
	}
	if calls < 2 {
		t.Fatalf("peer diff calls = %d, want at least 2", calls)
	}
}

func TestRemotePaneWSHTTPFailureEmitsErrorAndRetries(t *testing.T) {
	var calls int
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/pane/diff" {
			http.NotFound(w, r)
			return
		}
		calls++
		if calls == 1 {
			writeJSONError(w, http.StatusInternalServerError, "temporary")
			return
		}
		writeJSON(w, http.StatusOK, paneDiffMsg{T: "patch", From: 0, Lines: []string{"recovered"}, Cursor: "c1"})
	}))
	defer upstream.Close()

	local := httptest.NewServer((&Server{
		Peers: []statusd.Peer{{Name: "remote", URL: upstream.URL}},
	}).Handler())
	defer local.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	c, _, err := websocket.Dial(ctx, wsURL(local.URL)+"/ws/pane?pane=remote@%250", nil)
	if err != nil {
		t.Fatalf("dial local: %v", err)
	}
	defer c.CloseNow()

	var first outMsg
	if err := wsjson.Read(ctx, c, &first); err != nil {
		t.Fatalf("read first: %v", err)
	}
	if first.T != "error" || !strings.Contains(first.S, "HTTP 500") {
		t.Fatalf("first = %+v, want HTTP 500 error", first)
	}
	var second outMsg
	if err := wsjson.Read(ctx, c, &second); err != nil {
		t.Fatalf("read second: %v", err)
	}
	if second.T != "patch" || strings.Join(second.Lines, "") != "recovered" {
		t.Fatalf("second = %+v, want recovered patch", second)
	}
}

func TestRemotePaneWSOldPeerShowsUpdateError(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	defer upstream.Close()

	local := httptest.NewServer((&Server{
		Peers: []statusd.Peer{{Name: "remote", URL: upstream.URL}},
	}).Handler())
	defer local.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	c, _, err := websocket.Dial(ctx, wsURL(local.URL)+"/ws/pane?pane=remote@%250", nil)
	if err != nil {
		t.Fatalf("dial local: %v", err)
	}
	defer c.CloseNow()

	var m outMsg
	if err := wsjson.Read(ctx, c, &m); err != nil {
		t.Fatalf("read error: %v", err)
	}
	if m.T != "error" || !strings.Contains(m.S, "please update the peer") {
		t.Fatalf("message = %+v, want update-peer error", m)
	}
}

func TestRemotePaneWSUnreachablePeerKeepsBrowserOpenWithError(t *testing.T) {
	// Closed listener guarantees the dial fails fast.
	closedSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	closedURL := closedSrv.URL
	closedSrv.Close()

	local := httptest.NewServer((&Server{
		Peers: []statusd.Peer{{Name: "remote", URL: closedURL}},
	}).Handler())
	defer local.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	c, _, err := websocket.Dial(ctx, wsURL(local.URL)+"/ws/pane?pane=remote@%250", nil)
	if err != nil {
		t.Fatalf("dial local: %v", err)
	}
	defer c.CloseNow()
	var m outMsg
	if err := wsjson.Read(ctx, c, &m); err != nil {
		t.Fatalf("read error: %v", err)
	}
	if m.T != "error" || !strings.Contains(m.S, "diff request failed") {
		t.Fatalf("message = %+v, want diff request error", m)
	}
}
