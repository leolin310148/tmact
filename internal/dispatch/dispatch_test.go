package dispatch_test

import (
	"errors"
	"testing"
	"time"

	"github.com/leolin310148/tmact/internal/dispatch"
	"github.com/leolin310148/tmact/internal/panestatus"
	"github.com/leolin310148/tmact/internal/tmux"
)

type paste struct {
	target string
	text   string
	enter  bool
}

type keyPress struct {
	target string
	keys   []string
}

type recorder struct {
	pastes      []paste
	keys        []keyPress
	newSessions int
}

// enterCount returns how many bare Enter keystrokes the recorder captured.
func enterCount(rec *recorder) int {
	n := 0
	for _, k := range rec.keys {
		if len(k.keys) == 1 && k.keys[0] == "Enter" {
			n++
		}
	}
	return n
}

func baseDeps() (*recorder, dispatch.Deps) {
	rec := &recorder{}
	deps := dispatch.Deps{
		ListLayout: func() (tmux.Layout, error) {
			return tmux.Layout{Sessions: map[string]bool{}}, nil
		},
		ListSessionPanes: func(string) ([]tmux.Pane, error) {
			return nil, errors.New("ListSessionPanes not configured")
		},
		CapturePane: func(string, int) (string, error) {
			return "", errors.New("CapturePane not configured")
		},
		NewSession: func(string, string, string, []string) error {
			rec.newSessions++
			return nil
		},
		PasteText: func(target, text string, enter bool) error {
			rec.pastes = append(rec.pastes, paste{target, text, enter})
			return nil
		},
		SendKeys: func(target string, keys []string) error {
			rec.keys = append(rec.keys, keyPress{target, keys})
			return nil
		},
		ProcessRuntime: func(int) panestatus.RuntimeDetection {
			return panestatus.RuntimeDetection{Runtime: panestatus.RuntimeUnknown}
		},
		Sleep: func(time.Duration) {},
		Now:   func() time.Time { return time.Unix(0, 0) },
	}
	return rec, deps
}

func baseOpts() dispatch.Options {
	return dispatch.Options{
		Session:      "work",
		Dir:          ".",
		Agent:        "claude",
		Prompt:       "do the thing",
		ReadyTimeout: 30 * time.Second,
	}
}

func claudePane() tmux.Pane {
	return tmux.Pane{
		Session:        "work",
		PaneID:         "%1",
		PanePID:        111,
		CurrentCommand: "node",
		WindowName:     "claude",
		Active:         true,
		WindowActive:   true,
	}
}

func stepStatus(t *testing.T, report dispatch.Report, name string) string {
	t.Helper()
	for _, step := range report.Steps {
		if step.Name == name {
			return step.Status
		}
	}
	t.Fatalf("step %q not found in %+v", name, report.Steps)
	return ""
}

func TestRunRejectsUnsupportedAgent(t *testing.T) {
	_, deps := baseDeps()
	opts := baseOpts()
	opts.Agent = "rustc"
	if _, err := dispatch.RunWithDeps(opts, deps); err == nil {
		t.Fatal("expected error for unsupported agent")
	}
}

func TestRunRejectsEmptyPrompt(t *testing.T) {
	_, deps := baseDeps()
	opts := baseOpts()
	opts.Prompt = "  "
	if _, err := dispatch.RunWithDeps(opts, deps); err == nil {
		t.Fatal("expected error for empty prompt")
	}
}

func TestDryRunNewSessionPlan(t *testing.T) {
	rec, deps := baseDeps()
	report, err := dispatch.RunWithDeps(baseOpts(), deps)
	if err != nil {
		t.Fatalf("RunWithDeps: %v", err)
	}
	if report.SessionExisted {
		t.Fatal("session should not be reported as existing")
	}
	for _, name := range []string{"create-session", "launch-agent", "wait-ready", "send-prompt"} {
		if got := stepStatus(t, report, name); got != dispatch.StatusPlanned {
			t.Fatalf("step %q status = %q, want planned", name, got)
		}
	}
	if rec.newSessions != 0 || len(rec.pastes) != 0 {
		t.Fatalf("dry-run touched tmux: newSessions=%d pastes=%d", rec.newSessions, len(rec.pastes))
	}
}

