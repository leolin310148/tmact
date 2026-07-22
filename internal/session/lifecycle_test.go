package session

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/leolin310148/tmact/internal/statusd"
	"github.com/leolin310148/tmact/internal/tmux"
)

func TestCloseDryRunAndExecuteRecordExactSessionIntent(t *testing.T) {
	historyPath := filepath.Join(t.TempDir(), "closed.json")
	history := statusd.NewClosedSessionLog(historyPath, 10)
	now := time.Date(2026, 7, 22, 12, 0, 0, 0, time.UTC)
	panes := []tmux.Pane{
		{Session: "work-old", SessionID: "$1", PaneID: "%1", CurrentPath: "/old"},
		{Session: "work", SessionID: "$2", PaneID: "%2", WindowIndex: 1, CurrentPath: "/fallback", CurrentCommand: "zsh"},
		{Session: "work", SessionID: "$2", PaneID: "%3", WindowIndex: 2, PaneIndex: 1, WindowActive: true, Active: true, CurrentPath: "/repo", CurrentCommand: "node"},
	}
	var killed []string
	manager := &Manager{History: history, Deps: Dependencies{
		ListPanes: func() ([]tmux.Pane, error) { return panes, nil },
		FetchSnapshot: func() (statusd.Snapshot, error) {
			return statusd.Snapshot{
				Sessions: map[string]statusd.SessionStatus{
					"work": {Session: "work", SessionID: "$2", Runtime: "codex", ActiveTarget: "work:2.1"},
				},
				Panes: map[string]statusd.PaneStatus{
					"work:2.1": {Session: "work", SessionID: "$2", CWD: "/repo"},
				},
			}, nil
		},
		KillSession: func(name string) error {
			persisted := statusd.NewClosedSessionLog(historyPath, 10).List()
			if len(persisted) != 1 || persisted[0].Session != "work" || !persisted[0].ClosedAt.Equal(now) {
				t.Fatalf("history was not durable before kill: %#v", persisted)
			}
			killed = append(killed, name)
			return nil
		},
		Now: func() time.Time { return now },
	}}

	preview, err := manager.Close("work", false)
	if err != nil {
		t.Fatal(err)
	}
	if preview.Status != StatusPlanned || preview.Executed || preview.CWD != "/repo" || preview.Runtime != "codex" {
		t.Fatalf("preview = %#v", preview)
	}
	if len(killed) != 0 || len(history.List()) != 0 {
		t.Fatalf("dry-run mutated state: killed=%v history=%#v", killed, history.List())
	}

	result, err := manager.Close("work", true)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != StatusClosed || !result.Executed || len(killed) != 1 || killed[0] != "work" {
		t.Fatalf("result=%#v killed=%v", result, killed)
	}
	entries := history.List()
	if len(entries) != 1 || entries[0].Session != "work" || entries[0].Runtime != "codex" || entries[0].CWD != "/repo" || !entries[0].ClosedAt.Equal(now) {
		t.Fatalf("history = %#v", entries)
	}
}

func TestCloseDoesNotKillWhenHistoryPersistenceFails(t *testing.T) {
	parent := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(parent, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	killed := 0
	manager := &Manager{
		History: statusd.NewClosedSessionLog(filepath.Join(parent, "closed.json"), 10),
		Deps: Dependencies{
			ListPanes:   func() ([]tmux.Pane, error) { return []tmux.Pane{{Session: "work", CurrentPath: "/repo"}}, nil },
			KillSession: func(string) error { killed++; return nil },
		},
	}
	if _, err := manager.Close("work", true); err == nil || !strings.Contains(err.Error(), "stage reopen intent") {
		t.Fatalf("error = %v", err)
	}
	if killed != 0 {
		t.Fatalf("KillSession calls = %d, want 0", killed)
	}
}

func TestCloseKillFailureRollsBackDurableHistory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "closed.json")
	history := statusd.NewClosedSessionLog(path, 10)
	previous := statusd.ClosedSession{Session: "work", CWD: "/old", Runtime: "shell", ClosedAt: time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC)}
	if err := history.Record(previous); err != nil {
		t.Fatal(err)
	}
	manager := &Manager{
		History: history,
		Deps: Dependencies{
			ListPanes:   func() ([]tmux.Pane, error) { return []tmux.Pane{{Session: "work", CurrentPath: "/repo"}}, nil },
			KillSession: func(string) error { return errors.New("tmux kill failed") },
		},
	}
	if _, err := manager.Close("work", true); err == nil || !strings.Contains(err.Error(), "tmux kill failed") {
		t.Fatalf("error = %v", err)
	}
	if got := statusd.NewClosedSessionLog(path, 10).List(); len(got) != 1 || got[0] != previous {
		t.Fatalf("rollback was not durable: %#v", got)
	}
}

