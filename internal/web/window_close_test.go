package web

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/leolin310148/tmact/internal/statusd"
)

func TestWindowCloseTargetsOnlyRequestedWindow(t *testing.T) {
	store := statusd.NewStore()
	store.Publish(statusd.Snapshot{Panes: map[string]statusd.PaneStatus{
		"ops:0.0":      {Session: "ops", WindowIndex: 0, PaneID: "%10"},
		"ops:1.0":      {Session: "ops", WindowIndex: 1, PaneID: "%11"},
		"peer@ops:0.0": {Session: "peer@ops", Peer: "peer", PaneID: "%12"},
	}})
	var targets []string
	s := &Server{Store: store, CloseWindow: func(target string) error { targets = append(targets, target); return nil }, KillSession: func(string) error { t.Fatal("must not kill the session"); return nil }}
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, sessionJSONRequest("POST", "/api/window/close", `{"pane":"%11"}`))
	if rec.Code != 200 || len(targets) != 1 || targets[0] != "%11" {
		t.Fatalf("status=%d targets=%v", rec.Code, targets)
	}
	for _, tc := range []struct {
		body   string
		status int
	}{
		{`{"pane":"ops:1"}`, 400}, {`{"pane":"%999"}`, 404}, {`{"pane":"%12"}`, 404}, {`{"pane":"peer@%11"}`, 400}, {`{"session":"ops"}`, 400}, {`{"pane":"%11"} {}`, 400},
	} {
		rec = httptest.NewRecorder()
		s.Handler().ServeHTTP(rec, sessionJSONRequest("POST", "/api/window/close", tc.body))
		if rec.Code != tc.status {
			t.Fatalf("body=%s status=%d want=%d", tc.body, rec.Code, tc.status)
		}
	}
	if len(targets) != 1 {
		t.Fatalf("unexpected destructive calls: %v", targets)
	}
	s.CloseWindow = func(string) error { return errors.New("pane already gone") }
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, sessionJSONRequest("POST", "/api/window/close", `{"pane":"%11"}`))
	if rec.Code != 500 {
		t.Fatalf("status=%d", rec.Code)
	}
}

func TestWindowCloseRequestGuardsAndPeerProxy(t *testing.T) {
	peer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/window/close" || r.URL.Query().Get("peer") != "" {
			t.Errorf("wrong peer URL: %s", r.URL)
		}
		if !requireSessionMutationRequest(w, r) {
			t.Error("hub did not clear browser origin metadata")
			return
		}
		w.WriteHeader(200)
	}))
	defer peer.Close()
	s := &Server{Peers: []statusd.Peer{{Name: "mini", URL: peer.URL}}}
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, sessionJSONRequest("POST", "/api/window/close?peer=mini", `{"pane":"%11"}`))
	if rec.Code != 200 {
		t.Fatalf("peer status=%d body=%s", rec.Code, rec.Body.String())
	}
	for _, tc := range []struct {
		method, site, contentType string
		status                    int
	}{
		{"GET", "same-origin", "application/json", 405},
		{"POST", "cross-site", "application/json", 403},
		{"POST", "same-origin", "text/plain", 415},
	} {
		req := sessionJSONRequest(tc.method, "/api/window/close", `{"pane":"%11"}`)
		req.Header.Set("Sec-Fetch-Site", tc.site)
		req.Header.Set("Content-Type", tc.contentType)
		rec = httptest.NewRecorder()
		s.Handler().ServeHTTP(rec, req)
		if rec.Code != tc.status {
			t.Fatalf("status=%d want=%d", rec.Code, tc.status)
		}
	}
}
