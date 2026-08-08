package web

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/leolin310148/tmact/internal/prompt"
	"github.com/leolin310148/tmact/internal/statusd"
)

const snapshotKeepalive = 25 * time.Second
const webSnapshotMaxHeartbeatInterval = 5 * time.Second

// webSnapshot is the browser-facing projection of statusd.Snapshot. The full
// snapshot remains available from /api/snapshot and the default SSE stream for
// CLI and peer consumers; the React UI only needs pane-switcher fields.
type webSnapshot struct {
	Version      int                      `json:"version"`
	Timestamp    time.Time                `json:"ts"`
	IntervalMS   int64                    `json:"interval_ms"`
	StaleAfterMS int64                    `json:"stale_after_ms"`
	Panes        map[string]webPaneStatus `json:"panes"`
}

type webPaneStatus struct {
	Target      string         `json:"target"`
	PaneID      string         `json:"pane_id,omitempty"`
	Session     string         `json:"session"`
	WindowIndex int            `json:"window_index"`
	PaneIndex   int            `json:"pane_index"`
	CWD         string         `json:"cwd,omitempty"`
	Runtime     string         `json:"runtime"`
	State       string         `json:"state"`
	Idle        bool           `json:"idle"`
	Running     bool           `json:"running"`
	Asking      bool           `json:"asking"`
	Stale       bool           `json:"stale,omitempty"`
	Prompt      *prompt.Prompt `json:"prompt,omitempty"`
	Peer        string         `json:"peer,omitempty"`
}

type webSnapshotHeartbeat struct {
	Timestamp    time.Time `json:"ts"`
	IntervalMS   int64     `json:"interval_ms"`
	StaleAfterMS int64     `json:"stale_after_ms"`
}

func projectWebSnapshot(snap statusd.Snapshot) webSnapshot {
	panes := make(map[string]webPaneStatus, len(snap.Panes))
	for target, pane := range snap.Panes {
		panes[target] = webPaneStatus{
			Target:      pane.Target,
			PaneID:      pane.PaneID,
			Session:     pane.Session,
			WindowIndex: pane.WindowIndex,
			PaneIndex:   pane.PaneIndex,
			CWD:         pane.CWD,
			Runtime:     pane.Runtime,
			State:       pane.State,
			Idle:        pane.Idle,
			Running:     pane.Running,
			Asking:      pane.Asking,
			Stale:       pane.Stale,
			Prompt:      pane.Prompt,
			Peer:        pane.Peer,
		}
	}
	return webSnapshot{
		Version:      snap.Version,
		Timestamp:    snap.Timestamp,
		IntervalMS:   snap.IntervalMS,
		StaleAfterMS: snap.StaleAfterMS,
		Panes:        panes,
	}
}

func webSnapshotHeartbeatInterval(staleAfterMS int64) time.Duration {
	interval := time.Duration(staleAfterMS) * time.Millisecond / 2
	if interval <= 0 || interval > webSnapshotMaxHeartbeatInterval {
		return webSnapshotMaxHeartbeatInterval
	}
	return interval
}

// webSnapshotStreamState suppresses full browser snapshots until a field used
// by the UI changes. Timestamp/cadence-only updates become small heartbeats.
type webSnapshotStreamState struct {
	semantic     []byte
	lastEmission time.Time
}

func (s *webSnapshotStreamState) next(snap statusd.Snapshot) (string, any, bool, error) {
	projected := projectWebSnapshot(snap)
	semantic, err := json.Marshal(struct {
		Version int                      `json:"version"`
		Panes   map[string]webPaneStatus `json:"panes"`
	}{Version: projected.Version, Panes: projected.Panes})
	if err != nil {
		return "", nil, false, err
	}

	if !bytes.Equal(s.semantic, semantic) {
		s.semantic = semantic
		s.lastEmission = snap.Timestamp
		return "snapshot", projected, true, nil
	}
	if snap.Timestamp.Sub(s.lastEmission) < webSnapshotHeartbeatInterval(snap.StaleAfterMS) {
		return "", nil, false, nil
	}

	s.lastEmission = snap.Timestamp
	return "heartbeat", webSnapshotHeartbeat{
		Timestamp:    snap.Timestamp,
		IntervalMS:   snap.IntervalMS,
		StaleAfterMS: snap.StaleAfterMS,
	}, true, nil
}

// handleSnapshotStream pushes the statusd snapshot to the browser over
// Server-Sent Events. The handler subscribes to the in-memory store and
// fires the moment the daemon publishes a new snapshot — there is no polling,
// and an idle daemon costs the client zero traffic. A periodic comment-line
// keepalive keeps intermediary proxies from closing the long-lived connection.
func (s *Server) handleSnapshotStream(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if s.Store == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "snapshot store not configured")
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		writeJSONError(w, http.StatusInternalServerError, "streaming not supported")
		return
	}

	h := w.Header()
	h.Set("Content-Type", "text/event-stream")
	h.Set("Cache-Control", "no-store")
	h.Set("Connection", "keep-alive")
	h.Set("X-Accel-Buffering", "no") // disable proxy buffering (nginx)
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	ctx := r.Context()
	updates, cancel := s.Store.Subscribe()
	defer cancel()

	send := func(event string, payload any) bool {
		data, err := json.Marshal(payload)
		if err != nil {
			return false
		}
		if _, err := fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, data); err != nil {
			return false
		}
		flusher.Flush()
		return true
	}

	webView := r.URL.Query().Get("view") == "web"
	webState := webSnapshotStreamState{}
	sendSnapshot := func(snap statusd.Snapshot) bool {
		if !webView {
			return send("snapshot", snap)
		}
		event, payload, emit, err := webState.next(snap)
		if err != nil {
			return false
		}
		return !emit || send(event, payload)
	}

	// Prime the connection with the current snapshot so clients render
	// immediately instead of waiting for the next daemon tick.
	if snap, ok := s.Store.Latest(); ok {
		if !sendSnapshot(snap) {
			return
		}
	}

	keepalive := time.NewTicker(snapshotKeepalive)
	defer keepalive.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case snap, ok := <-updates:
			if !ok {
				return
			}
			if !sendSnapshot(snap) {
				return
			}
		case <-keepalive.C:
			if _, err := w.Write([]byte(": ping\n\n")); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}
