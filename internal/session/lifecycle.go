// Package session implements the recoverable local tmux-session lifecycle.
// It deliberately operates on exact session names and keeps all pane text out
// of the persisted history.
package session

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/leolin310148/tmact/internal/panestatus"
	"github.com/leolin310148/tmact/internal/statusd"
	"github.com/leolin310148/tmact/internal/tmux"
)

const (
	StatusPlanned  = "planned"
	StatusClosed   = "closed"
	StatusReopened = "reopened"
)

// Result describes a close or reopen plan/result without exposing pane text.
type Result struct {
	Action          string `json:"action"`
	Status          string `json:"status"`
	Session         string `json:"session"`
	Target          string `json:"target,omitempty"`
	CWD             string `json:"cwd,omitempty"`
	Runtime         string `json:"runtime,omitempty"`
	SessionID       string `json:"session_id,omitempty"`
	SessionExisted  bool   `json:"session_existed,omitempty"`
	RuntimeRestored bool   `json:"runtime_restored"`
	Executed        bool   `json:"executed"`
}

// Dependencies isolates tmux and filesystem effects for service tests.
type Dependencies struct {
	ListPanes      func() ([]tmux.Pane, error)
	FetchSnapshot  func() (statusd.Snapshot, error)
	KillSession    func(string) error
	NewSession     func(session, window, cwd string, command []string) error
	PasteText      func(target, text string, enter bool) error
	CapturePane    func(target string, lines int) (string, error)
	ProcessRuntime func(pid int) panestatus.RuntimeDetection
	DirExists      func(string) bool
	Now            func() time.Time
}

// Manager shares the same ClosedSessionLog used by statusd and the web UI.
type Manager struct {
	History *statusd.ClosedSessionLog
	Deps    Dependencies
}

// NewManager returns a live local-session manager.
func NewManager(history *statusd.ClosedSessionLog) *Manager {
	return &Manager{
		History: history,
		Deps: Dependencies{
			ListPanes: tmux.ListAllPanes,
			FetchSnapshot: func() (statusd.Snapshot, error) {
				return statusd.FetchSnapshot(statusd.DefaultSocketPath)
			},
			KillSession:    tmux.KillSession,
			NewSession:     tmux.NewSession,
			PasteText:      tmux.PasteText,
			CapturePane:    tmux.CapturePane,
			ProcessRuntime: panestatus.DetectChildProcessRuntime,
			DirExists: func(path string) bool {
				info, err := os.Stat(path)
				return err == nil && info.IsDir()
			},
			Now: time.Now,
		},
	}
}

// Close records the recoverable intent for one exact live session and closes
// it only when execute is true.
func (m *Manager) Close(name string, execute bool) (Result, error) {
	if err := validateSessionName(name); err != nil {
		return Result{}, err
	}
	panes, err := m.liveSessionPanes(name)
	if err != nil {
		return Result{}, err
	}
	if len(panes) == 0 {
		return Result{}, fmt.Errorf("session %q does not exist", name)
	}
	entry := m.closedEntry(name, panes)
	result := Result{
		Action:  "close",
		Status:  StatusPlanned,
		Session: entry.Session,
		CWD:     entry.CWD,
		Runtime: entry.Runtime,
	}
	if !execute {
		return result, nil
	}
	if m.Deps.KillSession == nil {
		return Result{}, errors.New("session close is unavailable")
	}
	if m.History == nil {
		return Result{}, errors.New("session close requires durable closed-session history")
	}
	entry.ClosedAt = m.now()()
	rollback, err := m.History.StageDurable(entry)
	if err != nil {
		return Result{}, fmt.Errorf("stage reopen intent for session %q: %w", name, err)
	}
	if err := m.Deps.KillSession(name); err != nil {
		rollbackErr := rollback()
		if rollbackErr != nil {
			return Result{}, fmt.Errorf("close session %q: %v (rollback staged reopen intent failed: %v)", name, err, rollbackErr)
		}
		return Result{}, fmt.Errorf("close session %q: %w", name, err)
	}
	result.Status = StatusClosed
	result.Executed = true
	return result, nil
}

// Closed lists the shared recently-closed history, newest first.
func (m *Manager) Closed() []statusd.ClosedSession {
	if m.History == nil {
		return []statusd.ClosedSession{}
	}
	entries := m.History.List()
	if entries == nil {
		return []statusd.ClosedSession{}
	}
	return entries
}

