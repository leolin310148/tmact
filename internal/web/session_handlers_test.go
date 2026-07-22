package web

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/leolin310148/tmact/internal/statusd"
)

func sessionTestStore(t *testing.T, sessions map[string]statusd.SessionStatus) *statusd.Store {
	t.Helper()
	store := statusd.NewStore()
	store.Publish(statusd.Snapshot{Sessions: sessions})
	return store
}

func sessionJSONRequest(method, target, body string) *http.Request {
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	req.Header.Set("Origin", "http://example.com")
	req.Header.Set("Sec-Fetch-Site", "same-origin")
	return req
}

func TestHandleSessionKill(t *testing.T) {
	var killed []string
	s := &Server{
		Store: sessionTestStore(t, map[string]statusd.SessionStatus{
			"work":       {Session: "work"},
			"z13@remote": {Session: "z13@remote", Peer: "z13"},
		}),
		KillSession: func(name string) error {
			killed = append(killed, name)
			return nil
		},
	}
	handler := s.Handler()

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, sessionJSONRequest(http.MethodPost, "/api/session/kill", `{"session":"work"}`))
	if rec.Code != http.StatusOK {
		t.Fatalf("kill status = %d body=%s", rec.Code, rec.Body.String())
	}
	if len(killed) != 1 || killed[0] != "work" {
		t.Fatalf("killed = %v", killed)
	}

	// Unknown sessions and merged peer sessions must be rejected — the web
	// surface only kills local sessions statusd knows about.
	for _, name := range []string{"ghost", "z13@remote"} {
		rec = httptest.NewRecorder()
		handler.ServeHTTP(rec, sessionJSONRequest(http.MethodPost, "/api/session/kill", `{"session":"`+name+`"}`))
		if rec.Code != http.StatusNotFound {
			t.Fatalf("kill %q status = %d", name, rec.Code)
		}
	}
	if len(killed) != 1 {
		t.Fatalf("killed = %v", killed)
	}

	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/session/kill", nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("GET status = %d", rec.Code)
	}
}

func TestHandleSessionKillPropagatesTmuxError(t *testing.T) {
	s := &Server{
		Store:       sessionTestStore(t, map[string]statusd.SessionStatus{"work": {Session: "work"}}),
		KillSession: func(string) error { return errors.New("tmux gone") },
		Logf:        func(string, ...any) {},
	}
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, sessionJSONRequest(http.MethodPost, "/api/session/kill", `{"session":"work"}`))
	if rec.Code != http.StatusInternalServerError || !strings.Contains(rec.Body.String(), "tmux gone") {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestHandleSessionReopen(t *testing.T) {
	log := statusd.NewClosedSessionLog(filepath.Join(t.TempDir(), "closed.json"), 10)
	log.Record(statusd.ClosedSession{Session: "old", CWD: "/repo", ClosedAt: time.Now()})

	type call struct{ session, window, cwd string }
	var created []call
	s := &Server{
		Store: sessionTestStore(t, map[string]statusd.SessionStatus{"work": {Session: "work"}}),
		NewSession: func(session, window, cwd string, command []string) error {
			created = append(created, call{session, window, cwd})
			return nil
		},
		DirExists:      func(path string) bool { return path == "/repo" },
		ClosedSessions: log,
		Logf:           func(string, ...any) {},
	}
	handler := s.Handler()

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, sessionJSONRequest(http.MethodPost, "/api/session/reopen", `{"session":"old","cwd":"/repo"}`))
	if rec.Code != http.StatusOK {
		t.Fatalf("reopen status = %d body=%s", rec.Code, rec.Body.String())
	}
	if len(created) != 1 || created[0].session != "old" || created[0].cwd != "/repo" {
		t.Fatalf("created = %#v", created)
	}
	if entries := log.List(); len(entries) != 0 {
		t.Fatalf("log after reopen = %#v", entries)
	}
}

