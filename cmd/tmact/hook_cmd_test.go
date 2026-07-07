package main

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/leolin310148/tmact/internal/shellhook"
)

func stubHookSend(t *testing.T, send func(string, shellhook.Event, time.Duration) error) {
	t.Helper()
	old := sendHookEvent
	sendHookEvent = send
	t.Cleanup(func() { sendHookEvent = old })
}

func stubHookFetch(t *testing.T, fetch func(string, string, time.Duration) (map[string]shellhook.PaneState, error)) {
	t.Helper()
	old := fetchHookStates
	fetchHookStates = fetch
	t.Cleanup(func() { fetchHookStates = old })
}

func TestHookInitPrintsScript(t *testing.T) {
	for _, shell := range shellhook.Shells {
		output, err := captureRun(t, "hook", "init", shell)
		if err != nil {
			t.Fatalf("hook init %s: %v", shell, err)
		}
		if !strings.Contains(output, "tmact hook emit") || !strings.Contains(output, "TMUX_PANE") {
			t.Fatalf("hook init %s output missing hook body:\n%s", shell, output)
		}
	}
}

func TestHookInitRejectsUnknownShell(t *testing.T) {
	if _, err := captureRun(t, "hook", "init", "powershell"); err == nil {
		t.Fatal("hook init accepted unsupported shell")
	}
	if _, err := captureRun(t, "hook", "init"); err == nil {
		t.Fatal("hook init accepted missing shell argument")
	}
}

func TestHookEmitSendsEvent(t *testing.T) {
	t.Setenv("TMUX_PANE", "%9")
	t.Setenv(hookSocketEnv, "")
	var gotSocket string
	var gotEvent shellhook.Event
	stubHookSend(t, func(socket string, e shellhook.Event, _ time.Duration) error {
		gotSocket = socket
		gotEvent = e
		return nil
	})

	_, err := captureRun(t, "hook", "emit", "--type", "precmd", "--command-id", "c1", "--exit-code", "0")
	if err != nil {
		t.Fatalf("hook emit: %v", err)
	}
	if gotEvent.PaneID != "%9" {
		t.Fatalf("pane id = %q, want env fallback %%9", gotEvent.PaneID)
	}
	if gotEvent.Type != shellhook.TypePrecmd || gotEvent.CommandID != "c1" {
		t.Fatalf("event = %+v", gotEvent)
	}
	if gotEvent.ExitCode == nil || *gotEvent.ExitCode != 0 {
		t.Fatalf("exit code = %v, want 0", gotEvent.ExitCode)
	}
	if gotSocket == "" {
		t.Fatal("socket path not defaulted")
	}
}

func TestHookEmitQuietSwallowsFailures(t *testing.T) {
	t.Setenv("TMUX_PANE", "%9")
	stubHookSend(t, func(string, shellhook.Event, time.Duration) error {
		return shellhook.ErrDaemonUnavailable
	})

	if _, err := captureRun(t, "hook", "emit", "--quiet", "--type", "preexec", "--command", "ls"); err != nil {
		t.Fatalf("quiet emit returned error: %v", err)
	}
	// Invalid input is swallowed too: hooks must never break a prompt.
	if _, err := captureRun(t, "hook", "emit", "--quiet", "--type", "nope"); err != nil {
		t.Fatalf("quiet emit with bad type returned error: %v", err)
	}
}

func TestHookEmitSurfacesFailuresWithoutQuiet(t *testing.T) {
	t.Setenv("TMUX_PANE", "%9")
	stubHookSend(t, func(string, shellhook.Event, time.Duration) error {
		return shellhook.ErrDaemonUnavailable
	})
	if _, err := captureRun(t, "hook", "emit", "--type", "preexec"); !errors.Is(err, shellhook.ErrDaemonUnavailable) {
		t.Fatalf("err = %v, want ErrDaemonUnavailable", err)
	}
}

func TestBuildHookEventFromStdinJSON(t *testing.T) {
	now := time.Date(2026, 7, 7, 10, 0, 0, 0, time.UTC)
	event, err := buildHookEvent(hookEmitInputs{Stdin: true},
		func(string) string { return "" },
		strings.NewReader(`{"type":"preexec","pane_id":"%3","command":"vim"}`), now)
	if err != nil {
		t.Fatalf("buildHookEvent: %v", err)
	}
	if event.PaneID != "%3" || event.Command != "vim" || !event.Timestamp.Equal(now) {
		t.Fatalf("event = %+v", event)
	}
}

func TestBuildHookEventValidation(t *testing.T) {
	now := time.Now()
	env := func(key string) string { return "" }
	if _, err := buildHookEvent(hookEmitInputs{Type: "preexec"}, env, strings.NewReader(""), now); err == nil {
		t.Fatal("accepted missing pane id with no env fallback")
	}
	if _, err := buildHookEvent(hookEmitInputs{Type: "precmd", PaneID: "%1", ExitCode: "abc"}, env, strings.NewReader(""), now); err == nil {
		t.Fatal("accepted non-numeric exit code")
	}
}

func TestHelpCatalogIncludesHook(t *testing.T) {
	for _, topic := range []string{"hook", "hook init", "hook emit", "hook state", "hook doctor"} {
		if _, ok := commandHelpFor(topic); !ok {
			t.Fatalf("help catalog missing %q", topic)
		}
	}
}