// Reopen recreates one recorded session at its saved cwd. Known interactive
// agent runtimes are relaunched through the new shell using an exact fixed
// allowlist; unknown/custom runtimes safely reopen as a plain shell.
func (m *Manager) Reopen(name string, execute bool) (Result, error) {
	if err := validateSessionName(name); err != nil {
		return Result{}, err
	}
	entry, ok := m.findClosed(name)
	if !ok {
		return Result{}, fmt.Errorf("session %q is not in closed history", name)
	}
	panes, err := m.liveSessionPanes(name)
	if err != nil {
		return Result{}, err
	}
	if len(panes) != 0 {
		return Result{}, fmt.Errorf("session %q already exists", name)
	}
	if entry.CWD != "" {
		if !filepath.IsAbs(entry.CWD) {
			return Result{}, fmt.Errorf("recorded cwd for session %q must be absolute", name)
		}
		if m.Deps.DirExists == nil || !m.Deps.DirExists(entry.CWD) {
			return Result{}, fmt.Errorf("recorded cwd for session %q no longer exists: %s", name, entry.CWD)
		}
	}
	runtime, launchRuntime := restorableRuntime(entry.Runtime)
	runtimeRestored := launchRuntime || isShellRuntime(entry.Runtime)
	result := Result{
		Action:          "reopen",
		Status:          StatusPlanned,
		Session:         entry.Session,
		CWD:             entry.CWD,
		Runtime:         entry.Runtime,
		RuntimeRestored: runtimeRestored,
	}
	if !execute {
		return result, nil
	}
	if m.Deps.NewSession == nil {
		return Result{}, errors.New("session reopen is unavailable")
	}
	if err := m.Deps.NewSession(name, "", entry.CWD, nil); err != nil {
		return Result{}, fmt.Errorf("reopen session %q: %w", name, err)
	}
	if launchRuntime {
		target, err := m.reopenedPane(name)
		if err == nil && m.Deps.PasteText == nil {
			err = errors.New("runtime launch is unavailable")
		}
		if err == nil {
			err = m.Deps.PasteText(target, runtime, true)
		}
		if err != nil {
			if cleanupErr := m.cleanupReopen(name); cleanupErr != nil {
				return Result{}, fmt.Errorf("restore runtime for session %q: %v (cleanup failed: %v)", name, err, cleanupErr)
			}
			return Result{}, fmt.Errorf("restore runtime for session %q: %w", name, err)
		}
	}
	if m.History != nil {
		if _, err := m.History.Remove(name); err != nil {
			if cleanupErr := m.cleanupReopen(name); cleanupErr != nil {
				return Result{}, fmt.Errorf("remove reopened session %q from history: %v (cleanup failed: %v)", name, err, cleanupErr)
			}
			return Result{}, fmt.Errorf("remove reopened session %q from history: %w", name, err)
		}
	}
	result.Status = StatusReopened
	result.Executed = true
	return result, nil
}

func (m *Manager) liveSessionPanes(name string) ([]tmux.Pane, error) {
	if m.Deps.ListPanes == nil {
		return nil, errors.New("session listing is unavailable")
	}
	panes, err := m.Deps.ListPanes()
	if err != nil {
		if tmux.IsTargetGoneError(err) {
			return []tmux.Pane{}, nil
		}
		return nil, fmt.Errorf("list tmux sessions: %w", err)
	}
	matched := make([]tmux.Pane, 0, len(panes))
	for _, pane := range panes {
		if pane.Session == name {
			matched = append(matched, pane)
		}
	}
	sort.Slice(matched, func(i, j int) bool {
		if matched[i].WindowIndex != matched[j].WindowIndex {
			return matched[i].WindowIndex < matched[j].WindowIndex
		}
		return matched[i].PaneIndex < matched[j].PaneIndex
	})
	return matched, nil
}

func (m *Manager) closedEntry(name string, panes []tmux.Pane) statusd.ClosedSession {
	pane := preferredPane(panes)
	entry := statusd.ClosedSession{Session: name, CWD: pane.CurrentPath, Runtime: runtimeFromCommand(pane.CurrentCommand)}
	if m.Deps.FetchSnapshot == nil {
		return entry
	}
	snapshot, err := m.Deps.FetchSnapshot()
	if err != nil {
		return entry
	}
	status, ok := snapshot.Sessions[name]
	if !ok || status.Peer != "" || status.SessionID == "" || status.SessionID != pane.SessionID {
		return entry
	}
	if status.Runtime != "" {
		entry.Runtime = status.Runtime
	}
	if snapshotPane, ok := snapshot.Panes[status.ActiveTarget]; ok && snapshotPane.Peer == "" && snapshotPane.SessionID == pane.SessionID && snapshotPane.CWD != "" {
		entry.CWD = snapshotPane.CWD
	}
	return entry
}

func (m *Manager) findClosed(name string) (statusd.ClosedSession, bool) {
	for _, entry := range m.Closed() {
		if entry.Peer == "" && entry.Session == name {
			return entry, true
		}
	}
	return statusd.ClosedSession{}, false
}

func (m *Manager) reopenedPane(name string) (string, error) {
	panes, err := m.liveSessionPanes(name)
	if err != nil {
		return "", err
	}
	if len(panes) == 0 {
		return "", errors.New("new session has no pane")
	}
	pane := preferredPane(panes)
	if pane.PaneID == "" {
		return "", errors.New("new session pane has no exact pane id")
	}
	return pane.PaneID, nil
}

func (m *Manager) cleanupReopen(name string) error {
	if m.Deps.KillSession == nil {
		return errors.New("session cleanup is unavailable")
	}
	return m.Deps.KillSession(name)
}

func (m *Manager) now() func() time.Time {
	if m.Deps.Now != nil {
		return m.Deps.Now
	}
	return time.Now
}

func preferredPane(panes []tmux.Pane) tmux.Pane {
	for _, pane := range panes {
		if pane.WindowActive && pane.Active {
			return pane
		}
	}
	for _, pane := range panes {
		if pane.Active {
			return pane
		}
	}
	return panes[0]
}

func runtimeFromCommand(command string) string {
	runtime, ok := restorableRuntime(filepath.Base(strings.TrimSpace(command)))
	if ok {
		return runtime
	}
	return "shell"
}

func restorableRuntime(runtime string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(runtime)) {
	case "claude":
		return "claude", true
	case "codex":
		return "codex", true
	case "gemini":
		return "gemini", true
	default:
		return "", false
	}
}

func isShellRuntime(runtime string) bool {
	runtime = strings.ToLower(strings.TrimSpace(runtime))
	return runtime == "" || runtime == "shell"
}

func validateSessionName(name string) error {
	if name == "" {
		return errors.New("session name is required")
	}
	if strings.TrimSpace(name) != name {
		return errors.New("session name must not have leading or trailing whitespace")
	}
	if strings.ContainsAny(name, ":.@*?[]") {
		return fmt.Errorf("session name %q is not an exact local session name", name)
	}
	return nil
}
