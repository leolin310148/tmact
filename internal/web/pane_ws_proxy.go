package web

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"sync"
	"sync/atomic"
	"time"

	"github.com/coder/websocket"
	gws "github.com/gorilla/websocket"
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
	// the pong, so we ping often and time out fast (worst-case ~15s). An idle
	// pane is never torn down — it still pongs.
	proxyPingInterval = 10 * time.Second
	proxyPingTimeout  = 5 * time.Second
	proxyStatInterval = 15 * time.Second
	// A healthy /ws/pane sends a full initial patch immediately after the
	// handshake. If a peer accepts and pongs but never sends that first frame,
	// the remote handler is usually stuck in its initial capture. Swap the
	// upstream instead of leaving the browser on stale content forever.
	proxyFirstFrameTimeout = proxyPingInterval + proxyPingTimeout
	// proxySlowWrite logs any single downstream write that blocks longer than
	// this — i.e. the receiver (usually the browser) applying backpressure.
	proxySlowWrite = 500 * time.Millisecond

	// When the upstream (hub↔peer) flow ends — a wedge caught by the keepalive
	// above, or the peer closing the WS — the bridge re-dials a fresh upstream
	// and keeps shuttling to the *same* browser connection instead of tearing
	// the browser leg down. A transient Tailscale/WSL wedge then costs one
	// ~12ms re-dial plus a single full-page reseed, invisible to the browser,
	// instead of a visible disconnect + reconnect-churn + repeated reseed.
	//
	// A genuinely dead pane (peer gone, pane closed) would otherwise re-dial
	// forever, so a circuit breaker gives up after maxConsecutiveSwaps re-dials
	// that each lasted under minHealthyEpoch. A real wedge resets the counter
	// because the flow was healthy for minutes first; rapid back-to-back short
	// epochs mean the peer is down, so we fall back to closing the browser leg
	// (the old behaviour — the browser then reconnects on its own).
	minHealthyEpoch     = 2 * time.Second
	maxConsecutiveSwaps = 4
	swapBackoffStep     = 200 * time.Millisecond
	swapBackoffMax      = 2 * time.Second
	inputWriteTimeout   = 5 * time.Second
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

type upstreamConn interface {
	Read(context.Context) (websocket.MessageType, []byte, error)
	Write(context.Context, websocket.MessageType, []byte) error
	Ping(context.Context) error
	CloseNow()
	SetReadLimit(int64)
}

type gorillaUpstream struct {
	conn *gws.Conn
}

func (g *gorillaUpstream) Read(ctx context.Context) (websocket.MessageType, []byte, error) {
	if deadline, ok := ctx.Deadline(); ok {
		_ = g.conn.SetReadDeadline(deadline)
	} else {
		_ = g.conn.SetReadDeadline(time.Time{})
	}
	typ, data, err := g.conn.ReadMessage()
	return websocket.MessageType(typ), data, err
}

func (g *gorillaUpstream) Write(ctx context.Context, typ websocket.MessageType, data []byte) error {
	if deadline, ok := ctx.Deadline(); ok {
		_ = g.conn.SetWriteDeadline(deadline)
	} else {
		_ = g.conn.SetWriteDeadline(time.Time{})
	}
	return g.conn.WriteMessage(int(typ), data)
}

func (g *gorillaUpstream) Ping(ctx context.Context) error {
	deadline := time.Now().Add(proxyPingTimeout)
	if d, ok := ctx.Deadline(); ok {
		deadline = d
	}
	return g.conn.WriteControl(gws.PingMessage, nil, deadline)
}

func (g *gorillaUpstream) CloseNow() {
	_ = g.conn.Close()
}