func TestCloseKillFailureReportsRollbackFailure(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "closed.json")
	manager := &Manager{
		History: statusd.NewClosedSessionLog(path, 10),
		Deps: Dependencies{
			ListPanes: func() ([]tmux.Pane, error) { return []tmux.Pane{{Session: "work", CurrentPath: "/repo"}}, nil },
			KillSession: func(string) error {
				if err := os.Remove(path); err != nil {
					t.Fatal(err)
				}
				if err := os.Mkdir(path, 0o700); err != nil {
					t.Fatal(err)
				}
				return errors.New("tmux kill failed")
			},
		},
	}
	_, err := manager.Close("work", true)
	if err == nil || !strings.Contains(err.Error(), "tmux kill failed") || !strings.Contains(err.Error(), "rollback staged reopen intent failed") {
		t.Fatalf("error = %v", err)
	}
}

func TestCloseRequiresExactExistingLocalName(t *testing.T) {
	manager := &Manager{Deps: Dependencies{ListPanes: func() ([]tmux.Pane, error) {
		return []tmux.Pane{{Session: "work-long"}}, nil
	}}}
	for _, name := range []string{"work", "work:0", "peer@work", "work.0", "work*"} {
		if _, err := manager.Close(name, false); err == nil {
			t.Fatalf("Close(%q) unexpectedly succeeded", name)
		}
	}
}

func TestReopenRestoresAllowlistedRuntimeAndRemovesHistory(t *testing.T) {
	history := statusd.NewClosedSessionLog("", 10)
	history.Record(statusd.ClosedSession{Session: "work", CWD: "/repo", Runtime: "claude", ClosedAt: time.Now()})
	live := []tmux.Pane{}
	var created []string
	var pastedTarget, pastedText string
	manager := &Manager{History: history, Deps: Dependencies{
		ListPanes: func() ([]tmux.Pane, error) { return live, nil },
		DirExists: func(path string) bool { return path == "/repo" },
		NewSession: func(session, window, cwd string, command []string) error {
			created = append(created, strings.Join([]string{session, window, cwd}, "|"))
			live = []tmux.Pane{{Session: session, PaneID: "%9", WindowActive: true, Active: true}}
			return nil
		},
		PasteText: func(target, text string, enter bool) error {
			pastedTarget, pastedText = target, text
			if !enter {
				t.Fatal("runtime launch did not press Enter")
			}
			return nil
		},
		KillSession: func(string) error { return nil },
	}}

	preview, err := manager.Reopen("work", false)
	if err != nil {
		t.Fatal(err)
	}
	if preview.Status != StatusPlanned || preview.Executed || !preview.RuntimeRestored || len(created) != 0 {
		t.Fatalf("preview=%#v created=%v", preview, created)
	}

	result, err := manager.Reopen("work", true)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != StatusReopened || !result.Executed || !result.RuntimeRestored {
		t.Fatalf("result = %#v", result)
	}
	if len(created) != 1 || created[0] != "work||/repo" || pastedTarget != "%9" || pastedText != "claude" {
		t.Fatalf("created=%v pasted=(%q,%q)", created, pastedTarget, pastedText)
	}
	if len(history.List()) != 0 {
		t.Fatalf("history retained reopened session: %#v", history.List())
	}
}

func TestReopenUnknownRuntimeFallsBackToShell(t *testing.T) {
	history := statusd.NewClosedSessionLog("", 10)
	history.Record(statusd.ClosedSession{Session: "work", CWD: "/repo", Runtime: "custom --unsafe", ClosedAt: time.Now()})
	pasteCalls := 0
	manager := &Manager{History: history, Deps: Dependencies{
		ListPanes:  func() ([]tmux.Pane, error) { return nil, nil },
		DirExists:  func(string) bool { return true },
		NewSession: func(string, string, string, []string) error { return nil },
		PasteText:  func(string, string, bool) error { pasteCalls++; return nil },
	}}
	result, err := manager.Reopen("work", true)
	if err != nil {
		t.Fatal(err)
	}
	if result.RuntimeRestored || pasteCalls != 0 {
		t.Fatalf("unsafe runtime restored: result=%#v pasteCalls=%d", result, pasteCalls)
	}
}