func TestHandleSessionReopenRollsBackWhenHistoryRemovalFails(t *testing.T) {
	path := filepath.Join(t.TempDir(), "closed.json")
	log := statusd.NewClosedSessionLog(path, 10)
	if err := log.Record(statusd.ClosedSession{Session: "old", CWD: "/repo", ClosedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	killed := 0
	s := &Server{
		ClosedSessions: log,
		DirExists:      func(string) bool { return true },
		NewSession: func(string, string, string, []string) error {
			if err := os.Remove(path); err != nil {
				t.Fatal(err)
			}
			if err := os.Mkdir(path, 0o700); err != nil {
				t.Fatal(err)
			}
			return nil
		},
		KillSession: func(string) error { killed++; return nil },
		Logf:        func(string, ...any) {},
	}
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, sessionJSONRequest(http.MethodPost, "/api/session/reopen", `{"session":"old"}`))
	if rec.Code != http.StatusInternalServerError || !strings.Contains(rec.Body.String(), "remove reopened session from history") {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if killed != 1 {
		t.Fatalf("cleanup KillSession calls = %d, want 1", killed)
	}
}

func TestHandleSessionReopenValidatesHistory(t *testing.T) {
	log := statusd.NewClosedSessionLog("", 10)
	log.Record(
		statusd.ClosedSession{Session: "old", CWD: "/repo", ClosedAt: time.Now()},
		statusd.ClosedSession{Session: "gone", CWD: "/gone", ClosedAt: time.Now()},
		statusd.ClosedSession{Session: "work", CWD: "/repo", ClosedAt: time.Now()},
	)
	var created []string
	s := &Server{
		Store:          sessionTestStore(t, map[string]statusd.SessionStatus{"work": {Session: "work"}}),
		ClosedSessions: log,
		DirExists:      func(path string) bool { return path == "/repo" },
		NewSession: func(_ string, _ string, cwd string, _ []string) error {
			created = append(created, cwd)
			return nil
		},
	}
	handler := s.Handler()
	tests := []struct {
		name string
		body string
		code int
	}{
		{"unknown history", `{"session":"unknown","cwd":"/repo"}`, http.StatusNotFound},
		{"cwd tampering", `{"session":"old","cwd":"/other"}`, http.StatusConflict},
		{"missing recorded cwd", `{"session":"gone","cwd":"/gone"}`, http.StatusBadRequest},
		{"existing session", `{"session":"work","cwd":"/repo"}`, http.StatusConflict},
		{"empty session", `{"session":" "}`, http.StatusBadRequest},
		{"invalid name", `{"session":"a:b","cwd":"/repo"}`, http.StatusBadRequest},
		{"null cwd", `{"session":"old","cwd":null}`, http.StatusBadRequest},
		{"unknown field", `{"session":"old","cwd":"/repo","execute":true}`, http.StatusBadRequest},
		{"trailing JSON", `{"session":"old","cwd":"/repo"}{}`, http.StatusBadRequest},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, sessionJSONRequest(http.MethodPost, "/api/session/reopen", tc.body))
			if rec.Code != tc.code {
				t.Fatalf("status = %d, want %d, body=%s", rec.Code, tc.code, rec.Body.String())
			}
		})
	}
	if len(created) != 0 {
		t.Fatalf("created after rejected requests = %#v", created)
	}

	// Omitting cwd is supported, but the new session still uses the history's
	// authoritative recorded cwd.
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, sessionJSONRequest(http.MethodPost, "/api/session/reopen", `{"session":"old"}`))
	if rec.Code != http.StatusOK {
		t.Fatalf("reopen without cwd status = %d body=%s", rec.Code, rec.Body.String())
	}
	if len(created) != 1 || created[0] != "/repo" {
		t.Fatalf("created = %#v", created)
	}
}

func TestSessionMutationsRejectCrossSiteAndSafelistedBodies(t *testing.T) {
	log := statusd.NewClosedSessionLog("", 10)
	log.Record(statusd.ClosedSession{Session: "old", CWD: "/repo", ClosedAt: time.Now()})
	var killed, created int
	s := &Server{
		Store:          sessionTestStore(t, map[string]statusd.SessionStatus{"work": {Session: "work"}}),
		ClosedSessions: log,
		DirExists:      func(string) bool { return true },
		KillSession:    func(string) error { killed++; return nil },
		NewSession: func(string, string, string, []string) error {
			created++
			return nil
		},
	}
	handler := s.Handler()
	tests := []struct {
		name        string
		path        string
		body        string
		contentType string
		origin      string
		fetchSite   string
		code        int
	}{
		{"kill cross origin", "/api/session/kill", `{"session":"work"}`, "application/json", "https://attacker.example", "", http.StatusForbidden},
		{"reopen cross site metadata", "/api/session/reopen", `{"session":"old","cwd":"/repo"}`, "application/json", "", "cross-site", http.StatusForbidden},
		{"kill text plain", "/api/session/kill", `{"session":"work"}`, "text/plain", "http://example.com", "same-origin", http.StatusUnsupportedMediaType},
		{"reopen text plain", "/api/session/reopen", `{"session":"old","cwd":"/repo"}`, "text/plain", "http://example.com", "same-origin", http.StatusUnsupportedMediaType},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, tc.path, strings.NewReader(tc.body))
			req.Header.Set("Content-Type", tc.contentType)
			req.Header.Set("Origin", tc.origin)
			req.Header.Set("Sec-Fetch-Site", tc.fetchSite)
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
			if rec.Code != tc.code {
				t.Fatalf("status = %d, want %d, body=%s", rec.Code, tc.code, rec.Body.String())
			}
		})
	}
	if killed != 0 || created != 0 {
		t.Fatalf("rejected request mutated sessions: killed=%d created=%d", killed, created)
	}
}

