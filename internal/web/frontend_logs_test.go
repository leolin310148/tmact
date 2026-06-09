package web

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHandleFrontendLogsAcceptsAndLogsPayload(t *testing.T) {
	var lines []string
	s := &Server{
		Logf: func(format string, args ...any) {
			lines = append(lines, fmt.Sprintf(format, args...))
		},
	}
	body := `{
		"session_id":"session-1",
		"sent_at":"2026-06-09T10:00:00.000Z",
		"device":{
			"platform":"macOS",
			"user_agent_summary":"Mozilla/5.0",
			"viewport":{"width":1440,"height":900},
			"screen":{"width":1440,"height":900},
			"device_pixel_ratio":2,
			"orientation":"landscape",
			"visibility_state":"visible",
			"online":true
		},
		"entries":[{"ts":"2026-06-09T10:00:00.000Z","level":"warn","event":"pane_ws","message":"reconnecting","data":{"pane":"%12","state":"reconnecting"}}]
	}`
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/frontend-logs", strings.NewReader(body)))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"ok":true`) {
		t.Fatalf("body = %s", rec.Body.String())
	}
	if len(lines) != 1 || !strings.HasPrefix(lines[0], "frontend log: ") {
		t.Fatalf("logs = %#v", lines)
	}
	got := decodeLoggedFrontendPayload(t, lines[0])
	if got["session_id"] != "session-1" {
		t.Fatalf("session_id = %#v", got["session_id"])
	}
	entries := got["entries"].([]any)
	entry := entries[0].(map[string]any)
	if entry["level"] != "warn" || entry["event"] != "pane_ws" {
		t.Fatalf("entry = %#v", entry)
	}
}

func TestHandleFrontendLogsRejectsMethodBadJSONAndOversize(t *testing.T) {
	handler := (&Server{}).Handler()

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/frontend-logs", nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("GET status = %d", rec.Code)
	}

	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/frontend-logs", strings.NewReader("{")))
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "invalid JSON body") {
		t.Fatalf("bad JSON status=%d body=%s", rec.Code, rec.Body.String())
	}

	rec = httptest.NewRecorder()
	tooLarge := `{"session_id":"` + strings.Repeat("x", frontendLogsMaxBodyBytes) + `","entries":[]}`
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/frontend-logs", strings.NewReader(tooLarge)))
	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("oversize status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestHandleFrontendLogsLimitsEntriesAndSanitizesFields(t *testing.T) {
	var lines []string
	s := &Server{
		Logf: func(format string, args ...any) {
			lines = append(lines, fmt.Sprintf(format, args...))
		},
	}
	entries := make([]string, 0, frontendLogsMaxEntries+5)
	for i := 0; i < frontendLogsMaxEntries+5; i++ {
		blob := "d"
		if i == 0 {
			blob = strings.Repeat("d", 5000)
		}
		entries = append(entries, fmt.Sprintf(
			`{"ts":"now","level":"bad","event":"api_error","message":"%s\n%s","data":{"blob":"%s","n":%d}}`,
			strings.Repeat("m", frontendLogMaxMessage),
			strings.Repeat("z", 50),
			blob,
			i,
		))
	}
	body := `{"session_id":"session-1","sent_at":"now","entries":[` + strings.Join(entries, ",") + `]}`
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/frontend-logs", strings.NewReader(body)))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	got := decodeLoggedFrontendPayload(t, lines[0])
	gotEntries := got["entries"].([]any)
	if len(gotEntries) != frontendLogsMaxEntries {
		t.Fatalf("entry count = %d", len(gotEntries))
	}
	entry := gotEntries[0].(map[string]any)
	if entry["level"] != "info" {
		t.Fatalf("level = %#v", entry["level"])
	}
	msg := entry["message"].(string)
	if strings.Contains(msg, "\n") || len([]rune(msg)) != frontendLogMaxMessage {
		t.Fatalf("message len=%d contains newline=%v", len([]rune(msg)), strings.Contains(msg, "\n"))
	}
	dataBytes, err := json.Marshal(entry["data"])
	if err != nil {
		t.Fatal(err)
	}
	if len(dataBytes) > frontendLogMaxDataBytes {
		t.Fatalf("data length = %d", len(dataBytes))
	}
}

func decodeLoggedFrontendPayload(t *testing.T, line string) map[string]any {
	t.Helper()
	raw := strings.TrimPrefix(line, "frontend log: ")
	var got map[string]any
	if err := json.Unmarshal([]byte(raw), &got); err != nil {
		t.Fatalf("logged JSON did not decode: %v\n%s", err, raw)
	}
	return got
}
