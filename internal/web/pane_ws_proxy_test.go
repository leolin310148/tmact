package web

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
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

func TestPaneWSProxySwapsUpstreamWithoutDroppingBrowser(t *testing.T) {
	// Upstream "peer" that closes the WS after greeting on the first
	// connection (simulating a wedged/closed hub↔peer flow) and stays up on the
	// second. The browser must receive both greetings on the SAME connection —
	// proving the bridge re-dialed the upstream instead of dropping the browser.
	var conns int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&conns, 1)
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			t.Errorf("upstream accept: %v", err)
			return
		}
		ctx := r.Context()
		if err := wsjson.Write(ctx, conn, outMsg{T: "patch", From: 0, Lines: []string{fmt.Sprintf("epoch %d", n)}}); err != nil {
			conn.CloseNow()
			return
		}
		if n == 1 {
			conn.Close(websocket.StatusNormalClosure, "rotating")
			return
		}
		defer conn.CloseNow()
		<-ctx.Done() // keep the second flow open
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

	var first, second outMsg
	if err := wsjson.Read(ctx, c, &first); err != nil {
		t.Fatalf("read first greeting: %v", err)
	}
	if strings.Join(first.Lines, "") != "epoch 1" {
		t.Fatalf("first greeting = %+v, want epoch 1", first)
	}
	// Same browser connection should now receive the second epoch's greeting
	// after the bridge transparently re-dials the upstream.
	if err := wsjson.Read(ctx, c, &second); err != nil {
		t.Fatalf("read second greeting (post-swap): %v", err)
	}
	if strings.Join(second.Lines, "") != "epoch 2" {
		t.Fatalf("second greeting = %+v, want epoch 2", second)
	}
	if got := atomic.LoadInt32(&conns); got < 2 {
		t.Fatalf("upstream saw %d connections, want >= 2 (a swap)", got)
	}
}

func TestPaneWSProxySwapsAfterUpstreamWedge(t *testing.T) {
	// The real z13 failure mode: the hub↔peer flow stalls — sends nothing and
	// never answers a ping — WITHOUT closing the socket. Here the first upstream
	// connection greets then stops reading (so coder/websocket never auto-pongs)
	// while holding the socket open; the second behaves normally. The browser
	// must ride across the wedge: receive epoch 1, then epoch 2 on the SAME
	// connection, driven by the keepalive pong timeout, not a clean close.
	var conns int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&conns, 1)
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			t.Errorf("upstream accept: %v", err)
			return
		}
		ctx := r.Context()
		if err := wsjson.Write(ctx, conn, outMsg{T: "patch", From: 0, Lines: []string{fmt.Sprintf("epoch %d", n)}}); err != nil {
			conn.CloseNow()
			return
		}
		if n == 1 {
			// Wedge: hold the socket open but never Read, so pings go unanswered.
			select {
			case <-ctx.Done():
			case <-time.After(5 * time.Second):
			}
			conn.CloseNow()
			return
		}
		// Healthy: keep reading so the library auto-responds to keepalive pings.
		defer conn.CloseNow()
		for {
			if _, _, err := conn.Read(ctx); err != nil {
				return
			}
		}
	}))
	defer upstream.Close()

	srv := &Server{
		Peers:                     []statusd.Peer{{Name: "remote", URL: upstream.URL}},
		proxyPingIntervalOverride: 150 * time.Millisecond,
		proxyPingTimeoutOverride:  150 * time.Millisecond,
	}
	local := httptest.NewServer(srv.Handler())
	defer local.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	c, _, err := websocket.Dial(ctx, wsURL(local.URL)+"/ws/pane?pane=remote@%250", nil)
	if err != nil {
		t.Fatalf("dial local: %v", err)
	}
	defer c.CloseNow()

	// Read continuously so the browser leg always auto-pongs (the same fast
	// keepalive runs on the browser side and must not false-time-out).
	msgs := make(chan string, 8)
	go func() {
		for {
			var m outMsg
			if err := wsjson.Read(ctx, c, &m); err != nil {
				return
			}
			msgs <- strings.Join(m.Lines, "")
		}
	}()
	want := func(exp string) {
		t.Helper()
		select {
		case got := <-msgs:
			if got != exp {
				t.Fatalf("got %q, want %q", got, exp)
			}
		case <-ctx.Done():
			t.Fatalf("timed out waiting for %q", exp)
		}
	}

	want("epoch 1")
	// After the pong timeout (~interval + timeout) the bridge re-dials upstream
	// and the SAME browser connection receives the next epoch — no reconnect.
	want("epoch 2")
	if got := atomic.LoadInt32(&conns); got < 2 {
		t.Fatalf("upstream saw %d connections, want >= 2 (a wedge-driven swap)", got)
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
