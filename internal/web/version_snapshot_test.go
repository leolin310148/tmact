package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

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

func TestHealthReturnsLatestSnapshotStatus(t *testing.T) {
	store := statusd.NewStore()
	ts := time.Now().Add(-250 * time.Millisecond)
	store.Publish(statusd.Snapshot{
		Version:      1,
		Timestamp:    ts,
		IntervalMS:   500,
		StaleAfterMS: 10_000,
		Summary:      statusd.Summary{Sessions: 2, Panes: 3},
	})

	handler := (&Server{Store: store}).Handler()
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/health", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var got struct {
		OK                bool            `json:"ok"`
		SnapshotAvailable bool            `json:"snapshot_available"`
		SnapshotAgeMS     int64           `json:"snapshot_age_ms"`
		IntervalMS        int64           `json:"interval_ms"`
		StaleAfterMS      int64           `json:"stale_after_ms"`
		Summary           statusd.Summary `json:"summary"`
		ErrorCount        int             `json:"error_count"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if !got.OK || !got.SnapshotAvailable {
		t.Fatalf("health = %#v, want ok snapshot", got)
	}
	if got.SnapshotAgeMS <= 0 {
		t.Fatalf("snapshot_age_ms = %d, want positive", got.SnapshotAgeMS)
	}
	if got.IntervalMS != 500 || got.StaleAfterMS != 10_000 {
		t.Fatalf("interval/stale = %d/%d", got.IntervalMS, got.StaleAfterMS)
	}
	if got.Summary.Sessions != 2 || got.Summary.Panes != 3 {
		t.Fatalf("summary = %#v", got.Summary)
	}
}

func TestHealthReturns503WhenSnapshotMissing(t *testing.T) {
	handler := (&Server{Store: statusd.NewStore()}).Handler()
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/health", nil))

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", rec.Code)
	}
	var got struct {
		OK                bool `json:"ok"`
		SnapshotAvailable bool `json:"snapshot_available"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if got.OK || got.SnapshotAvailable {
		t.Fatalf("health = %#v, want unavailable", got)
	}
}

func TestHealthRejectsPost(t *testing.T) {
	handler := (&Server{}).Handler()
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/health", nil))

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want 405", rec.Code)
	}
	if rec.Header().Get("Allow") != http.MethodGet {
		t.Fatalf("Allow = %q, want GET", rec.Header().Get("Allow"))
	}
}