func TestExecuteNewSession(t *testing.T) {
	rec, deps := baseDeps()
	deps.ListSessionPanes = func(string) ([]tmux.Pane, error) {
		return []tmux.Pane{claudePane()}, nil
	}
	// Pane 0: fresh shell. Pane 1: agent launched, input-ready. Pane 2:
	// prompt submitted, agent working.
	deps.CapturePane = func(string, int) (string, error) {
		switch {
		case len(rec.pastes) < 2:
			return "Claude Code\nready for input", nil
		default:
			return "Claude Code\nWorking... esc to interrupt", nil
		}
	}

	opts := baseOpts()
	opts.Execute = true
	report, err := dispatch.RunWithDeps(opts, deps)
	if err != nil {
		t.Fatalf("RunWithDeps: %v", err)
	}
	if rec.newSessions != 1 {
		t.Fatalf("newSessions = %d, want 1", rec.newSessions)
	}
	if report.Target != "%1" {
		t.Fatalf("target = %q, want %%1", report.Target)
	}
	want := []paste{{"%1", "claude", true}, {"%1", "do the thing", true}}
	if len(rec.pastes) != len(want) {
		t.Fatalf("pastes = %+v, want %+v", rec.pastes, want)
	}
	for i := range want {
		if rec.pastes[i] != want[i] {
			t.Fatalf("paste %d = %+v, want %+v", i, rec.pastes[i], want[i])
		}
	}
	if n := enterCount(rec); n != 0 {
		t.Fatalf("a working pane should need no re-sent Enter, got %d", n)
	}
	for _, name := range []string{"create-session", "launch-agent", "wait-ready", "send-prompt"} {
		if got := stepStatus(t, report, name); got != dispatch.StatusOK {
			t.Fatalf("step %q status = %q, want ok", name, got)
		}
	}
}

func TestExistingSessionReuseSameAgent(t *testing.T) {
	rec, deps := baseDeps()
	deps.ListLayout = func() (tmux.Layout, error) {
		return tmux.Layout{Sessions: map[string]bool{"work": true}}, nil
	}
	deps.ListSessionPanes = func(string) ([]tmux.Pane, error) {
		return []tmux.Pane{claudePane()}, nil
	}
	deps.CapturePane = func(string, int) (string, error) {
		if len(rec.pastes) >= 2 {
			return "Claude Code\nWorking... esc to interrupt", nil
		}
		return "Claude Code\nidle", nil
	}

	opts := baseOpts()
	opts.Execute = true
	report, err := dispatch.RunWithDeps(opts, deps)
	if err != nil {
		t.Fatalf("RunWithDeps: %v", err)
	}
	if !report.AgentWasRunning {
		t.Fatal("agent_was_running should be true")
	}
	want := []paste{{"%1", "/clear", true}, {"%1", "do the thing", true}}
	if len(rec.pastes) != len(want) {
		t.Fatalf("pastes = %+v, want %+v", rec.pastes, want)
	}
	for i := range want {
		if rec.pastes[i] != want[i] {
			t.Fatalf("paste %d = %+v, want %+v", i, rec.pastes[i], want[i])
		}
	}
}

func TestExistingSessionAgentBusy(t *testing.T) {
	_, deps := baseDeps()
	deps.ListLayout = func() (tmux.Layout, error) {
		return tmux.Layout{Sessions: map[string]bool{"work": true}}, nil
	}
	deps.ListSessionPanes = func(string) ([]tmux.Pane, error) {
		return []tmux.Pane{claudePane()}, nil
	}
	deps.CapturePane = func(string, int) (string, error) {
		return "Claude Code\nWorking... esc to interrupt", nil
	}

	if _, err := dispatch.RunWithDeps(baseOpts(), deps); err == nil {
		t.Fatal("expected error when the agent is busy")
	}
}

func TestExistingSessionDifferentAgent(t *testing.T) {
	_, deps := baseDeps()
	deps.ListLayout = func() (tmux.Layout, error) {
		return tmux.Layout{Sessions: map[string]bool{"work": true}}, nil
	}
	deps.ListSessionPanes = func(string) ([]tmux.Pane, error) {
		pane := claudePane()
		pane.WindowName = "codex"
		pane.CurrentCommand = "codex"
		return []tmux.Pane{pane}, nil
	}
	deps.CapturePane = func(string, int) (string, error) {
		return "OpenAI Codex\nidle", nil
	}

	if _, err := dispatch.RunWithDeps(baseOpts(), deps); err == nil {
		t.Fatal("expected error when a different agent is running")
	}
}

