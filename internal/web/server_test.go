package web

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func newTestServer(t *testing.T, statePath string, capture func(string, int) (string, error)) http.Handler {
	t.Helper()
	s := &Server{StatePath: statePath, CapturePane: capture}
	return s.Handler()
}

func TestSnapshotServesFileVerbatim(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "status.json")
	body := `{"version":1,"summary":{"sessions":2}}`
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	handler := newTestServer(t, path, nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/snapshot", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if got := rec.Body.String(); got != body {
		t.Fatalf("body = %q, want %q", got, body)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Fatalf("content-type = %q", ct)
	}
}

func TestSnapshotMissingFileReturns503(t *testing.T) {
	handler := newTestServer(t, filepath.Join(t.TempDir(), "absent.json"), nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/snapshot", nil))

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", rec.Code)
	}
}

func TestPaneCapturesByPaneID(t *testing.T) {
	var gotTarget string
	var gotLines int
	capture := func(target string, lines int) (string, error) {
		gotTarget, gotLines = target, lines
		return "hello from pane", nil
	}
	handler := newTestServer(t, "", capture)

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/pane?pane=%2512&lines=80", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if gotTarget != "%12" {
		t.Fatalf("capture target = %q, want %%12", gotTarget)
	}
	if gotLines != 80 {
		t.Fatalf("capture lines = %d, want 80", gotLines)
	}
	var payload struct {
		Pane    string `json:"pane"`
		Lines   int    `json:"lines"`
		Content string `json:"content"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Pane != "%12" || payload.Content != "hello from pane" {
		t.Fatalf("payload = %+v", payload)
	}
}

func TestPaneRejectsNonPaneIDTargets(t *testing.T) {
	called := false
	capture := func(string, int) (string, error) {
		called = true
		return "", nil
	}
	handler := newTestServer(t, "", capture)

	for _, bad := range []string{"", "session:0.1", "-X", "%12;rm", "12"} {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/pane?pane="+bad, nil)
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("pane=%q status = %d, want 400", bad, rec.Code)
		}
	}
	if called {
		t.Fatal("capture must not run for rejected pane ids")
	}
}

func TestPaneClampsLines(t *testing.T) {
	var gotLines int
	capture := func(_ string, lines int) (string, error) {
		gotLines = lines
		return "", nil
	}
	handler := newTestServer(t, "", capture)

	rec := httptest.NewRecorder()
	url := fmt.Sprintf("/api/pane?pane=%%251&lines=%d", maxCaptureLines+1000)
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, url, nil))

	if gotLines != maxCaptureLines {
		t.Fatalf("lines = %d, want clamp to %d", gotLines, maxCaptureLines)
	}
}

func TestPaneCaptureErrorReturns502(t *testing.T) {
	capture := func(string, int) (string, error) {
		return "", fmt.Errorf("tmux capture-pane failed")
	}
	handler := newTestServer(t, "", capture)

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/pane?pane=%251", nil))

	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502", rec.Code)
	}
}

func TestIndexPageServed(t *testing.T) {
	handler := newTestServer(t, "", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "<title>tmact") {
		t.Fatal("index page body missing expected title")
	}
}
