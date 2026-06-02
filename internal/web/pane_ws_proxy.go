package web

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"sync/atomic"
	"time"

	"github.com/coder/websocket"
	"github.com/leolin310148/tmact/internal/statusd"
)

const (
	peerDialTimeout = 5 * time.Second

	// Keepalive + diagnostics for the peer bridge. Unlike the direct
	// handlePaneWS handler, a proxied pane has two network legs (browser↔hub
	// and hub↔peer) and the hub is just shuttling raw frames, so it has no
	// liveness check of its own. Without these pings a silently wedged leg
	// hangs both Reads forever (the "panel frozen, takes ages to respond"
	// symptom). proxyStatInterval controls a periodic throughput log line.
	//
	// The cadence is deliberately tighter than the direct handler's 25s/10s: a
	// long-lived hub↔peer TCP flow over Tailscale/WSL occasionally wedges for
	// ~20s (sends nothing, never pongs) while the peer's HTTP and a *fresh* WS
	// flow stay healthy — the classic z13 peer-flap. The only signal that tells
	// a wedged flow apart from a legitimately idle pane (which still pongs) is
	// the pong, so we ping often and time out fast: worst-case detection drops
	// from ~35s (25+10) to ~15s (10+5), after which the browser reconnects onto
	// a fresh, working flow. An idle pane is never torn down — it still pongs.
	proxyPingInterval = 10 * time.Second
	proxyPingTimeout  = 5 * time.Second
	proxyStatInterval = 15 * time.Second
	// proxySlowWrite logs any single downstream write that blocks longer than
	// this — i.e. the receiver (usually the browser) applying backpressure.
	proxySlowWrite = 500 * time.Millisecond
)

// dirStats accumulates per-direction frame counters for one leg of the bridge.
// Each field is written by exactly one copy goroutine and read by the keepalive
// goroutine, so atomics suffice (no read-modify-write contention per field).
type dirStats struct {
	frames        atomic.Int64
	bytes         atomic.Int64
	lastFrameNano atomic.Int64 // UnixNano of the most recent frame, 0 if none yet
	maxWriteNano  atomic.Int64 // longest single downstream write seen
}

// quietFor reports how long it has been since the last frame in this direction.
// Zero means no frame has flowed yet.
func (d *dirStats) quietFor() time.Duration {
	last := d.lastFrameNano.Load()
	if last == 0 {
		return 0
	}
	return time.Since(time.Unix(0, last))
}

func durMs(nanos int64) time.Duration {
	return (time.Duration(nanos) * time.Nanosecond).Round(time.Millisecond)
}

// copyResult reports which bridge leg ended and why, so the close summary can
// attribute the teardown to z13 or the browser.
type copyResult struct {
	dir string
	err error
}

// proxyPaneWS bridges a browser WebSocket connection to the matching /ws/pane
// endpoint on a remote statusd peer. Raw frames are forwarded in both
// directions until either side closes or the request context is cancelled.
//
// Errors during the upstream dial are reported as a JSON HTTP response before
// the upgrade happens; once both ends are upgraded the bridge shuttles frames,
// keeps both legs alive with pings, and logs throughput so a freeze can be
// attributed to the right leg.
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
	dialDur := time.Since(started)

	client, err := websocket.Accept(w, r, nil)
	if err != nil {
		return
	}
	defer client.CloseNow()
	client.SetReadLimit(wsReadLimit)
	s.logf("peer ws %s pane %s connected in %s (dial %s)", peer.Name, pane, time.Since(started).Round(time.Millisecond), dialDur.Round(time.Millisecond))

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	var down, up dirStats // down: peer→client (pane output); up: client→peer (input)

	// Keepalive both legs + periodic throughput log. See the const block above
	// for why the proxied path needs its own liveness check.
	go s.proxyKeepalive(ctx, cancel, peer.Name, pane, client, upstream, &down, &up)

	// Bidirectional raw-frame copy. Either side closing cancels the context,
	// which unblocks the other Read and ends the bridge. The result carries the
	// direction so the close summary names which leg ended the bridge first —
	// "peer→client" means z13 (or its tmux capture) closed; "client→peer" means
	// the browser closed (navigated away / reconnect-churn / tab hidden).
	done := make(chan copyResult, 2)
	go func() {
		err := s.copyFrames(ctx, upstream, client, fmt.Sprintf("%s pane %s peer→client", peer.Name, pane), proxySlowWrite, &down)
		done <- copyResult{dir: "peer→client (z13 closed)", err: err}
		cancel()
	}()
	go func() {
		err := s.copyFrames(ctx, client, upstream, fmt.Sprintf("%s pane %s client→peer", peer.Name, pane), 0, &up)
		done <- copyResult{dir: "client→peer (browser closed)", err: err}
		cancel()
	}()
	first := <-done
	// Drain the second goroutine so deferred CloseNow / cancel happen after
	// both copy loops have unwound.
	<-done

	s.logf("peer ws %s pane %s closed after %s: peer→client %d frames/%d bytes (max write %s, quiet %s); client→peer %d frames/%d bytes; first close: %s: %v",
		peer.Name, pane, time.Since(started).Round(time.Second),
		down.frames.Load(), down.bytes.Load(), durMs(down.maxWriteNano.Load()), down.quietFor().Round(time.Second),
		up.frames.Load(), up.bytes.Load(), first.dir, first.err)
}

