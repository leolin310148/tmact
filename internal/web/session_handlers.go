// Session lifecycle endpoints for the web UI's pane-switcher overflow menu:
// killing a session outright (the menu's exit button) and listing / reopening
// recently closed sessions. Reopening recreates the session at its old cwd
// with a plain shell — it deliberately does NOT relaunch any agent runtime.
package web

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/leolin310148/tmact/internal/statusd"
	"github.com/leolin310148/tmact/internal/tmux"
)

// peerClosedSessionsTimeout bounds each peer's contribution to the merged
// closed-sessions list so one unreachable peer can't stall the menu.
const peerClosedSessionsTimeout = 3 * time.Second

func (s *Server) killSession() func(string) error {
	if s.KillSession != nil {
		return s.KillSession
	}
	return tmux.KillSession
}

func (s *Server) newSession() func(session, window, cwd string, command []string) error {
	if s.NewSession != nil {
		return s.NewSession
	}
	return tmux.NewSession
}

func (s *Server) dirExists() func(string) bool {
	if s.DirExists != nil {
		return s.DirExists
	}
	return func(path string) bool {
		info, err := os.Stat(path)
		return err == nil && info.IsDir()
	}
}

// localSession returns the local (peer-less) session entry for name from the
// latest snapshot, or false when it is absent or the snapshot is unavailable.
func (s *Server) localSession(name string) (statusd.SessionStatus, bool) {
	if s.Store == nil {
		return statusd.SessionStatus{}, false
	}
	snap, ok := s.Store.Latest()
	if !ok {
		return statusd.SessionStatus{}, false
	}
	sess, ok := snap.Sessions[name]
	if !ok || sess.Peer != "" {
		return statusd.SessionStatus{}, false
	}
	return sess, true
}

// handleSessionKill kills one exact local tmux session. The name must match a
// session in the current snapshot — the web surface only ever acts on sessions
// statusd knows about, never on arbitrary tmux targets.
func (s *Server) handleSessionKill(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if s.maybeProxyPeerHTTP(w, r, "/api/session/kill") {
		return
	}
	defer r.Body.Close()
	var req struct {
		Session string `json:"session"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid JSON body: "+err.Error())
		return
	}
	name := strings.TrimSpace(req.Session)
	if name == "" {
		writeJSONError(w, http.StatusBadRequest, "session is required")
		return
	}
	if _, ok := s.localSession(name); !ok {
		writeJSONError(w, http.StatusNotFound, "unknown session "+name)
		return
	}
	if err := s.killSession()(name); err != nil {
		writeJSONError(w, http.StatusInternalServerError, "kill session: "+err.Error())
		return
	}
	s.logf("killed session %q via web UI", name)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// handleSessionReopen recreates a recently closed session: same name, same
// cwd, plain shell.
func (s *Server) handleSessionReopen(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if s.maybeProxyPeerHTTP(w, r, "/api/session/reopen") {
		return
	}
	defer r.Body.Close()
	var req struct {
		Session string `json:"session"`
		CWD     string `json:"cwd"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid JSON body: "+err.Error())
		return
	}
	name := strings.TrimSpace(req.Session)
	if name == "" {
		writeJSONError(w, http.StatusBadRequest, "session is required")
		return
	}
	// tmux session names cannot contain ":" or "." (target syntax).
	if strings.ContainsAny(name, ":.") {
		writeJSONError(w, http.StatusBadRequest, "invalid session name "+name)
		return
	}
	if _, ok := s.localSession(name); ok {
		writeJSONError(w, http.StatusConflict, "session "+name+" already exists")
		return
	}
	cwd := strings.TrimSpace(req.CWD)
	if cwd != "" && !filepath.IsAbs(cwd) {
		writeJSONError(w, http.StatusBadRequest, "cwd must be absolute")
		return
	}
	if cwd != "" && !s.dirExists()(cwd) {
		writeJSONError(w, http.StatusBadRequest, "cwd no longer exists: "+cwd)
		return
	}
	if err := s.newSession()(name, "", cwd, nil); err != nil {
		writeJSONError(w, http.StatusInternalServerError, "reopen session: "+err.Error())
		return
	}
	if s.ClosedSessions != nil {
		s.ClosedSessions.Remove(name)
	}
	s.logf("reopened session %q at %q via web UI", name, cwd)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// handleSessionsClosed lists recently closed sessions, newest first. By
// default the local log is merged with every pane peer's (each entry tagged
// with its peer name); ?scope=local returns only the local log and is what
// peers request from each other so federation never recurses.
func (s *Server) handleSessionsClosed(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var entries []statusd.ClosedSession
	if s.ClosedSessions != nil {
		entries = s.ClosedSessions.List()
	}
	if r.URL.Query().Get("scope") != "local" && len(s.Peers) > 0 {
		entries = append(entries, s.fetchPeerClosedSessions(r.Context())...)
		sort.SliceStable(entries, func(i, j int) bool {
			return entries[i].ClosedAt.After(entries[j].ClosedAt)
		})
	}
	if entries == nil {
		entries = []statusd.ClosedSession{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"sessions": entries})
}

// fetchPeerClosedSessions queries every pane peer's local closed-session log
// concurrently. Unreachable peers are skipped (logged, not surfaced) — the
// menu should render with whatever is available.
func (s *Server) fetchPeerClosedSessions(ctx context.Context) []statusd.ClosedSession {
	ctx, cancel := context.WithTimeout(ctx, peerClosedSessionsTimeout)
	defer cancel()
	var (
		mu  sync.Mutex
		out []statusd.ClosedSession
		wg  sync.WaitGroup
	)
	for _, peer := range s.Peers {
		wg.Add(1)
		go func(peer statusd.Peer) {
			defer wg.Done()
			upstream, err := peerHTTPURL(peer.URL, "/api/sessions/closed", map[string][]string{"scope": {"local"}})
			if err != nil {
				s.logf("closed sessions: invalid peer URL %q: %v", peer.URL, err)
				return
			}
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, upstream, nil)
			if err != nil {
				return
			}
			resp, err := s.httpClient().Do(req)
			if err != nil {
				s.logf("closed sessions: peer %s unreachable: %v", peer.Name, err)
				return
			}
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				s.logf("closed sessions: peer %s returned %d", peer.Name, resp.StatusCode)
				return
			}
			var body struct {
				Sessions []statusd.ClosedSession `json:"sessions"`
			}
			if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
				s.logf("closed sessions: peer %s bad response: %v", peer.Name, err)
				return
			}
			mu.Lock()
			defer mu.Unlock()
			for _, entry := range body.Sessions {
				entry.Peer = peer.Name
				out = append(out, entry)
			}
		}(peer)
	}
	wg.Wait()
	return out
}
