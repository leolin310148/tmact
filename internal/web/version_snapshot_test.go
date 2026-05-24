package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

/* ---- HTTP endpoints ---- */

func TestSnapshotServesFileVerbatim(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "status.json")
	body := `{"version":1,"summary":{"sessions":2}}`
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	handler := (&Server{StatePath: path}).Handler()
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/snapshot", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if got := rec.Body.String(); got != body {
		t.Fatalf("body = %q, want %q", got, body)
	}
}

func TestSnapshotMissingFileReturns503(t *testing.T) {
	handler := (&Server{StatePath: filepath.Join(t.TempDir(), "absent.json")}).Handler()
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/snapshot", nil))

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", rec.Code)
	}
}

func TestVersionReturnsBuildTime(t *testing.T) {
	handler := (&Server{BuildTime: "2026-05-22T06:01:39Z"}).Handler()
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/version", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var got struct {
		BuildTime string `json:"build_time"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.BuildTime != "2026-05-22T06:01:39Z" {
		t.Fatalf("build_time = %q", got.BuildTime)
	}
}

func TestVersionRejectsPost(t *testing.T) {
	handler := (&Server{}).Handler()
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/version", nil))

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want 405", rec.Code)
	}
	if rec.Header().Get("Allow") != http.MethodGet {
		t.Fatalf("Allow = %q, want GET", rec.Header().Get("Allow"))
	}
}