func (g *gorillaUpstream) SetReadLimit(limit int64) {
	g.conn.SetReadLimit(limit)
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

// proxyPaneWS bridges a browser WebSocket connection to the matching /ws/pane
// endpoint on a remote statusd peer.
//
// The browser leg is the anchor: it is accepted once and kept up for the whole
// session. The hub↔peer (upstream) leg is *swappable* — when it wedges or
// closes, the bridge re-dials a fresh upstream and keeps shuttling to the same
// browser connection (see runSwappableBridge). That turns a transient z13
// peer-flap from a visible disconnect into an invisible re-dial.
//
// The first dial happens before the browser upgrade so an unreachable peer is
// reported as a clean HTTP 502 instead of an accepted-then-dropped WebSocket.
func (s *Server) proxyPaneWS(w http.ResponseWriter, r *http.Request, peer statusd.Peer, pane string) {
	started := time.Now()
	upstreamURL, err := peerWSURL(peer.URL, pane)
	if err != nil {
		writeJSONError(w, http.StatusBadGateway, fmt.Sprintf("invalid peer URL %q: %v", peer.URL, err))
		return
	}

	upstream, err := s.dialUpstream(r.Context(), upstreamURL)
	if err != nil {
		s.logf("peer ws %s pane %s dial failed after %s: %v", peer.Name, pane, time.Since(started).Round(time.Millisecond), err)
		writeJSONError(w, http.StatusBadGateway, fmt.Sprintf("peer %s dial failed: %v", peer.Name, err))
		return
	}
	dialDur := time.Since(started)

	client, err := websocket.Accept(w, r, nil)
	if err != nil {
		upstream.CloseNow()
		return
	}
	client.SetReadLimit(wsReadLimit)
	s.logf("peer ws %s pane %s connected in %s (dial %s)", peer.Name, pane, time.Since(started).Round(time.Millisecond), dialDur.Round(time.Millisecond))

	s.runSwappableBridge(r.Context(), peer.Name, pane, upstreamURL, client, upstream, started)
}

// dialUpstream opens a /ws/pane connection to a peer with the standard dial
// timeout and read limit applied.
func (s *Server) dialUpstream(ctx context.Context, upstreamURL string) (upstreamConn, error) {
	dialCtx, cancel := context.WithTimeout(ctx, peerDialTimeout)
	defer cancel()
	d := gws.Dialer{HandshakeTimeout: peerDialTimeout}
	conn, _, err := d.DialContext(dialCtx, upstreamURL, nil)
	if err != nil {
		return nil, err
	}
	upstream := &gorillaUpstream{conn: conn}
	upstream.SetReadLimit(wsReadLimit)
	return upstream, nil
}

// runSwappableBridge shuttles frames between a persistent browser connection
// and a series of upstream (hub↔peer) connections. The input pump and browser
// keepalive run for the whole session; the upstream leg is re-dialed in place
// whenever it wedges or closes, so a transient peer-flap never reaches the
// browser. It only gives up — closing the browser leg so it reconnects on its
// own — when the browser itself dies or the peer fails to stay up (circuit
// breaker), preserving the old worst-case behaviour.
func (s *Server) runSwappableBridge(parent context.Context, peerName, pane, upstreamURL string, client *websocket.Conn, first upstreamConn, started time.Time) {
	defer client.CloseNow()

	ctx, cancel := context.WithCancel(parent)
	defer cancel()

	var curMu sync.RWMutex
	var cur upstreamConn  // current upstream; nil while re-dialing
	var down, up dirStats // down: peer→client; up: client→peer
	var swaps atomic.Int64

	// Input pump (client→peer): persists across upstream swaps. A client Read
	// error means the browser is gone, which ends the whole bridge.
	go func() {
		for {
			typ, data, err := client.Read(ctx)
			if err != nil {
				cancel()
				return
			}
			curMu.RLock()
			u := cur
			curMu.RUnlock()
			if u == nil {
				continue // input typed during a re-dial gap is dropped
			}
			up.frames.Add(1)
			up.bytes.Add(int64(len(data)))
			up.lastFrameNano.Store(time.Now().UnixNano())
			wctx, wcancel := context.WithTimeout(ctx, inputWriteTimeout)
			if err := u.Write(wctx, typ, data); err != nil {
				s.logf("peer ws %s pane %s client→peer write failed: %v — forcing upstream swap", peerName, pane, err)
				u.CloseNow()
			}
			wcancel()
		}
	}()

	// Browser keepalive: a dead browser leg cannot be repaired by swapping the
	// upstream, so a failed ping ends the whole bridge.
	go func() {
		t := time.NewTicker(s.proxyPingEvery())
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				if err := pingConn(ctx, client, s.proxyPingDeadline()); err != nil {
					s.logf("peer ws %s pane %s client (browser) keepalive failed: %v", peerName, pane, err)
					cancel()
					return
				}
			}
		}
	}()

	go s.proxyStatLogger(ctx, peerName, pane, &down, &up, &swaps)

	upstream := first
	consecutiveShort := 0
	firstClose := "browser closed"
	for {
		curMu.Lock()
		cur = upstream
		curMu.Unlock()
		epochStart := time.Now()
		epochCtx, epochCancel := context.WithCancel(ctx)
		go s.upstreamKeepalive(epochCtx, epochCancel, peerName, pane, upstream)

		clientGone, perr := s.pumpDownstream(epochCtx, upstream, client, peerName, pane, &down)
		curMu.Lock()
		cur = nil
		curMu.Unlock()
		upstream.CloseNow()
		epochCancel()

		if ctx.Err() != nil {
			break // browser closed / browser keepalive failed
		}
		if clientGone {
			firstClose = fmt.Sprintf("browser write failed: %v", perr)
			cancel()
			break
		}

		// Upstream leg ended while the browser is still here — re-dial and keep
		// going. A flow that stayed healthy a while (>= minHealthyEpoch) resets
		// the breaker; back-to-back short epochs mean the peer is down.
		if time.Since(epochStart) < minHealthyEpoch {
			consecutiveShort++
		} else {
			consecutiveShort = 0
		}
		if consecutiveShort > maxConsecutiveSwaps {
			firstClose = fmt.Sprintf("peer kept failing fast (%d short epochs); giving up: %v", consecutiveShort, perr)
			s.logf("peer ws %s pane %s %s — closing browser leg so it reconnects", peerName, pane, firstClose)
			cancel()
			break
		}
		if d := swapBackoff(consecutiveShort); d > 0 {
			select {
			case <-ctx.Done():
			case <-time.After(d):
			}
			if ctx.Err() != nil {
				break
			}
		}
		swaps.Add(1)
		s.logf("peer ws %s pane %s upstream flow ended (%v) — re-dialing to keep browser connected (swap #%d)", peerName, pane, perr, swaps.Load())
		newUp, derr := s.dialUpstream(ctx, upstreamURL)
		if derr != nil {
			firstClose = fmt.Sprintf("re-dial failed: %v", derr)
			s.logf("peer ws %s pane %s %s — closing browser leg", peerName, pane, firstClose)
			cancel()
			break
		}
		upstream = newUp
	}

	s.logf("peer ws %s pane %s closed after %s: peer→client %d frames/%d bytes (max write %s, quiet %s); client→peer %d frames/%d bytes; swaps %d; first close: %s",
		peerName, pane, time.Since(started).Round(time.Second),
		down.frames.Load(), down.bytes.Load(), durMs(down.maxWriteNano.Load()), down.quietFor().Round(time.Second),
		up.frames.Load(), up.bytes.Load(), swaps.Load(), firstClose)
}