func TestExistingSessionShellLaunch(t *testing.T) {
	rec, deps := baseDeps()
	deps.ListLayout = func() (tmux.Layout, error) {
		return tmux.Layout{Sessions: map[string]bool{"work": true}}, nil
	}
	listCalls := 0
	deps.ListSessionPanes = func(string) ([]tmux.Pane, error) {
		listCalls++
		pane := tmux.Pane{
			Session:      "work",
			PaneID:       "%5",
			PanePID:      55,
			Active:       true,
			WindowActive: true,
		}
		if listCalls == 1 {
			pane.CurrentCommand = "zsh"
			pane.WindowName = "0"
		} else {
			pane.CurrentCommand = "node"
			pane.WindowName = "claude"
		}
		return []tmux.Pane{pane}, nil
	}
	deps.CapturePane = func(string, int) (string, error) {
		switch {
		case len(rec.pastes) == 0:
			return "user@host project %", nil
		case len(rec.pastes) >= 2:
			return "Claude Code\nWorking... esc to interrupt", nil
		default:
			return "Claude Code\nready", nil
		}
	}

	opts := baseOpts()
	opts.Execute = true
	report, err := dispatch.RunWithDeps(opts, deps)
	if err != nil {
		t.Fatalf("RunWithDeps: %v", err)
	}
	if report.AgentWasRunning {
		t.Fatal("agent_was_running should be false for a shell pane")
	}
	want := []paste{{"%5", "claude", true}, {"%5", "do the thing", true}}
	if len(rec.pastes) != len(want) {
		t.Fatalf("pastes = %+v, want %+v", rec.pastes, want)
	}
	for i := range want {
		if rec.pastes[i] != want[i] {
			t.Fatalf("paste %d = %+v, want %+v", i, rec.pastes[i], want[i])
		}
	}
	for _, name := range []string{"launch-agent", "wait-ready", "send-prompt"} {
		if got := stepStatus(t, report, name); got != dispatch.StatusOK {
			t.Fatalf("step %q status = %q, want ok", name, got)
		}
	}
}

func TestExecuteNewSessionResendsEnterWhenPromptStuck(t *testing.T) {
	rec, deps := baseDeps()
	deps.ListSessionPanes = func(string) ([]tmux.Pane, error) {
		return []tmux.Pane{claudePane()}, nil
	}
	// The first paste launches the agent; the second is the prompt. A cold
	// start swallows the first Enter on the prompt: the pane stays
	// input-ready until a second Enter is sent, then the agent starts working.
	deps.CapturePane = func(string, int) (string, error) {
		switch {
		case len(rec.pastes) < 2:
			return "Claude Code\nready", nil
		case enterCount(rec) == 0:
			return "Claude Code\n1 MCP server failed\n❯ do the thing", nil
		default:
			return "Claude Code\nWorking... esc to interrupt", nil
		}
	}

	opts := baseOpts()
	opts.Execute = true
	report, err := dispatch.RunWithDeps(opts, deps)
	if err != nil {
		t.Fatalf("RunWithDeps: %v", err)
	}
	if got := stepStatus(t, report, "send-prompt"); got != dispatch.StatusOK {
		t.Fatalf("send-prompt status = %q, want ok", got)
	}
	if n := enterCount(rec); n != 1 {
		t.Fatalf("expected exactly 1 re-sent Enter, got %d", n)
	}
	want := []paste{{"%1", "claude", true}, {"%1", "do the thing", true}}
	if len(rec.pastes) != len(want) {
		t.Fatalf("pastes = %+v, want %+v", rec.pastes, want)
	}
	if rec.keys[0].target != "%1" {
		t.Fatalf("re-sent Enter target = %q, want %%1", rec.keys[0].target)
	}
}

func TestExecuteNewSessionFailsWhenPromptNeverSubmits(t *testing.T) {
	rec, deps := baseDeps()
	deps.ListSessionPanes = func(string) ([]tmux.Pane, error) {
		return []tmux.Pane{claudePane()}, nil
	}
	// The agent launches and becomes ready, but the prompt never leaves the
	// input box, no matter how many times Enter is re-sent.
	deps.CapturePane = func(string, int) (string, error) {
		if len(rec.pastes) < 2 {
			return "Claude Code\nready", nil
		}
		return "Claude Code\n❯ do the thing", nil
	}

	opts := baseOpts()
	opts.Execute = true
	report, err := dispatch.RunWithDeps(opts, deps)
	if err == nil {
		t.Fatal("expected an error when the prompt never submits")
	}
	if got := stepStatus(t, report, "send-prompt"); got != dispatch.StatusFailed {
		t.Fatalf("send-prompt status = %q, want failed", got)
	}
	if n := enterCount(rec); n == 0 {
		t.Fatal("expected dispatch to re-send Enter before giving up")
	}
}

