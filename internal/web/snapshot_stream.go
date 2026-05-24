package web

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

const snapshotKeepalive = 25 * time.Second

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

	send := func(snap any) bool {
		data, err := json.Marshal(snap)
		if err != nil {
			return false
		}
		if _, err := fmt.Fprintf(w, "event: snapshot\ndata: %s\n\n", data); err != nil {
			return false
		}
		flusher.Flush()
		return true
	}

	// Prime the connection with the current snapshot so clients render
	// immediately instead of waiting for the next daemon tick.
	if snap, ok := s.Store.Latest(); ok {
		if !send(snap) {
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
			if !send(snap) {
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