// pumpDownstream forwards peer→client frames for one upstream epoch. It returns
// when the upstream Read fails (wedge caught by upstreamKeepalive, or the peer
// closing) or a client Write fails. clientGone reports the latter: a broken
// browser leg can't be repaired by swapping the upstream, so the caller ends
// the whole bridge instead of re-dialing.
func (s *Server) pumpDownstream(ctx context.Context, upstream upstreamConn, client *websocket.Conn, peerName, pane string, st *dirStats) (clientGone bool, err error) {
	seenFrame := false
	for {
		readCtx := ctx
		var readCancel context.CancelFunc
		if !seenFrame {
			readCtx, readCancel = context.WithTimeout(ctx, proxyFirstFrameTimeout)
		}
		typ, data, rerr := upstream.Read(readCtx)
		if readCancel != nil {
			readCancel()
		}
		if rerr != nil {
			return false, rerr
		}
		seenFrame = true
		st.frames.Add(1)
		st.bytes.Add(int64(len(data)))
		st.lastFrameNano.Store(time.Now().UnixNano())

		writeStart := time.Now()
		writeCtx, writeCancel := context.WithTimeout(ctx, wsWriteTimeout)
		werr := client.Write(writeCtx, typ, data)
		writeCancel()
		if werr != nil {
			return true, werr
		}
		wd := time.Since(writeStart)
		if wd.Nanoseconds() > st.maxWriteNano.Load() {
			st.maxWriteNano.Store(wd.Nanoseconds())
		}
		if wd > proxySlowWrite {
			s.logf("peer ws %s pane %s slow write %s for %d-byte frame — browser backpressure", peerName, pane, wd.Round(time.Millisecond), len(data))
		}
	}
}

// upstreamKeepalive pings one upstream epoch and cancels it on a failed ping,
// which surfaces a wedged hub↔peer flow within proxyPingTimeout so the bridge
// can swap onto a fresh flow. An idle pane still pongs and is never torn down.
func (s *Server) upstreamKeepalive(ctx context.Context, cancel context.CancelFunc, peerName, pane string, upstream upstreamConn) {
	t := time.NewTicker(s.proxyPingEvery())
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			pingCtx, pingCancel := context.WithTimeout(ctx, s.proxyPingDeadline())
			err := upstream.Ping(pingCtx)
			pingCancel()
			if err != nil {
				if ctx.Err() == nil {
					s.logf("peer ws %s pane %s peer (upstream) keepalive failed: %v — swapping flow", peerName, pane, err)
				}
				upstream.CloseNow()
				cancel()
				return
			}
		}
	}
}

// proxyStatLogger emits a periodic throughput line covering all upstream epochs
// of a bridge, so a freeze can be attributed to the right leg.
func (s *Server) proxyStatLogger(ctx context.Context, peerName, pane string, down, up *dirStats, swaps *atomic.Int64) {
	t := time.NewTicker(proxyStatInterval)
	defer t.Stop()
	var lastDown, lastUp int64
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			df, uf := down.frames.Load(), up.frames.Load()
			s.logf("peer ws %s pane %s live: peer→client %d frames (+%d, %d bytes, quiet %s); client→peer %d frames (+%d); swaps %d",
				peerName, pane, df, df-lastDown, down.bytes.Load(), down.quietFor().Round(time.Millisecond), uf, uf-lastUp, swaps.Load())
			lastDown, lastUp = df, uf
		}
	}
}

// swapBackoff staggers re-dials after consecutive short-lived upstream epochs so
// a down peer is not hammered, while a single transient wedge re-dials at once.
func swapBackoff(consecutiveShort int) time.Duration {
	if consecutiveShort <= 0 {
		return 0
	}
	d := time.Duration(consecutiveShort) * swapBackoffStep
	if d > swapBackoffMax {
		d = swapBackoffMax
	}
	return d
}

func pingConn(ctx context.Context, c *websocket.Conn, timeout time.Duration) error {
	pingCtx, pingCancel := context.WithTimeout(ctx, timeout)
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
