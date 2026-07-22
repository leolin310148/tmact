package session

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/leolin310148/tmact/internal/panestatus"
	"github.com/leolin310148/tmact/internal/tmux"
)

func TestProviderResumeCommandUsesFixedProviderForms(t *testing.T) {
	tests := []struct {
		agent string
		id    string
		want  string
	}{
		{agent: "claude", id: "01234567-89ab-cdef-0123-456789abcdef", want: "claude --resume 01234567-89ab-cdef-0123-456789abcdef"},
		{agent: "codex", id: "019c_example.1:turn-2", want: "codex resume 019c_example.1:turn-2"},
	}
	for _, test := range tests {
		got, err := ProviderResumeCommand(test.agent, test.id)
		if err != nil {
			t.Fatal(err)
		}
		if got != test.want {
			t.Fatalf("ProviderResumeCommand(%q, %q) = %q, want %q", test.agent, test.id, got, test.want)
		}
	}
	for _, test := range []struct{ agent, id string }{
		{agent: "gemini", id: "safe"},
		{agent: "codex", id: ""},
		{agent: "codex", id: "--last"},
		{agent: "claude", id: "id; touch /tmp/no"},
		{agent: "claude", id: "id\nnext"},
	} {
		if _, err := ProviderResumeCommand(test.agent, test.id); err == nil {
			t.Fatalf("ProviderResumeCommand(%q, %q) unexpectedly succeeded", test.agent, test.id)
		}
	}
}

func TestCreateCanonicalizesDirAndDefaultsToDryRun(t *testing.T) {
	realDir := t.TempDir()
	canonicalRealDir, err := filepath.EvalSymlinks(realDir)
	if err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(t.TempDir(), "repo-link")
	if err := os.Symlink(realDir, link); err != nil {
		t.Fatal(err)
	}
	created := 0
	live := false
	manager := &Manager{Deps: Dependencies{
		ListPanes: func() ([]tmux.Pane, error) {
			if !live {
				return nil, nil
			}
			return []tmux.Pane{{Session: "work", PaneID: "%4", CurrentPath: canonicalRealDir, CurrentCommand: "zsh"}}, nil
		},
		CapturePane: func(string, int) (string, error) { return "$ ", nil },
		NewSession: func(session, window, cwd string, command []string) error {
			created++
			if session != "work" || cwd != canonicalRealDir || len(command) != 0 {
				t.Fatalf("new session = (%q, %q, %q, %#v)", session, window, cwd, command)
			}
			live = true
			return nil
		},
		KillSession: func(string) error { live = false; return nil },
	}}
	preview, err := manager.Create("work", link, false)
	if err != nil {
		t.Fatal(err)
	}
	if preview.CWD != canonicalRealDir || preview.Status != StatusPlanned || preview.Executed || created != 0 {
		t.Fatalf("preview=%#v created=%d", preview, created)
	}
	result, err := manager.Create("work", link, true)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != StatusCreated || !result.Executed || result.Target != "%4" || created != 1 {
		t.Fatalf("result=%#v created=%d", result, created)
	}
}

func TestCreateReusesOnlyExactIdleShell(t *testing.T) {
	dir := t.TempDir()
	pane := tmux.Pane{Session: "work", PaneID: "%7", CurrentPath: dir, CurrentCommand: "zsh"}
	manager := &Manager{Deps: Dependencies{
		ListPanes:   func() ([]tmux.Pane, error) { return []tmux.Pane{pane}, nil },
		CapturePane: func(string, int) (string, error) { return "repo % ", nil },
	}}
	result, err := manager.Create("work", dir, true)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != StatusExisting || !result.Executed || !result.SessionExisted || result.Target != "%7" {
		t.Fatalf("result = %#v", result)
	}

	pane.CurrentCommand = "codex"
	if _, err := manager.Create("work", dir, false); err == nil || !strings.Contains(err.Error(), "different runtime") {
		t.Fatalf("agent runtime error = %v", err)
	}
	pane.CurrentCommand = "go"
	if _, err := manager.Create("work", dir, false); err == nil || !strings.Contains(err.Error(), "busy") {
		t.Fatalf("busy runtime error = %v", err)
	}
	pane.CurrentCommand = "zsh"
	manager.Deps.CapturePane = func(string, int) (string, error) { return "Waiting for approval\n", nil }
	if _, err := manager.Create("work", dir, false); err == nil || !strings.Contains(err.Error(), "waiting on a prompt") {
		t.Fatalf("prompt error = %v", err)
	}
}

