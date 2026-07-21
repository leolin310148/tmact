package web

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
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
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/session/kill", strings.NewReader(`{"session":"work"}`)))
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
		handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/session/kill", strings.NewReader(`{"session":"`+name+`"}`)))
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
	s.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/session/kill", strings.NewReader(`{"session":"work"}`)))
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
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/session/reopen", strings.NewReader(`{"session":"old","cwd":"/repo"}`)))
	if rec.Code != http.StatusOK {
		t.Fatalf("reopen status = %d body=%s", rec.Code, rec.Body.String())
	}
	if len(created) != 1 || created[0].session != "old" || created[0].cwd != "/repo" {
		t.Fatalf("created = %#v", created)
	}
	if entries := log.List(); len(entries) != 0 {
		t.Fatalf("log after reopen = %#v", entries)
	}

	cases := []struct {
		name string
		body string
		code int
	}{
		{"existing session", `{"session":"work","cwd":"/repo"}`, http.StatusConflict},
		{"empty session", `{"session":" "}`, http.StatusBadRequest},
		{"invalid name", `{"session":"a:b","cwd":"/repo"}`, http.StatusBadRequest},
		{"relative cwd", `{"session":"old","cwd":"repo"}`, http.StatusBadRequest},
		{"missing cwd dir", `{"session":"old","cwd":"/gone"}`, http.StatusBadRequest},
	}
	for _, tc := range cases {
		rec = httptest.NewRecorder()
		handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/session/reopen", strings.NewReader(tc.body)))
		if rec.Code != tc.code {
			t.Fatalf("%s: status = %d body=%s", tc.name, rec.Code, rec.Body.String())
		}
	}
	if len(created) != 1 {
		t.Fatalf("created after invalid requests = %#v", created)
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
