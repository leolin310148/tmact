package shellhook

import (
	"strings"
	"testing"
	"time"
)

func TestParseEventDefaultsAndValidates(t *testing.T) {
	now := time.Date(2026, 7, 7, 10, 0, 0, 0, time.UTC)
	event, err := ParseEvent([]byte(`{"type":"preexec","pane_id":"%5","command":"go test ./...","command_id":"c1"}`), now)
	if err != nil {
		t.Fatalf("ParseEvent: %v", err)
	}
	if event.Version != EventVersion {
		t.Fatalf("version = %d, want %d", event.Version, EventVersion)
	}
	if !event.Timestamp.Equal(now) {
		t.Fatalf("timestamp = %v, want %v", event.Timestamp, now)
	}
	if event.Type != TypePreexec || event.PaneID != "%5" || event.CommandID != "c1" {
		t.Fatalf("unexpected event: %+v", event)
	}
}

func TestParseEventKeepsExplicitFields(t *testing.T) {
	now := time.Date(2026, 7, 7, 10, 0, 0, 0, time.UTC)
	ts := now.Add(-2 * time.Second)
	raw := `{"v":1,"type":"precmd","pane_id":"%12","session_id":"$3","exit_code":0,"ts":"` + ts.Format(time.RFC3339) + `"}`
	event, err := ParseEvent([]byte(raw), now)
	if err != nil {
		t.Fatalf("ParseEvent: %v", err)
	}
	if !event.Timestamp.Equal(ts) {
		t.Fatalf("timestamp = %v, want %v", event.Timestamp, ts)
	}
	if event.ExitCode == nil || *event.ExitCode != 0 {
		t.Fatalf("exit_code = %v, want 0", event.ExitCode)
	}
}

func TestParseEventRejectsInvalid(t *testing.T) {
	now := time.Now()
	cases := []struct {
		name string
		raw  string
		want string
	}{
		{"bad json", `{`, "decode shell hook event"},
		{"missing type", `{"pane_id":"%1"}`, "type is required"},
		{"unknown type", `{"type":"postexec","pane_id":"%1"}`, "unsupported shell hook event type"},
		{"missing pane", `{"type":"preexec"}`, "pane_id is required"},
		{"bad pane id", `{"type":"preexec","pane_id":"main:0.0"}`, "not a tmux pane id"},
		{"bad version", `{"v":2,"type":"preexec","pane_id":"%1"}`, "unsupported shell hook event version"},
		{"long command", `{"type":"preexec","pane_id":"%1","command":"` + strings.Repeat("x", maxCommandLen+1) + `"}`, "command exceeds"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ParseEvent([]byte(tc.raw), now)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want containing %q", err, tc.want)
			}
		})
	}
}
