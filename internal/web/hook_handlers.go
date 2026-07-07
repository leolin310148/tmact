package web

import (
	"io"
	"net/http"
	"time"

	"github.com/leolin310148/tmact/internal/shellhook"
)

const hookEventMaxBodyBytes = 16 << 10

// tcpConnContextKey marks request contexts whose connection arrived over a
// TCP listener rather than the unix socket; set by Serve's ConnContext.
type tcpConnContextKey struct{}

// handleHookEvent ingests one shell preexec/precmd event (from
// `tmact hook emit` over the unix socket) into the daemon's hook store.
// It is local IPC only: hook events describe local panes, so TCP callers
// (the browser UI origin, peers) are rejected rather than allowed to spoof
// pane state.
func (s *Server) handleHookEvent(w http.ResponseWriter, r *http.Request) {
	if r.Context().Value(tcpConnContextKey{}) != nil {
		writeJSONError(w, http.StatusForbidden, "hook events are accepted only over the local IPC socket")
		return
	}
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if s.HookRecord == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "shell hook ingestion not configured")
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, hookEventMaxBodyBytes))
	if err != nil {
		writeJSONError(w, http.StatusRequestEntityTooLarge, "request body too large")
		return
	}
	event, err := shellhook.ParseEvent(body, time.Now())
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := s.HookRecord(event); err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
