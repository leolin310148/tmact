package dispatch_test

import (
	"errors"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

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
	sleeps      []time.Duration
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
		Sleep: func(d time.Duration) {
			rec.sleeps = append(rec.sleeps, d)
		},
		Now: func() time.Time { return time.Unix(0, 0) },
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

func TestDryRunPromptDetailTruncatesLongUnicodePromptAtRuneBoundary(t *testing.T) {
	_, deps := baseDeps()
	opts := baseOpts()
	opts.Execute = false
	opts.Prompt = "a" + strings.Repeat("請", 80)

	report, err := dispatch.RunWithDeps(opts, deps)
	if err != nil {
		t.Fatal(err)
	}
	detail := stepDetail(t, report, "send-prompt")
	if !utf8.ValidString(detail) {
		t.Fatalf("send-prompt detail is invalid UTF-8: %q", detail)
	}
	if !strings.HasSuffix(detail, "...") {
		t.Fatalf("send-prompt detail = %q, want ellipsis suffix", detail)
	}
	if utf8.RuneCountInString(detail) != 60 {
		t.Fatalf("send-prompt detail rune count = %d, want 60", utf8.RuneCountInString(detail))
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

func codexPane() tmux.Pane {
	pane := claudePane()
	pane.CurrentCommand = "codex"
	pane.WindowName = "codex"
	return pane
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

func stepDetail(t *testing.T, report dispatch.Report, name string) string {
	t.Helper()
	for _, step := range report.Steps {
		if step.Name == name {
			return step.Detail
		}
	}
	t.Fatalf("step %q not found in %+v", name, report.Steps)
	return ""
}

func TestRunRejectsUnsupportedAgent(t *testing.T) {
	_, deps := baseDeps()
	opts := baseOpts()
	opts.Agent = "copilot"
	if _, err := dispatch.RunWithDeps(opts, deps); err == nil {
		t.Fatal("expected error for unsupported agent")
	}
}

func TestRunRejectsTrustFolderForUnsupportedAgent(t *testing.T) {
	_, deps := baseDeps()
	opts := baseOpts()
	opts.Agent = "gemini"
	opts.TrustFolder = true
	if _, err := dispatch.RunWithDeps(opts, deps); err == nil || !strings.Contains(err.Error(), "only supports claude or codex") {
		t.Fatalf("err = %v", err)
	}
}

func TestRunRejectsModelForUnsupportedAgent(t *testing.T) {
	_, deps := baseDeps()
	opts := baseOpts()
	opts.Agent = "gemini"
	opts.Model = "gemini-2.5-pro"
	if _, err := dispatch.RunWithDeps(opts, deps); err == nil || !strings.Contains(err.Error(), "only supports claude or codex") {
		t.Fatalf("err = %v", err)
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

func TestDryRunNewClaudeSessionWithModel(t *testing.T) {
	_, deps := baseDeps()
	opts := baseOpts()
	opts.Model = "sonnet"
	report, err := dispatch.RunWithDeps(opts, deps)
	if err != nil {
		t.Fatal(err)
	}
	if report.Model != "sonnet" {
		t.Fatalf("model = %q", report.Model)
	}
	if detail := stepDetail(t, report, "launch-agent"); !strings.Contains(detail, "claude --model 'sonnet'") {
		t.Fatalf("launch detail = %q", detail)
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

func TestExecuteNewSessionWithModelShellQuotesModel(t *testing.T) {
	rec, deps := baseDeps()
	deps.ListSessionPanes = func(string) ([]tmux.Pane, error) {
		return []tmux.Pane{codexPane()}, nil
	}
	deps.CapturePane = func(string, int) (string, error) {
		if len(rec.pastes) < 2 {
			return "OpenAI Codex\n› ", nil
		}
		return "OpenAI Codex\nWorking... esc to interrupt", nil
	}

	opts := baseOpts()
	opts.Agent = "codex"
	opts.Model = "gpt-5.4'; echo unsafe"
	opts.Execute = true
	report, err := dispatch.RunWithDeps(opts, deps)
	if err != nil {
		t.Fatal(err)
	}
	wantCommand := "codex --model 'gpt-5.4'\\''; echo unsafe'"
	if report.Model != opts.Model {
		t.Fatalf("model = %q, want %q", report.Model, opts.Model)
	}
	if len(rec.pastes) < 1 || rec.pastes[0].text != wantCommand {
		t.Fatalf("launch paste = %#v, want %q", rec.pastes, wantCommand)
	}
	if detail := stepDetail(t, report, "launch-agent"); !strings.Contains(detail, "codex --model") {
		t.Fatalf("launch detail = %q", detail)
	}
}

func TestExecuteNewSessionAutoTrustsExactCodexDirectory(t *testing.T) {
	rec, deps := baseDeps()
	dir := t.TempDir()
	pane := codexPane()
	pane.CurrentPath = dir
	deps.ListSessionPanes = func(string) ([]tmux.Pane, error) { return []tmux.Pane{pane}, nil }
	deps.ProcessRuntime = func(int) panestatus.RuntimeDetection {
		return panestatus.RuntimeDetection{Runtime: panestatus.RuntimeCodex}
	}
	deps.CapturePane = func(string, int) (string, error) {
		if len(rec.keys) == 0 {
			return "OpenAI Codex\nDo you trust the contents of this directory?\n› 1. Yes, continue\n  2. No, quit\n", nil
		}
		if len(rec.pastes) < 2 {
			return "OpenAI Codex\n› ", nil
		}
		return "OpenAI Codex\nWorking... esc to interrupt", nil
	}
	opts := baseOpts()
	opts.Dir = dir
	opts.Agent = "codex"
	opts.Execute = true
	opts.TrustFolder = true
	opts.ReadySettle = 0

	report, err := dispatch.RunWithDeps(opts, deps)
	if err != nil {
		t.Fatal(err)
	}
	if !report.TrustFolder || !report.TrustedFolder {
		t.Fatalf("report = %#v", report)
	}
	if len(rec.keys) != 1 || len(rec.keys[0].keys) != 1 || rec.keys[0].keys[0] != "Enter" {
		t.Fatalf("keys = %#v", rec.keys)
	}
}

func TestExecuteNewSessionStillRefusesTrustPromptWithoutOptIn(t *testing.T) {
	_, deps := baseDeps()
	dir := t.TempDir()
	pane := codexPane()
	pane.CurrentPath = dir
	deps.ListSessionPanes = func(string) ([]tmux.Pane, error) { return []tmux.Pane{pane}, nil }
	deps.ProcessRuntime = func(int) panestatus.RuntimeDetection {
		return panestatus.RuntimeDetection{Runtime: panestatus.RuntimeCodex}
	}
	deps.CapturePane = func(string, int) (string, error) {
		return "OpenAI Codex\nDo you trust the contents of this directory?\n› 1. Yes, continue\n  2. No, quit\n", nil
	}
	opts := baseOpts()
	opts.Dir = dir
	opts.Agent = "codex"
	opts.Execute = true

	_, err := dispatch.RunWithDeps(opts, deps)
	if err == nil || !strings.Contains(err.Error(), "refusing to auto-confirm") {
		t.Fatalf("err = %v", err)
	}
}

func TestExecuteNewSessionDebouncesCodexReadyBeforePrompt(t *testing.T) {
	rec, deps := baseDeps()
	now := time.Unix(0, 0)
	var promptPastedAt time.Time
	deps.Now = func() time.Time { return now }
	deps.Sleep = func(d time.Duration) {
		rec.sleeps = append(rec.sleeps, d)
		now = now.Add(d)
	}
	deps.ListSessionPanes = func(string) ([]tmux.Pane, error) {
		return []tmux.Pane{codexPane()}, nil
	}
	deps.PasteText = func(target, text string, enter bool) error {
		rec.pastes = append(rec.pastes, paste{target, text, enter})
		if text == "do the thing" && promptPastedAt.IsZero() {
			promptPastedAt = now
		}
		return nil
	}
	waitCaptures := 0
	deps.CapturePane = func(string, int) (string, error) {
		if len(rec.pastes) < 2 {
			waitCaptures++
			return "OpenAI Codex\n› ", nil
		}
		return "OpenAI Codex\nWorking... esc to interrupt", nil
	}

	opts := baseOpts()
	opts.Agent = "codex"
	opts.Execute = true
	opts.ReadySettle = 2 * time.Second
	report, err := dispatch.RunWithDeps(opts, deps)
	if err != nil {
		t.Fatalf("RunWithDeps: %v", err)
	}
	if got := stepStatus(t, report, "send-prompt"); got != dispatch.StatusOK {
		t.Fatalf("send-prompt status = %q, want ok", got)
	}
	if waitCaptures < 3 {
		t.Fatalf("wait-ready captures = %d, want at least 3", waitCaptures)
	}
	if got := promptPastedAt.Sub(time.Unix(0, 0)); got < 2*time.Second {
		t.Fatalf("prompt pasted after %s, want at least 2s", got)
	}
	want := []paste{{"%1", "codex", true}, {"%1", "do the thing", true}}
	if len(rec.pastes) != len(want) {
		t.Fatalf("pastes = %+v, want %+v", rec.pastes, want)
	}
	for i := range want {
		if rec.pastes[i] != want[i] {
			t.Fatalf("paste %d = %+v, want %+v", i, rec.pastes[i], want[i])
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
		return "Claude Code\n❯", nil
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

func TestExistingSessionUnknownStateRefusesClear(t *testing.T) {
	rec, deps := baseDeps()
	deps.ListLayout = func() (tmux.Layout, error) {
		return tmux.Layout{Sessions: map[string]bool{"work": true}}, nil
	}
	deps.ListSessionPanes = func(string) ([]tmux.Pane, error) {
		return []tmux.Pane{claudePane()}, nil
	}
	deps.CapturePane = func(string, int) (string, error) {
		return "Claude Code\nstatus unavailable", nil
	}

	opts := baseOpts()
	opts.Execute = true
	if _, err := dispatch.RunWithDeps(opts, deps); err == nil || !strings.Contains(err.Error(), "explicitly input-ready") {
		t.Fatalf("error=%v", err)
	}
	if len(rec.pastes) != 0 {
		t.Fatalf("unknown pane received input: %#v", rec.pastes)
	}
}

func TestExistingSessionReusesDimSuggestionButRejectsOperatorDraft(t *testing.T) {
	tests := []struct {
		name    string
		pane    tmux.Pane
		agent   string
		plain   string
		ansi    string
		wantErr string
	}{
		{
			name:  "claude suggestion",
			pane:  claudePane(),
			agent: "claude",
			plain: "old working output\n❯ source ~/.zsh_aliases\n⏵⏵ auto mode on (shift+tab to cycle) · ← for agents\n",
			ansi:  "old working output\n\x1b[39m❯ \x1b[2msource ~/.zsh_aliases\x1b[0m\n⏵⏵ auto mode on (shift+tab to cycle) · ← for agents\n",
		},
		{
			name:    "claude draft",
			pane:    claudePane(),
			agent:   "claude",
			plain:   "❯ source ~/.zsh_aliases\n",
			ansi:    "\x1b[38;5;239m\x1b[48;5;237m❯ \x1b[38;5;231msource ~/.zsh_aliases\x1b[0m\n",
			wantErr: "draft_input",
		},
		{
			name:  "codex suggestion",
			pane:  codexPane(),
			agent: "codex",
			plain: "› Write tests for @filename\n~/repo · main · Context 30% used · 353K window\n",
			ansi:  "\x1b[0;1m›\x1b[0m \x1b[2mWrite tests for @filename\x1b[0m\n~/repo · main · Context 30% used · 353K window\n",
		},
		{
			name:    "codex draft",
			pane:    codexPane(),
			agent:   "codex",
			plain:   "› Write tests for store.go\n",
			ansi:    "\x1b[0;1m›\x1b[0m \x1b[38;2;205;214;244mWrite tests for store.go\x1b[0m\n",
			wantErr: "draft_input",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec, deps := baseDeps()
			deps.ListLayout = func() (tmux.Layout, error) {
				return tmux.Layout{Sessions: map[string]bool{"work": true}}, nil
			}
			deps.ListSessionPanes = func(string) ([]tmux.Pane, error) { return []tmux.Pane{tt.pane}, nil }
			deps.CapturePane = func(string, int) (string, error) { return tt.plain, nil }
			deps.CapturePaneANSI = func(string, int) (string, error) { return tt.ansi, nil }

			opts := baseOpts()
			opts.Agent = tt.agent
			report, err := dispatch.RunWithDeps(opts, deps)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("error=%v", err)
				}
				if len(rec.pastes) != 0 {
					t.Fatalf("draft pane received input: %#v", rec.pastes)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if !report.AgentWasRunning || stepStatus(t, report, "clear") != dispatch.StatusPlanned {
				t.Fatalf("report=%#v", report)
			}
		})
	}
}

func TestExistingSessionReuseClaudePromptAboveIdleFooter(t *testing.T) {
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
		return `
I am working on the synthesis now.
用戶目前無待辦。
❯
guru-scp-web | Opus 4.8 (1M context) | high | ctx:13% | master
⏵⏵ auto mode on (shift+tab to cycle) · ← for agents
`, nil
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

func TestExistingRunningAgentRejectsModel(t *testing.T) {
	_, deps := baseDeps()
	deps.ListLayout = func() (tmux.Layout, error) {
		return tmux.Layout{Sessions: map[string]bool{"work": true}}, nil
	}
	deps.ListSessionPanes = func(string) ([]tmux.Pane, error) {
		return []tmux.Pane{claudePane()}, nil
	}
	deps.CapturePane = func(string, int) (string, error) {
		return "Claude Code\nready for input", nil
	}
	opts := baseOpts()
	opts.Model = "sonnet"
	if _, err := dispatch.RunWithDeps(opts, deps); err == nil || !strings.Contains(err.Error(), "--model only applies when launching") {
		t.Fatalf("err = %v", err)
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
