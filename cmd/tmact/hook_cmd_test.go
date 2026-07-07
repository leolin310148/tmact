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
	for _, topic := range []string{"hook", "hook init", "hook emit"} {
		if _, ok := commandHelpFor(topic); !ok {
			t.Fatalf("help catalog missing %q", topic)
		}
	}
}
