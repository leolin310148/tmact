package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func getHumanActive(t *testing.T, server *Server, query string) (int, HumanActiveStatus) {
	t.Helper()
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/human-active"+query, nil))
	var status HumanActiveStatus
	if rec.Code == http.StatusOK {
		if err := json.Unmarshal(rec.Body.Bytes(), &status); err != nil {
			t.Fatalf("decode body %q: %v", rec.Body.String(), err)
		}
	}
	return rec.Code, status
}

func TestHumanActiveInactiveBeforeAnyActivity(t *testing.T) {
	code, status := getHumanActive(t, &Server{}, "")
	if code != http.StatusOK {
		t.Fatalf("status code = %d, want 200", code)
	}
	if status.Active {
		t.Fatal("active = true before any activity, want false")
	}
	if status.LastActivity != nil || status.IdleSeconds != nil {
		t.Fatalf("expected omitted last_activity/idle_seconds, got %+v", status)
	}
	if status.ThresholdSeconds != DefaultHumanActiveThreshold.Seconds() {
		t.Fatalf("threshold_seconds = %v, want %v", status.ThresholdSeconds, DefaultHumanActiveThreshold.Seconds())
	}
}

func TestHumanActivityReportMarksActive(t *testing.T) {
	server := &Server{}
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/human-activity", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("report status = %d body=%s", rec.Code, rec.Body.String())
	}

	code, status := getHumanActive(t, server, "")
	if code != http.StatusOK || !status.Active {
		t.Fatalf("code=%d status=%+v, want active", code, status)
	}
	if status.LastActivity == nil || status.IdleSeconds == nil {
		t.Fatalf("expected last_activity/idle_seconds, got %+v", status)
	}
}

func TestHumanActivityMethodNotAllowed(t *testing.T) {
	rec := httptest.NewRecorder()
	(&Server{}).Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/human-activity", nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want 405", rec.Code)
	}
}

func TestHumanActiveThresholdBoundary(t *testing.T) {
	now := time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC)
	server := &Server{humanNow: func() time.Time { return now }}
	server.recordHumanActivity()

	// 9 minutes idle: still active at the default 10m threshold.
	now = now.Add(9 * time.Minute)
	if _, status := getHumanActive(t, server, ""); !status.Active {
		t.Fatalf("9m idle: active=false, want true (status=%+v)", status)
	}

	// 11 minutes idle: inactive by default, active with a wider threshold.
	now = now.Add(2 * time.Minute)
	if _, status := getHumanActive(t, server, ""); status.Active {
		t.Fatalf("11m idle: active=true, want false (status=%+v)", status)
	}
	if _, status := getHumanActive(t, server, "?threshold=30m"); !status.Active {
		t.Fatalf("11m idle with 30m threshold: active=false, want true (status=%+v)", status)
	}
	if code, _ := getHumanActive(t, server, "?threshold=bogus"); code != http.StatusBadRequest {
		t.Fatalf("bogus threshold code = %d, want 400", code)
	}
}

func TestPaneInputCountsAsHumanActivity(t *testing.T) {
	server := &Server{
		SendText: func(target, text string, enter bool) error { return nil },
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/pane/input?pane=%257", strings.NewReader(`{"t":"send","s":"hello"}`))
	server.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("input status = %d body=%s", rec.Code, rec.Body.String())
	}
	if _, status := getHumanActive(t, server, ""); !status.Active {
		t.Fatalf("pane input did not mark human active: %+v", status)
	}
}
