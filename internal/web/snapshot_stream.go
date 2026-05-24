package web

import (
	"fmt"
	"net/http"
	"os"
	"time"
)

const (
	snapshotPollInterval = 250 * time.Millisecond
	snapshotKeepalive    = 25 * time.Second
)

// handleSnapshotStream pushes the statusd snapshot file to the browser over
// Server-Sent Events. The handler stat()s the snapshot every snapshotPollInterval
// and only ships bytes when the mtime advances, so an idle daemon costs the
// client zero traffic. A periodic comment-line keepalive keeps intermediary
// proxies from closing the long-lived connection.
func (s *Server) handleSnapshotStream(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
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
	var lastMod time.Time

	sendSnapshot := func() bool {
		info, err := os.Stat(s.StatePath)
		if err != nil {
			return true // tolerate transient unavailability; keep streaming
		}
		mod := info.ModTime()
		if mod.Equal(lastMod) {
			return true
		}
		data, err := os.ReadFile(s.StatePath)
		if err != nil {
			return true
		}
		lastMod = mod
		if _, err := fmt.Fprintf(w, "event: snapshot\ndata: %s\n\n", data); err != nil {
			return false
		}
		flusher.Flush()
		return true
	}

	if !sendSnapshot() {
		return
	}

	poll := time.NewTicker(snapshotPollInterval)
	defer poll.Stop()
	keepalive := time.NewTicker(snapshotKeepalive)
	defer keepalive.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-poll.C:
			if !sendSnapshot() {
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