// TestExecuteNewSessionRepastesWhenPasteLost covers a cold start where the
// agent UI was still painting when the prompt was pasted and dropped the text
// entirely: the input box stays empty until dispatch re-pastes.
func TestExecuteNewSessionRepastesWhenPasteLost(t *testing.T) {
	rec, deps := baseDeps()
	deps.ListSessionPanes = func(string) ([]tmux.Pane, error) {
		return []tmux.Pane{claudePane()}, nil
	}
	// The first prompt paste lands on a UI that drops it (empty input box);
	// only after a re-paste does the agent start working.
	deps.CapturePane = func(string, int) (string, error) {
		switch {
		case len(rec.pastes) < 2:
			return "Claude Code\nready", nil
		case len(rec.pastes) < 3:
			return "Claude Code\n❯ ", nil
		default:
			return "Claude Code\nWorking... esc to interrupt", nil
		}
	}

	opts := baseOpts()
	opts.Execute = true
	report, err := dispatch.RunWithDeps(opts, deps)
	if err != nil {
		t.Fatalf("RunWithDeps: %v", err)
	}
	if got := stepStatus(t, report, "send-prompt"); got != dispatch.StatusOK {
		t.Fatalf("send-prompt status = %q, want ok", got)
	}
	if n := enterCount(rec); n != 0 {
		t.Fatalf("a lost paste should be recovered by re-pasting, not bare Enter, got %d", n)
	}
	want := []paste{{"%1", "claude", true}, {"%1", "do the thing", true}, {"%1", "do the thing", true}}
	if len(rec.pastes) != len(want) {
		t.Fatalf("pastes = %+v, want %+v", rec.pastes, want)
	}
	for i := range want {
		if rec.pastes[i] != want[i] {
			t.Fatalf("paste %d = %+v, want %+v", i, rec.pastes[i], want[i])
		}
	}
}

// TestExecuteNewSessionSucceedsWhenAgentFinishesFast covers a prompt that the
// agent accepts and completes between polls: "working" is never observed, but
// the prompt has left the input box into the transcript, so the dispatch must
// still report success rather than re-sending Enter forever.
func TestExecuteNewSessionSucceedsWhenAgentFinishesFast(t *testing.T) {
	rec, deps := baseDeps()
	deps.ListSessionPanes = func(string) ([]tmux.Pane, error) {
		return []tmux.Pane{claudePane()}, nil
	}
	deps.CapturePane = func(string, int) (string, error) {
		if len(rec.pastes) < 2 {
			return "Claude Code\nready", nil
		}
		// The prompt was submitted (now in the transcript) and the fast
		// task already finished; the live input box at the bottom is empty.
		return "Claude Code\n❯ do the thing\n\n⏺ done\n\n────\n❯ \n────\nCost: $0.01", nil
	}

	opts := baseOpts()
	opts.Execute = true
	report, err := dispatch.RunWithDeps(opts, deps)
	if err != nil {
		t.Fatalf("RunWithDeps: %v", err)
	}
	if got := stepStatus(t, report, "send-prompt"); got != dispatch.StatusOK {
		t.Fatalf("send-prompt status = %q, want ok", got)
	}
	if n := enterCount(rec); n != 0 {
		t.Fatalf("a submitted prompt should need no re-sent Enter, got %d", n)
	}
	want := []paste{{"%1", "claude", true}, {"%1", "do the thing", true}}
	if len(rec.pastes) != len(want) {
		t.Fatalf("pastes = %+v, want %+v", rec.pastes, want)
	}
}

func TestExistingSessionUnknownRuntime(t *testing.T) {
	_, deps := baseDeps()
	deps.ListLayout = func() (tmux.Layout, error) {
		return tmux.Layout{Sessions: map[string]bool{"work": true}}, nil
	}
	deps.ListSessionPanes = func(string) ([]tmux.Pane, error) {
		return []tmux.Pane{{
			Session:        "work",
			PaneID:         "%9",
			CurrentCommand: "vim",
			WindowName:     "editor",
			Active:         true,
			WindowActive:   true,
		}}, nil
	}
	deps.CapturePane = func(string, int) (string, error) {
		return "some buffer contents", nil
	}

	if _, err := dispatch.RunWithDeps(baseOpts(), deps); err == nil {
		t.Fatal("expected error for an undetermined runtime")
	}
}
