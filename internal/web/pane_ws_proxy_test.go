package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	"github.com/leolin310148/tmact/internal/statusd"
)

func TestPeerWSURLConvertsScheme(t *testing.T) {
	cases := []struct {
		base, pane, want string
	}{
		{"http://host:7890", "%0", "ws://host:7890/ws/pane?pane=%250"},
		{"https://host", "%12", "wss://host/ws/pane?pane=%2512"},
		{"ws://host:1234/", "%3", "ws://host:1234/ws/pane?pane=%253"},
		{"wss://host", "%4", "wss://host/ws/pane?pane=%254"},
	}
	for _, c := range cases {
		got, err := peerWSURL(c.base, c.pane)
		if err != nil {
			t.Fatalf("peerWSURL(%q, %q) error: %v", c.base, c.pane, err)
		}
		if got != c.want {
			t.Fatalf("peerWSURL(%q, %q) = %q, want %q", c.base, c.pane, got, c.want)
		}
	}
}

func TestPeerWSURLRejectsBadScheme(t *testing.T) {
	for _, base := range []string{"file:///etc/passwd", "ftp://host", "host:7890"} {
		if _, err := peerWSURL(base, "%0"); err == nil {
			t.Fatalf("peerWSURL(%q) expected error", base)
		}
	}
}

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

func TestPaneWSProxyBridgesFrames(t *testing.T) {
	// Upstream "peer" that echoes any frame it receives back to the client.
	gotPane := make(chan string, 1)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPane <- r.URL.Query().Get("pane")
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			t.Errorf("upstream accept: %v", err)
			return
		}
		defer conn.CloseNow()
		ctx := r.Context()
		// Greet the client so the proxy has data flowing both ways.
		if err := wsjson.Write(ctx, conn, outMsg{T: "patch", From: 0, Lines: []string{"hello from peer"}}); err != nil {
			return
		}
		for {
			typ, data, err := conn.Read(ctx)
			if err != nil {
				return
			}
			if err := conn.Write(ctx, typ, data); err != nil {
				return
			}
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
		t.Fatal("upstream never got a connection")
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
	var echo inputMsg
	if err := wsjson.Read(ctx, c, &echo); err != nil {
		t.Fatalf("read echo: %v", err)
	}
	if echo.T != "text" || echo.S != "ping" {
		t.Fatalf("echo = %+v, want text/ping", echo)
	}
}

func TestPaneWSProxyUnreachablePeerReturnsBadGateway(t *testing.T) {
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
	_, resp, err := websocket.Dial(ctx, wsURL(local.URL)+"/ws/pane?pane=remote@%250", nil)
	if err == nil {
		t.Fatal("expected dial error")
	}
	if resp == nil || resp.StatusCode != http.StatusBadGateway {
		t.Fatalf("expected HTTP 502, got resp=%v", resp)
	}
}
