package web

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/coder/websocket"
	"github.com/leolin310148/tmact/internal/statusd"
)

const peerDialTimeout = 5 * time.Second

// proxyPaneWS bridges a browser WebSocket connection to the matching /ws/pane
// endpoint on a remote statusd peer. Raw frames are forwarded in both
// directions until either side closes or the request context is cancelled.
//
// Errors during the upstream dial are reported as a JSON HTTP response before
// the upgrade happens; once both ends are upgraded the bridge just shuttles
// frames and exits on the first failure.
func (s *Server) proxyPaneWS(w http.ResponseWriter, r *http.Request, peer statusd.Peer, pane string) {
	started := time.Now()
	upstreamURL, err := peerWSURL(peer.URL, pane)
	if err != nil {
		writeJSONError(w, http.StatusBadGateway, fmt.Sprintf("invalid peer URL %q: %v", peer.URL, err))
		return
	}

	dialCtx, dialCancel := context.WithTimeout(r.Context(), peerDialTimeout)
	upstream, _, err := websocket.Dial(dialCtx, upstreamURL, nil)
	dialCancel()
	if err != nil {
		s.logf("peer ws %s pane %s dial failed after %s: %v", peer.Name, pane, time.Since(started).Round(time.Millisecond), err)
		writeJSONError(w, http.StatusBadGateway, fmt.Sprintf("peer %s dial failed: %v", peer.Name, err))
		return
	}
	defer upstream.CloseNow()
	upstream.SetReadLimit(wsReadLimit)

	client, err := websocket.Accept(w, r, nil)
	if err != nil {
		return
	}
	defer client.CloseNow()
	client.SetReadLimit(wsReadLimit)
	s.logf("peer ws %s pane %s connected in %s", peer.Name, pane, time.Since(started).Round(time.Millisecond))
	defer func() {
		s.logf("peer ws %s pane %s closed after %s", peer.Name, pane, time.Since(started).Round(time.Second))
	}()

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	// Bidirectional raw-frame copy. Either side closing cancels the context,
	// which unblocks the other Read and ends the bridge.
	done := make(chan struct{}, 2)
	go func() {
		copyFrames(ctx, upstream, client)
		cancel()
		done <- struct{}{}
	}()
	go func() {
		copyFrames(ctx, client, upstream)
		cancel()
		done <- struct{}{}
	}()
	<-done
	// Drain the second goroutine so deferred CloseNow / cancel happen after
	// both copy loops have unwound.
	<-done
}

func copyFrames(ctx context.Context, src, dst *websocket.Conn) {
	for {
		typ, data, err := src.Read(ctx)
		if err != nil {
			return
		}
		if err := dst.Write(ctx, typ, data); err != nil {
			return
		}
	}
}

// peerWSURL converts a peer base URL (e.g. "http://host:7890") plus a tmux
// pane id into the upstream WebSocket URL for /ws/pane.
func peerWSURL(base, pane string) (string, error) {
	u, err := url.Parse(base)
	if err != nil {
		return "", err
	}
	switch u.Scheme {
	case "http":
		u.Scheme = "ws"
	case "https":
		u.Scheme = "wss"
	case "ws", "wss":
		// already a ws URL — pass through
	case "":
		return "", fmt.Errorf("missing scheme in peer URL")
	default:
		return "", fmt.Errorf("unsupported peer scheme %q", u.Scheme)
	}
	u.Path = "/ws/pane"
	q := u.Query()
	q.Set("pane", pane)
	u.RawQuery = q.Encode()
	return u.String(), nil
}
