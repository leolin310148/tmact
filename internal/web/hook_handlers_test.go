package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/leolin310148/tmact/internal/shellhook"
)

func postHookEvent(t *testing.T, server *Server, body string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/hook-event", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	server.Handler().ServeHTTP(rec, req)
	return rec
}

func TestHookEventRecordsIntoStore(t *testing.T) {
	store := shellhook.NewStore()
	server := &Server{HookRecord: store.Record}

	rec := postHookEvent(t, server, `{"type":"preexec","pane_id":"%7","command_id":"c1","command":"make test"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	state, ok := store.State("%7")
	if !ok || state.Active == nil || state.Active.Command != "make test" {
		t.Fatalf("store state = %+v ok=%t", state, ok)
	}
}

func TestHookEventRejectsInvalidEvent(t *testing.T) {
	server := &Server{HookRecord: shellhook.NewStore().Record}
	rec := postHookEvent(t, server, `{"type":"preexec","pane_id":"not-a-pane"}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestHookEventMethodNotAllowed(t *testing.T) {
	server := &Server{HookRecord: shellhook.NewStore().Record}
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/hook-event", nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want 405", rec.Code)
	}
}

func TestHookEventUnavailableWithoutRecorder(t *testing.T) {
	rec := postHookEvent(t, &Server{}, `{"type":"preexec","pane_id":"%7"}`)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", rec.Code)
	}
}

func TestHookEventRejectsTCPConnections(t *testing.T) {
	store := shellhook.NewStore()
	server := &Server{HookRecord: store.Record}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/hook-event",
		strings.NewReader(`{"type":"preexec","pane_id":"%7"}`))
	// Serve's ConnContext tags every non-unix connection with this key.
	req = req.WithContext(context.WithValue(req.Context(), tcpConnContextKey{}, true))
	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rec.Code)
	}
	if _, ok := store.State("%7"); ok {
		t.Fatal("TCP-origin event was recorded")
	}
}

func TestHookEventBodyTooLarge(t *testing.T) {
	server := &Server{HookRecord: shellhook.NewStore().Record}
	rec := postHookEvent(t, server, `{"command":"`+strings.Repeat("x", hookEventMaxBodyBytes+1)+`"}`)
	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want 413", rec.Code)
	}
}