func TestHookStatePrintsRecordedPanes(t *testing.T) {
	t.Setenv(hookSocketEnv, "")
	stubHookFetch(t, func(_, paneID string, _ time.Duration) (map[string]shellhook.PaneState, error) {
		return map[string]shellhook.PaneState{
			"%5": {PaneID: "%5", SessionID: "main", Active: &shellhook.CommandRecord{CommandID: "c1", Command: "make test"}},
		}, nil
	})
	out, err := captureRun(t, "hook", "state")
	if err != nil {
		t.Fatalf("hook state: %v", err)
	}
	if !strings.Contains(out, "%5") || !strings.Contains(out, "make test") || !strings.Contains(out, "active") {
		t.Fatalf("output missing pane summary:\n%s", out)
	}
}

func TestHookStateReportsNoEvents(t *testing.T) {
	t.Setenv(hookSocketEnv, "")
	stubHookFetch(t, func(_, _ string, _ time.Duration) (map[string]shellhook.PaneState, error) {
		return map[string]shellhook.PaneState{}, nil
	})
	out, err := captureRun(t, "hook", "state")
	if err != nil {
		t.Fatalf("hook state: %v", err)
	}
	if !strings.Contains(out, "no shell hook events recorded") {
		t.Fatalf("output missing empty notice:\n%s", out)
	}
}

func TestHookStatePropagatesFetchError(t *testing.T) {
	t.Setenv(hookSocketEnv, "")
	stubHookFetch(t, func(_, _ string, _ time.Duration) (map[string]shellhook.PaneState, error) {
		return nil, shellhook.ErrDaemonUnavailable
	})
	if _, err := captureRun(t, "hook", "state"); !errors.Is(err, shellhook.ErrDaemonUnavailable) {
		t.Fatalf("err = %v, want ErrDaemonUnavailable", err)
	}
}

func TestBuildHookDoctorHealthyWhenPaneHasEvents(t *testing.T) {
	fetch := func(_, _ string, _ time.Duration) (map[string]shellhook.PaneState, error) {
		return map[string]shellhook.PaneState{"%5": {PaneID: "%5", Active: &shellhook.CommandRecord{Command: "vim"}}}, nil
	}
	report := buildHookDoctor(hookDoctorInputs{Socket: "/tmp/s.sock", SocketExists: true, PaneID: "%5", InTmux: true}, fetch)
	if !report.Healthy || !report.DaemonReachable || !report.PaneHasEvents {
		t.Fatalf("report = %+v", report)
	}
	if report.Pane == nil || report.Pane.Active == nil {
		t.Fatalf("expected pane detail, got %+v", report.Pane)
	}
}

func TestBuildHookDoctorDoesNotMislabelCheckedPaneAsEnvPane(t *testing.T) {
	fetch := func(_, _ string, _ time.Duration) (map[string]shellhook.PaneState, error) {
		return map[string]shellhook.PaneState{"%99999": {PaneID: "%99999", Active: &shellhook.CommandRecord{Command: "vim"}}}, nil
	}
	report := buildHookDoctor(hookDoctorInputs{Socket: "/tmp/s.sock", SocketExists: true, PaneID: "%99999", InTmux: true}, fetch)
	for _, check := range report.Checks {
		if check.Name == "tmux" && strings.Contains(check.Detail, "TMUX_PANE=%99999") {
			t.Fatalf("tmux check mislabels explicit pane as env pane: %+v", check)
		}
	}
}

func TestBuildHookDoctorWarnsWhenPaneUnseen(t *testing.T) {
	fetch := func(_, _ string, _ time.Duration) (map[string]shellhook.PaneState, error) {
		return map[string]shellhook.PaneState{}, nil
	}
	report := buildHookDoctor(hookDoctorInputs{Socket: "/tmp/s.sock", SocketExists: true, PaneID: "%5", InTmux: true}, fetch)
	// Daemon reachable but no events for the pane: healthy (warn, not fail).
	if !report.Healthy || report.PaneHasEvents {
		t.Fatalf("report = %+v", report)
	}
	if !hasDoctorStatus(report, "pane_events", "warn") {
		t.Fatalf("expected pane_events warn, got %+v", report.Checks)
	}
}

func TestBuildHookDoctorFailsWhenDaemonUnreachable(t *testing.T) {
	fetch := func(_, _ string, _ time.Duration) (map[string]shellhook.PaneState, error) {
		return nil, shellhook.ErrDaemonUnavailable
	}
	report := buildHookDoctor(hookDoctorInputs{Socket: "/tmp/s.sock", SocketExists: false, PaneID: "%5", InTmux: true}, fetch)
	if report.Healthy || report.DaemonReachable {
		t.Fatalf("report = %+v", report)
	}
	if !hasDoctorStatus(report, "daemon", "fail") {
		t.Fatalf("expected daemon fail, got %+v", report.Checks)
	}
}

func TestHookDoctorExitsNonZeroWhenUnhealthy(t *testing.T) {
	t.Setenv(hookSocketEnv, "")
	t.Setenv("TMUX_PANE", "")
	stubHookFetch(t, func(_, _ string, _ time.Duration) (map[string]shellhook.PaneState, error) {
		return nil, shellhook.ErrDaemonUnavailable
	})
	out, err := captureRun(t, "hook", "doctor")
	if err == nil {
		t.Fatalf("expected non-zero exit, got nil (output:\n%s)", out)
	}
	if !strings.Contains(out, "daemon") {
		t.Fatalf("doctor output missing daemon check:\n%s", out)
	}
}

func hasDoctorStatus(report hookDoctorReport, name, status string) bool {
	for _, c := range report.Checks {
		if c.Name == name && c.Status == status {
			return true
		}
	}
	return false
}
