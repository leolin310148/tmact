package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/leolin310148/tmact/internal/statusd"
)

/* ---- HTTP endpoints ---- */

func TestSnapshotReturnsLatestFromStore(t *testing.T) {
	store := statusd.NewStore()
	store.Publish(statusd.Snapshot{Version: 1, Summary: statusd.Summary{Sessions: 2}})

	handler := (&Server{Store: store}).Handler()
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/snapshot", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var got statusd.Snapshot
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if got.Summary.Sessions != 2 {
		t.Fatalf("sessions = %d, want 2", got.Summary.Sessions)
	}
}

func TestSnapshotReturns503WhenStoreEmpty(t *testing.T) {
	handler := (&Server{Store: statusd.NewStore()}).Handler()
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/snapshot", nil))

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", rec.Code)
	}
}

func TestSnapshotReturns503WhenStoreNil(t *testing.T) {
	handler := (&Server{}).Handler()
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