// copyFrames shuttles frames from src to dst until either errors, recording
// throughput into st. dir is a human label used only for logging. When
// slowWrite > 0, any single downstream Write that blocks longer than it is
// logged as backpressure from the receiver.
func (s *Server) copyFrames(ctx context.Context, src, dst *websocket.Conn, dir string, slowWrite time.Duration, st *dirStats) error {
	for {
		typ, data, err := src.Read(ctx)
		if err != nil {
			return err
		}
		st.frames.Add(1)
		st.bytes.Add(int64(len(data)))
		st.lastFrameNano.Store(time.Now().UnixNano())

		writeStart := time.Now()
		if err := dst.Write(ctx, typ, data); err != nil {
			return err
		}
		wd := time.Since(writeStart)
		if wd.Nanoseconds() > st.maxWriteNano.Load() {
			st.maxWriteNano.Store(wd.Nanoseconds())
		}
		if slowWrite > 0 && wd > slowWrite {
			s.logf("peer ws %s slow write %s for %d-byte frame — downstream backpressure", dir, wd.Round(time.Millisecond), len(data))
		}
	}
}

// proxyKeepalive pings both bridge legs and emits a periodic throughput line.
// A failing ping identifies which leg died and tears the bridge down via
// cancel(), so a wedged connection surfaces within proxyPingInterval instead of
// blocking the copy goroutines indefinitely.
func (s *Server) proxyKeepalive(ctx context.Context, cancel context.CancelFunc, peerName, pane string, client, upstream *websocket.Conn, down, up *dirStats) {
	pingT := time.NewTicker(proxyPingInterval)
	defer pingT.Stop()
	statT := time.NewTicker(proxyStatInterval)
	defer statT.Stop()

	var lastDown, lastUp int64
	for {
		select {
		case <-ctx.Done():
			return
		case <-statT.C:
			df, uf := down.frames.Load(), up.frames.Load()
			s.logf("peer ws %s pane %s live: peer→client %d frames (+%d, %d bytes, quiet %s); client→peer %d frames (+%d)",
				peerName, pane, df, df-lastDown, down.bytes.Load(), down.quietFor().Round(time.Millisecond), uf, uf-lastUp)
			lastDown, lastUp = df, uf
		case <-pingT.C:
			// Browser leg first, then the peer leg, so the log says which died.
			if err := pingConn(ctx, client); err != nil {
				s.logf("peer ws %s pane %s client (browser) keepalive failed: %v", peerName, pane, err)
				cancel()
				return
			}
			if err := pingConn(ctx, upstream); err != nil {
				s.logf("peer ws %s pane %s peer (upstream) keepalive failed: %v", peerName, pane, err)
				cancel()
				return
			}
		}
	}
}

func pingConn(ctx context.Context, c *websocket.Conn) error {
	pingCtx, pingCancel := context.WithTimeout(ctx, proxyPingTimeout)
	defer pingCancel()
	return c.Ping(pingCtx)
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