func TestSessionMutationsProxyConfiguredPeer(t *testing.T) {
	remoteLog := statusd.NewClosedSessionLog("", 10)
	remoteLog.Record(statusd.ClosedSession{Session: "old", CWD: "/remote", ClosedAt: time.Now()})
	var killed, created string
	remote := &Server{
		Store:          sessionTestStore(t, map[string]statusd.SessionStatus{"work": {Session: "work"}}),
		ClosedSessions: remoteLog,
		DirExists:      func(path string) bool { return path == "/remote" },
		KillSession:    func(name string) error { killed = name; return nil },
		NewSession: func(name, _ string, cwd string, _ []string) error {
			created = name + "@" + cwd
			return nil
		},
	}
	peer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Origin"); got != "" {
			t.Errorf("peer Origin = %q, want server-to-server request", got)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer peer-token" {
			t.Errorf("peer Authorization = %q", got)
		}
		remote.Handler().ServeHTTP(w, r)
	}))
	defer peer.Close()

	handler := (&Server{Peers: []statusd.Peer{{Name: "remote", URL: peer.URL}}}).Handler()
	for _, tc := range []struct {
		path string
		body string
	}{
		{"/api/session/kill?peer=remote", `{"session":"work"}`},
		{"/api/session/reopen?peer=remote", `{"session":"old","cwd":"/remote"}`},
	} {
		req := sessionJSONRequest(http.MethodPost, tc.path, tc.body)
		req.Header.Set("Authorization", "Bearer peer-token")
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s status = %d body=%s", tc.path, rec.Code, rec.Body.String())
		}
	}
	if killed != "work" || created != "old@/remote" {
		t.Fatalf("peer mutations: killed=%q created=%q", killed, created)
	}
}

func TestSessionMutationsRejectCrossSiteBeforePeerProxy(t *testing.T) {
	var requests int
	peer := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		requests++
	}))
	defer peer.Close()

	s := &Server{Peers: []statusd.Peer{{Name: "remote", URL: peer.URL}}}
	for _, path := range []string{
		"/api/session/kill?peer=remote",
		"/api/session/reopen?peer=remote",
	} {
		req := sessionJSONRequest(http.MethodPost, path, `{"session":"work"}`)
		req.Header.Set("Origin", "https://attacker.example")
		req.Header.Set("Sec-Fetch-Site", "cross-site")
		rec := httptest.NewRecorder()
		s.Handler().ServeHTTP(rec, req)
		if rec.Code != http.StatusForbidden {
			t.Fatalf("%s status = %d body=%s", path, rec.Code, rec.Body.String())
		}
	}
	if requests != 0 {
		t.Fatalf("rejected cross-site requests proxied = %d", requests)
	}
}

func TestHandleSessionsClosedMergesPeers(t *testing.T) {
	peerLog := statusd.NewClosedSessionLog("", 10)
	peerLog.Record(statusd.ClosedSession{Session: "remote-work", CWD: "/peer", ClosedAt: time.Date(2026, 7, 21, 12, 0, 2, 0, time.UTC)})
	peer := httptest.NewServer((&Server{ClosedSessions: peerLog}).Handler())
	defer peer.Close()

	localLog := statusd.NewClosedSessionLog("", 10)
	localLog.Record(statusd.ClosedSession{Session: "local-work", CWD: "/local", ClosedAt: time.Date(2026, 7, 21, 12, 0, 1, 0, time.UTC)})

	s := &Server{
		ClosedSessions: localLog,
		Peers:          []statusd.Peer{{Name: "mini", URL: peer.URL}},
	}
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/sessions/closed", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	var body struct {
		Sessions []statusd.ClosedSession `json:"sessions"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Sessions) != 2 {
		t.Fatalf("sessions = %#v", body.Sessions)
	}
	// Newest first, and the peer entry is tagged with the peer name.
	if body.Sessions[0].Session != "remote-work" || body.Sessions[0].Peer != "mini" {
		t.Fatalf("first = %#v", body.Sessions[0])
	}
	if body.Sessions[1].Session != "local-work" || body.Sessions[1].Peer != "" {
		t.Fatalf("second = %#v", body.Sessions[1])
	}

	// scope=local must not fan out to peers (what peers ask each other).
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/sessions/closed?scope=local", nil))
	body.Sessions = nil
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Sessions) != 1 || body.Sessions[0].Session != "local-work" {
		t.Fatalf("local scope = %#v", body.Sessions)
	}
}

func TestHandleSessionsClosedToleratesDownPeer(t *testing.T) {
	localLog := statusd.NewClosedSessionLog("", 10)
	localLog.Record(statusd.ClosedSession{Session: "local-work", ClosedAt: time.Now()})
	s := &Server{
		ClosedSessions: localLog,
		Peers:          []statusd.Peer{{Name: "down", URL: "http://127.0.0.1:1"}},
		Logf:           func(string, ...any) {},
	}
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/sessions/closed", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	var body struct {
		Sessions []statusd.ClosedSession `json:"sessions"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Sessions) != 1 || body.Sessions[0].Session != "local-work" {
		t.Fatalf("sessions = %#v", body.Sessions)
	}
}
