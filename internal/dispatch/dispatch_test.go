package dispatch_test

import (
	"errors"
	"os/exec"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/leolin310148/tmact/internal/dispatch"
	"github.com/leolin310148/tmact/internal/panestatus"
	"github.com/leolin310148/tmact/internal/tmux"
	"github.com/leolin310148/tmact/internal/workspacelease"
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

func TestRunRejectsUnknownModelForAgent(t *testing.T) {
	_, deps := baseDeps()
	opts := baseOpts()
	opts.Model = "sonnett"
	if _, err := dispatch.RunWithDeps(opts, deps); err == nil || !strings.Contains(err.Error(), `unsupported model "sonnett" for claude`) || !strings.Contains(err.Error(), "sonnet") {
		t.Fatalf("err = %v", err)
	}
}

func TestSupportedModelsReturnsCopy(t *testing.T) {
	models := dispatch.SupportedModels("codex")
	if len(models) == 0 {
		t.Fatal("codex model allowlist is empty")
	}
	models[0] = "changed"
	if dispatch.SupportedModels("codex")[0] == "changed" {
		t.Fatal("SupportedModels exposed its internal slice")
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

func TestExecuteRejectsWorkspaceLeasedByWorkflow(t *testing.T) {
	dir := t.TempDir()
	cmd := exec.Command("git", "init", "-q")
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init: %v: %s", err, out)
	}
	lease, err := workspacelease.Acquire(dir, "wf-test")
	if err != nil {
		t.Fatal(err)
	}
	defer lease.Release()
	_, deps := baseDeps()
	opts := baseOpts()
	opts.Dir = dir
	opts.Execute = true
	if _, err := dispatch.RunWithDeps(opts, deps); err == nil || !strings.Contains(err.Error(), "workspace unavailable") {
		t.Fatalf("error=%v", err)
	}
	opts.WorkspaceLeaseOwner = "wf-test"
	if _, err := dispatch.RunWithDeps(opts, deps); err != nil && strings.Contains(err.Error(), "workspace unavailable") {
		t.Fatalf("lease owner was rejected: %v", err)
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

func TestExecuteNewSessionRejectsUnsafeModelBeforeLaunching(t *testing.T) {
	rec, deps := baseDeps()
	opts := baseOpts()
	opts.Agent = "codex"
	opts.Model = "gpt-5.4'; echo unsafe"
	opts.Execute = true
	if _, err := dispatch.RunWithDeps(opts, deps); err == nil || !strings.Contains(err.Error(), "unsupported model") {
		t.Fatalf("err = %v", err)
	}
	if rec.newSessions != 0 || len(rec.pastes) != 0 {
		t.Fatalf("invalid model touched tmux: newSessions=%d pastes=%#v", rec.newSessions, rec.pastes)
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

func TestExistingCodexUsageLimitRequiresExactMaturedResumeCapability(t *testing.T) {
	location, err := time.LoadLocation("Asia/Taipei")
	if err != nil {
		t.Fatal(err)
	}
	originalLocal := time.Local
	time.Local = location
	t.Cleanup(func() { time.Local = originalLocal })
	resetAt := time.Date(2026, 8, 18, 9, 34, 0, 0, location)
	resumeAt := resetAt.Add(time.Minute)
	raw := "■ You've hit your usage limit. Visit https://chatgpt.com/codex/settings/usage to purchase more credits or try again at Aug 18th, 2026 9:34 AM.\n› Suggestion\n/work · Context 0% used\n"

	for _, tc := range []struct {
		name   string
		resume *dispatch.QuotaResume
		now    time.Time
	}{
		{name: "missing capability", now: resumeAt},
		{name: "before deadline", resume: &dispatch.QuotaResume{Provider: "codex", ResetAt: resetAt, ResumeAt: resumeAt}, now: resumeAt.Add(-time.Nanosecond)},
		{name: "changed reset", resume: &dispatch.QuotaResume{Provider: "codex", ResetAt: resetAt.Add(time.Minute), ResumeAt: resumeAt}, now: resumeAt},
		{name: "wrong provider", resume: &dispatch.QuotaResume{Provider: "claude", ResetAt: resetAt, ResumeAt: resumeAt}, now: resumeAt},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec, deps := baseDeps()
			deps.ListLayout = func() (tmux.Layout, error) { return tmux.Layout{Sessions: map[string]bool{"work": true}}, nil }
			deps.ListSessionPanes = func(string) ([]tmux.Pane, error) { return []tmux.Pane{codexPane()}, nil }
			deps.ProcessRuntime = func(int) panestatus.RuntimeDetection {
				return panestatus.RuntimeDetection{Runtime: panestatus.RuntimeCodex}
			}
			deps.CapturePane = func(string, int) (string, error) { return raw, nil }
			deps.Now = func() time.Time { return tc.now }
			opts := baseOpts()
			opts.Agent = "codex"
			opts.Execute = true
			opts.ReadySettle = 0
			opts.QuotaResume = tc.resume
			if _, err := dispatch.RunWithDeps(opts, deps); err == nil {
				t.Fatal("expected quota resume rejection")
			}
			if len(rec.pastes) != 0 || len(rec.keys) != 0 {
				t.Fatalf("rejected quota resume touched pane: pastes=%#v keys=%#v", rec.pastes, rec.keys)
			}
		})
	}
}

func TestExistingCodexUsageLimitResumesOnceAfterPersistedDeadline(t *testing.T) {
	rec, deps := baseDeps()
	location, err := time.LoadLocation("Asia/Taipei")
	if err != nil {
		t.Fatal(err)
	}
	originalLocal := time.Local
	time.Local = location
	t.Cleanup(func() { time.Local = originalLocal })
	resetAt := time.Date(2026, 8, 18, 9, 34, 0, 0, location)
	resumeAt := resetAt.Add(time.Minute)
	quotaRaw := "■ You've hit your usage limit. Visit https://chatgpt.com/codex/settings/usage to purchase more credits or try again at Aug 18th, 2026 9:34 AM.\n› Suggestion\n/work · Context 0% used\n"
	deps.ListLayout = func() (tmux.Layout, error) { return tmux.Layout{Sessions: map[string]bool{"work": true}}, nil }
	deps.ListSessionPanes = func(string) ([]tmux.Pane, error) { return []tmux.Pane{codexPane()}, nil }
	deps.ProcessRuntime = func(int) panestatus.RuntimeDetection {
		return panestatus.RuntimeDetection{Runtime: panestatus.RuntimeCodex}
	}
	deps.CapturePane = func(string, int) (string, error) {
		if len(rec.pastes) < 2 {
			return quotaRaw, nil
		}
		return "OpenAI Codex\nWorking... esc to interrupt", nil
	}
	deps.Now = func() time.Time { return resumeAt }
	opts := baseOpts()
	opts.Agent = "codex"
	opts.Execute = true
	opts.ReadySettle = 0
	opts.QuotaResume = &dispatch.QuotaResume{Provider: "codex", ResetAt: resetAt, ResumeAt: resumeAt}

	report, err := dispatch.RunWithDeps(opts, deps)
	if err != nil {
		t.Fatal(err)
	}
	if !report.AgentWasRunning {
		t.Fatalf("report=%#v", report)
	}
	want := []paste{{"%1", "/clear", true}, {"%1", "do the thing", true}}
	if len(rec.pastes) != len(want) {
		t.Fatalf("pastes=%#v want=%#v", rec.pastes, want)
	}
	for index := range want {
		if rec.pastes[index] != want[index] {
			t.Fatalf("paste[%d]=%#v want=%#v", index, rec.pastes[index], want[index])
		}
	}
	if len(rec.keys) != 0 {
		t.Fatalf("quota resume sent unexpected keys: %#v", rec.keys)
	}
}

func TestCodexLimitAfterSubmissionDoesNotRetryPromptOrPressEnter(t *testing.T) {
	rec, deps := baseDeps()
	deps.ListLayout = func() (tmux.Layout, error) { return tmux.Layout{Sessions: map[string]bool{"work": true}}, nil }
	deps.ListSessionPanes = func(string) ([]tmux.Pane, error) { return []tmux.Pane{codexPane()}, nil }
	deps.ProcessRuntime = func(int) panestatus.RuntimeDetection {
		return panestatus.RuntimeDetection{Runtime: panestatus.RuntimeCodex}
	}
	deps.CapturePane = func(string, int) (string, error) {
		if len(rec.pastes) < 2 {
			return "OpenAI Codex\n› Suggestion\n/work · Context 0% used\n", nil
		}
		return "› do the thing\n■ You've hit your usage limit. Visit https://chatgpt.com/codex/settings/usage to purchase more credits or try again at Aug 18th, 2026 9:34 AM.\n› Suggestion\n/work · Context 0% used\n", nil
	}
	opts := baseOpts()
	opts.Agent = "codex"
	opts.Execute = true
	opts.ReadySettle = 0
	if _, err := dispatch.RunWithDeps(opts, deps); err != nil {
		t.Fatal(err)
	}
	want := []paste{{"%1", "/clear", true}, {"%1", "do the thing", true}}
	if len(rec.pastes) != len(want) {
		t.Fatalf("Codex quota caused duplicate prompt submission: pastes=%#v", rec.pastes)
	}
	if len(rec.keys) != 0 {
		t.Fatalf("Codex quota caused unexpected key input: %#v", rec.keys)
	}
}

func TestDispatchTargetSelectsPaneInInactiveWindow(t *testing.T) {
	rec, deps := baseDeps()
	shell := tmux.Pane{Session: "work", PaneID: "%0", PanePID: 100, CurrentCommand: "zsh", WindowIndex: 0, PaneIndex: 0, Active: true, WindowActive: true}
	agent := claudePane()
	agent.PaneID = "%2"
	agent.WindowIndex = 1
	agent.WindowActive = false
	deps.ListLayout = func() (tmux.Layout, error) {
		return tmux.Layout{Sessions: map[string]bool{"work": true}}, nil
	}
	deps.ListSessionPanes = func(string) ([]tmux.Pane, error) {
		return []tmux.Pane{shell, agent}, nil
	}
	deps.ProcessRuntime = func(pid int) panestatus.RuntimeDetection {
		if pid == agent.PanePID {
			return panestatus.RuntimeDetection{Runtime: panestatus.RuntimeClaude}
		}
		return panestatus.RuntimeDetection{Runtime: panestatus.RuntimeShell}
	}
	deps.CapturePane = func(string, int) (string, error) {
		if len(rec.pastes) >= 2 {
			return "Claude Code\nWorking... esc to interrupt", nil
		}
		return "Claude Code\n❯", nil
	}

	opts := baseOpts()
	opts.Execute = true
	opts.Target = "work:1"
	report, err := dispatch.RunWithDeps(opts, deps)
	if err != nil {
		t.Fatalf("RunWithDeps: %v", err)
	}
	if report.Target != "%2" {
		t.Fatalf("target = %q, want %%2", report.Target)
	}
	for _, p := range rec.pastes {
		if p.target != "%2" {
			t.Fatalf("paste went to %q, want %%2: %+v", p.target, rec.pastes)
		}
	}
}

func TestDispatchTargetRejectsUnknownPaneAndMissingSession(t *testing.T) {
	_, deps := baseDeps()
	deps.ListLayout = func() (tmux.Layout, error) {
		return tmux.Layout{Sessions: map[string]bool{"work": true}}, nil
	}
	deps.ListSessionPanes = func(string) ([]tmux.Pane, error) {
		return []tmux.Pane{claudePane()}, nil
	}

	opts := baseOpts()
	opts.Execute = true
	opts.Target = "work:5"
	if _, err := dispatch.RunWithDeps(opts, deps); err == nil || !strings.Contains(err.Error(), "does not match any pane") {
		t.Fatalf("error = %v, want unknown-pane refusal", err)
	}

	deps.ListLayout = func() (tmux.Layout, error) {
		return tmux.Layout{Sessions: map[string]bool{}}, nil
	}
	opts.Target = "work:1"
	if _, err := dispatch.RunWithDeps(opts, deps); err == nil || !strings.Contains(err.Error(), "already exist") {
		t.Fatalf("error = %v, want missing-session refusal", err)
	}
}

func TestExistingSessionReuseWaitsForStableReadyBeforeClear(t *testing.T) {
	rec, deps := baseDeps()
	now := time.Unix(0, 0)
	var clearAt time.Time
	deps.Now = func() time.Time { return now }
	deps.Sleep = func(d time.Duration) {
		rec.sleeps = append(rec.sleeps, d)
		now = now.Add(d)
	}
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
	deps.PasteText = func(target, text string, enter bool) error {
		rec.pastes = append(rec.pastes, paste{target, text, enter})
		if text == "/clear" {
			clearAt = now
		}
		return nil
	}

	opts := baseOpts()
	opts.Execute = true
	opts.ReadySettle = 1500 * time.Millisecond
	if _, err := dispatch.RunWithDeps(opts, deps); err != nil {
		t.Fatalf("RunWithDeps: %v", err)
	}
	if elapsed := clearAt.Sub(time.Unix(0, 0)); elapsed < opts.ReadySettle {
		t.Fatalf("/clear sent after %s, want at least %s", elapsed, opts.ReadySettle)
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

func TestExistingSessionReuseRefusesTransientIdleBeforeClear(t *testing.T) {
	rec, deps := baseDeps()
	now := time.Unix(0, 0)
	deps.Now = func() time.Time { return now }
	deps.Sleep = func(d time.Duration) {
		rec.sleeps = append(rec.sleeps, d)
		now = now.Add(d)
	}
	deps.ListLayout = func() (tmux.Layout, error) {
		return tmux.Layout{Sessions: map[string]bool{"work": true}}, nil
	}
	deps.ListSessionPanes = func(string) ([]tmux.Pane, error) {
		return []tmux.Pane{claudePane()}, nil
	}
	captures := 0
	deps.CapturePane = func(string, int) (string, error) {
		captures++
		if captures == 1 {
			return "Claude Code\n❯", nil
		}
		return "Claude Code\nWorking... esc to interrupt", nil
	}

	opts := baseOpts()
	opts.Execute = true
	opts.ReadySettle = 1500 * time.Millisecond
	_, err := dispatch.RunWithDeps(opts, deps)
	if err == nil || !strings.Contains(err.Error(), "did not remain idle") {
		t.Fatalf("error = %v, want transient-idle refusal", err)
	}
	if len(rec.pastes) != 0 {
		t.Fatalf("pastes = %+v, want no /clear or prompt", rec.pastes)
	}
}

func TestExistingSessionTrustWaitsPastStaleAcceptedPrompt(t *testing.T) {
	rec, deps := baseDeps()
	dir := t.TempDir()
	pane := codexPane()
	pane.CurrentPath = dir
	deps.ListLayout = func() (tmux.Layout, error) {
		return tmux.Layout{Sessions: map[string]bool{"work": true}}, nil
	}
	deps.ListSessionPanes = func(string) ([]tmux.Pane, error) {
		return []tmux.Pane{pane}, nil
	}
	deps.ProcessRuntime = func(int) panestatus.RuntimeDetection {
		return panestatus.RuntimeDetection{Runtime: panestatus.RuntimeCodex}
	}
	captures := 0
	deps.CapturePane = func(string, int) (string, error) {
		captures++
		if captures <= 2 {
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
		t.Fatalf("RunWithDeps: %v", err)
	}
	if !report.AgentWasRunning || !report.TrustedFolder {
		t.Fatalf("report = %#v", report)
	}
	if len(rec.keys) != 1 || len(rec.keys[0].keys) != 1 || rec.keys[0].keys[0] != "Enter" {
		t.Fatalf("trust prompt keys = %#v, want exactly one Enter", rec.keys)
	}
	if captures < 3 {
		t.Fatalf("captures = %d, want stale trust prompt followed by ready state", captures)
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
example-web | Opus 4.8 (1M context) | high | ctx:13% | master
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

func TestExistingSessionReuseNoClearSkipsClear(t *testing.T) {
	rec, deps := baseDeps()
	deps.ListLayout = func() (tmux.Layout, error) {
		return tmux.Layout{Sessions: map[string]bool{"work": true}}, nil
	}
	deps.ListSessionPanes = func(string) ([]tmux.Pane, error) {
		return []tmux.Pane{claudePane()}, nil
	}
	deps.CapturePane = func(string, int) (string, error) {
		if len(rec.pastes) >= 1 {
			return "Claude Code\nWorking... esc to interrupt", nil
		}
		return "Claude Code\n❯", nil
	}

	opts := baseOpts()
	opts.NoClear = true
	report, err := dispatch.RunWithDeps(opts, deps)
	if err != nil {
		t.Fatalf("dry-run RunWithDeps: %v", err)
	}
	for _, step := range report.Steps {
		if step.Name == "clear" {
			t.Fatalf("dry-run planned a clear step: %+v", report.Steps)
		}
	}
	if len(report.Steps) != 1 || report.Steps[0].Name != "send-prompt" {
		t.Fatalf("dry-run steps = %+v", report.Steps)
	}

	opts.Execute = true
	report, err = dispatch.RunWithDeps(opts, deps)
	if err != nil {
		t.Fatalf("RunWithDeps: %v", err)
	}
	if !report.AgentWasRunning {
		t.Fatal("agent_was_running should be true")
	}
	want := []paste{{"%1", "do the thing", true}}
	if len(rec.pastes) != len(want) || rec.pastes[0] != want[0] {
		t.Fatalf("pastes = %+v, want %+v", rec.pastes, want)
	}
	if got := stepStatus(t, report, "send-prompt"); got != dispatch.StatusOK {
		t.Fatalf("send-prompt status = %q", got)
	}
}