func TestResumeExistingIdleShellUsesExplicitCommand(t *testing.T) {
	dir := t.TempDir()
	pane := tmux.Pane{Session: "work", PaneID: "%9", PanePID: 42, CurrentPath: dir, CurrentCommand: "zsh"}
	var target, text string
	manager := &Manager{Deps: Dependencies{
		ListPanes:   func() ([]tmux.Pane, error) { return []tmux.Pane{pane}, nil },
		CapturePane: func(string, int) (string, error) { return "repo $ ", nil },
		ProcessRuntime: func(int) panestatus.RuntimeDetection {
			return panestatus.RuntimeDetection{Runtime: panestatus.RuntimeShell}
		},
		PasteText: func(gotTarget, gotText string, enter bool) error {
			target, text = gotTarget, gotText
			if !enter {
				t.Fatal("resume did not press Enter")
			}
			return nil
		},
	}}
	preview, err := manager.Resume("work", dir, "codex", "explicit-id", false)
	if err != nil {
		t.Fatal(err)
	}
	if !preview.SessionExisted || preview.Executed || text != "" {
		t.Fatalf("preview=%#v text=%q", preview, text)
	}
	result, err := manager.Resume("work", dir, "codex", "explicit-id", true)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != StatusResumed || !result.Executed || target != "%9" || text != "codex resume explicit-id" {
		t.Fatalf("result=%#v paste=(%q,%q)", result, target, text)
	}
}

func TestResumeNewSessionRollsBackLaunchFailure(t *testing.T) {
	dir := t.TempDir()
	live := false
	killed := 0
	manager := &Manager{Deps: Dependencies{
		ListPanes: func() ([]tmux.Pane, error) {
			if !live {
				return nil, nil
			}
			return []tmux.Pane{{Session: "work", PaneID: "%3", CurrentPath: dir, CurrentCommand: "zsh"}}, nil
		},
		CapturePane: func(string, int) (string, error) { return "$ ", nil },
		NewSession:  func(string, string, string, []string) error { live = true; return nil },
		PasteText:   func(string, string, bool) error { return errors.New("paste failed") },
		KillSession: func(name string) error {
			if name != "work" {
				t.Fatalf("killed %q", name)
			}
			killed++
			live = false
			return nil
		},
	}}
	if _, err := manager.Resume("work", dir, "claude", "explicit-id", true); err == nil || !strings.Contains(err.Error(), "paste failed") {
		t.Fatalf("error = %v", err)
	}
	if killed != 1 || live {
		t.Fatalf("killed=%d live=%t", killed, live)
	}
}

func TestResumeRefusesCWDModeAndMultiplePanes(t *testing.T) {
	dir := t.TempDir()
	other := t.TempDir()
	pane := tmux.Pane{Session: "work", PaneID: "%1", CurrentPath: other, CurrentCommand: "zsh"}
	manager := &Manager{Deps: Dependencies{
		ListPanes:   func() ([]tmux.Pane, error) { return []tmux.Pane{pane}, nil },
		CapturePane: func(string, int) (string, error) { return "$ ", nil },
	}}
	if _, err := manager.Resume("work", dir, "codex", "id", false); err == nil || !strings.Contains(err.Error(), "does not exactly match") {
		t.Fatalf("cwd error = %v", err)
	}
	pane.CurrentPath, pane.InMode = dir, true
	if _, err := manager.Resume("work", dir, "codex", "id", false); err == nil || !strings.Contains(err.Error(), "tmux mode") {
		t.Fatalf("mode error = %v", err)
	}
	pane.InMode = false
	manager.Deps.ListPanes = func() ([]tmux.Pane, error) { return []tmux.Pane{pane, pane}, nil }
	if _, err := manager.Resume("work", dir, "codex", "id", false); err == nil || !strings.Contains(err.Error(), "2 panes") {
		t.Fatalf("multiple panes error = %v", err)
	}
}