func TestReopenShellIntentNeedsNoRuntimeLaunch(t *testing.T) {
	history := statusd.NewClosedSessionLog("", 10)
	history.Record(statusd.ClosedSession{Session: "work", CWD: "/repo", Runtime: "shell", ClosedAt: time.Now()})
	manager := &Manager{History: history, Deps: Dependencies{
		ListPanes: func() ([]tmux.Pane, error) { return nil, nil },
		DirExists: func(string) bool { return true },
	}}
	result, err := manager.Reopen("work", false)
	if err != nil {
		t.Fatal(err)
	}
	if !result.RuntimeRestored {
		t.Fatalf("shell intent should be restored by a plain shell: %#v", result)
	}
}

func TestReopenRefusesConflictAndMissingCWD(t *testing.T) {
	history := statusd.NewClosedSessionLog("", 10)
	history.Record(statusd.ClosedSession{Session: "work", CWD: "/gone", Runtime: "shell", ClosedAt: time.Now()})
	manager := &Manager{History: history, Deps: Dependencies{
		ListPanes: func() ([]tmux.Pane, error) { return []tmux.Pane{{Session: "work"}}, nil },
		DirExists: func(string) bool { return false },
	}}
	if _, err := manager.Reopen("work", false); err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("conflict error = %v", err)
	}
	manager.Deps.ListPanes = func() ([]tmux.Pane, error) { return nil, nil }
	if _, err := manager.Reopen("work", false); err == nil || !strings.Contains(err.Error(), "no longer exists") {
		t.Fatalf("cwd error = %v", err)
	}
}

func TestReopenRuntimeFailureRollsBackAndKeepsHistory(t *testing.T) {
	history := statusd.NewClosedSessionLog("", 10)
	history.Record(statusd.ClosedSession{Session: "work", CWD: "/repo", Runtime: "codex", ClosedAt: time.Now()})
	live := false
	killed := 0
	manager := &Manager{History: history, Deps: Dependencies{
		ListPanes: func() ([]tmux.Pane, error) {
			if live {
				return []tmux.Pane{{Session: "work", PaneID: "%7"}}, nil
			}
			return nil, nil
		},
		DirExists:  func(string) bool { return true },
		NewSession: func(string, string, string, []string) error { live = true; return nil },
		PasteText:  func(string, string, bool) error { return errors.New("paste failed") },
		KillSession: func(string) error {
			killed++
			live = false
			return nil
		},
	}}
	if _, err := manager.Reopen("work", true); err == nil || !strings.Contains(err.Error(), "paste failed") {
		t.Fatalf("error = %v", err)
	}
	if killed != 1 || len(history.List()) != 1 {
		t.Fatalf("rollback/history = %d/%#v", killed, history.List())
	}
}

func TestReopenHistoryRemovalFailureCleansUpSession(t *testing.T) {
	path := filepath.Join(t.TempDir(), "closed.json")
	history := statusd.NewClosedSessionLog(path, 10)
	if err := history.Record(statusd.ClosedSession{Session: "work", CWD: "/repo", Runtime: "shell", ClosedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	killed := 0
	manager := &Manager{History: history, Deps: Dependencies{
		ListPanes: func() ([]tmux.Pane, error) { return nil, nil },
		DirExists: func(string) bool { return true },
		NewSession: func(string, string, string, []string) error {
			if err := os.Remove(path); err != nil {
				t.Fatal(err)
			}
			if err := os.Mkdir(path, 0o700); err != nil {
				t.Fatal(err)
			}
			return nil
		},
		KillSession: func(string) error { killed++; return nil },
	}}
	if _, err := manager.Reopen("work", true); err == nil || !strings.Contains(err.Error(), "remove reopened session") {
		t.Fatalf("error = %v", err)
	}
	if killed != 1 {
		t.Fatalf("cleanup KillSession calls = %d, want 1", killed)
	}
}
