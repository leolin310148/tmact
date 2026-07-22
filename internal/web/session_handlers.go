// Session lifecycle endpoints for the web UI's pane-switcher overflow menu:
// killing a session outright (the menu's exit button) and listing / reopening
// recently closed sessions. Reopening recreates the session at its old cwd
// with a plain shell — it deliberately does NOT relaunch any agent runtime.
package web

import (
	"context"
	"encoding/json"
	"io"
	"mime"
	"net/http"
	"net/url"
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

// localClosedSession resolves one exact local history entry. Closed-session
// logs are normally deduplicated, but requiring exactly one match keeps a
// corrupt or hand-edited history file from selecting an arbitrary cwd.
func (s *Server) localClosedSession(name string) (statusd.ClosedSession, bool) {
	if s.ClosedSessions == nil {
		return statusd.ClosedSession{}, false
	}
	var (
		match statusd.ClosedSession
		count int
	)
	for _, entry := range s.ClosedSessions.List() {
		if entry.Session == name && entry.Peer == "" {
			match = entry
			count++
		}
	}
	return match, count == 1
}

// requireSessionMutationRequest rejects browser cross-site requests and
// CORS-safelisted body types before a destructive session mutation can run.
// Requests without browser origin metadata remain supported for local API
// clients and configured server-to-server peer proxying.
func requireSessionMutationRequest(w http.ResponseWriter, r *http.Request) bool {
	switch site := strings.ToLower(strings.TrimSpace(r.Header.Get("Sec-Fetch-Site"))); site {
	case "", "none", "same-origin":
	default:
		writeJSONError(w, http.StatusForbidden, "cross-site session mutation rejected")
		return false
	}
	if origin := strings.TrimSpace(r.Header.Get("Origin")); origin != "" && !requestOriginMatches(origin, r) {
		writeJSONError(w, http.StatusForbidden, "cross-origin session mutation rejected")
		return false
	}
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || !strings.EqualFold(mediaType, "application/json") {
		writeJSONError(w, http.StatusUnsupportedMediaType, "Content-Type must be application/json")
		return false
	}
	return true
}

func requestOriginMatches(origin string, r *http.Request) bool {
	u, err := url.Parse(origin)
	if err != nil || u.Scheme == "" || u.Host == "" || u.User != nil || u.Path != "" || u.RawQuery != "" || u.Fragment != "" {
		return false
	}
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	return strings.EqualFold(u.Scheme, scheme) && strings.EqualFold(u.Host, r.Host)
}

func decodeSessionMutationJSON(w http.ResponseWriter, r *http.Request, dst any) bool {
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid JSON body: "+err.Error())
		return false
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		writeJSONError(w, http.StatusBadRequest, "invalid JSON body: trailing data")
		return false
	}
	return true
}

// maybeProxyPeerSessionMutation validates browser metadata at the local hub,
// then removes it from the configured server-to-server hop. Authorization and
// all other end-to-end headers remain available to an authenticated peer.
func (s *Server) maybeProxyPeerSessionMutation(w http.ResponseWriter, r *http.Request, path string) bool {
	if r.URL.Query().Get("peer") == "" {
		return false
	}
	proxied := r.Clone(r.Context())
	proxied.Header = r.Header.Clone()
	for _, name := range []string{"Origin", "Referer", "Sec-Fetch-Site", "Sec-Fetch-Mode", "Sec-Fetch-Dest", "Sec-Fetch-User"} {
		proxied.Header.Del(name)
	}
	return s.maybeProxyPeerHTTP(w, proxied, path)
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
	if !requireSessionMutationRequest(w, r) {
		return
	}
	if s.maybeProxyPeerSessionMutation(w, r, "/api/session/kill") {
		return
	}
	defer r.Body.Close()
	var req struct {
		Session string `json:"session"`
	}
	if !decodeSessionMutationJSON(w, r, &req) {
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
	if !requireSessionMutationRequest(w, r) {
		return
	}
	if s.maybeProxyPeerSessionMutation(w, r, "/api/session/reopen") {
		return
	}
	defer r.Body.Close()
	var req struct {
		Session string          `json:"session"`
		CWD     json.RawMessage `json:"cwd,omitempty"`
	}
	if !decodeSessionMutationJSON(w, r, &req) {
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
	entry, ok := s.localClosedSession(name)
	if !ok {
		writeJSONError(w, http.StatusNotFound, "unknown closed session "+name)
		return
	}
	if _, ok := s.localSession(name); ok {
		writeJSONError(w, http.StatusConflict, "session "+name+" already exists")
		return
	}
	cwd := entry.CWD
	if req.CWD != nil {
		var requestedCWD *string
		if err := json.Unmarshal(req.CWD, &requestedCWD); err != nil || requestedCWD == nil {
			writeJSONError(w, http.StatusBadRequest, "cwd must be a JSON string")
			return
		}
		if *requestedCWD != cwd {
			writeJSONError(w, http.StatusConflict, "cwd does not match closed-session history")
			return
		}
	}
	if cwd != "" && !filepath.IsAbs(cwd) {
		writeJSONError(w, http.StatusBadRequest, "recorded cwd must be absolute")
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
