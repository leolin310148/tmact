package session

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/leolin310148/tmact/internal/panestate"
	"github.com/leolin310148/tmact/internal/panestatus"
	"github.com/leolin310148/tmact/internal/tmux"
)

const (
	StatusCreated     = "created"
	StatusExisting    = "existing"
	StatusResumed     = "resumed"
	guardCaptureLines = 120
)

var providerSessionIDPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:-]*$`)

// Create previews or creates one exact local session containing an idle shell.
// An existing session is reusable only when it is the same single idle shell
// in the exact requested canonical directory.
func (m *Manager) Create(name, dir string, execute bool) (Result, error) {
	if err := validateSessionName(name); err != nil {
		return Result{}, err
	}
	canonical, err := canonicalDirectory(dir)
	if err != nil {
		return Result{}, fmt.Errorf("resolve dir: %w", err)
	}
	result := Result{Action: "create", Status: StatusPlanned, Session: name, CWD: canonical, Runtime: panestatus.RuntimeShell}
	panes, err := m.liveSessionPanes(name)
	if err != nil {
		return Result{}, err
	}
	if len(panes) != 0 {
		pane, err := m.reusableIdleShell(name, canonical, panes)
		if err != nil {
			return Result{}, err
		}
		result.Target = pane.PaneID
		result.SessionExisted = true
		if execute {
			result.Status = StatusExisting
			result.Executed = true
		}
		return result, nil
	}
	if !execute {
		return result, nil
	}
	if m.Deps.NewSession == nil {
		return Result{}, errors.New("session create is unavailable")
	}
	if err := m.Deps.NewSession(name, "", canonical, nil); err != nil {
		return Result{}, fmt.Errorf("create session %q: %w", name, err)
	}
	panes, err = m.liveSessionPanes(name)
	if err == nil {
		var pane tmux.Pane
		pane, err = m.reusableIdleShell(name, canonical, panes)
		result.Target = pane.PaneID
	}
	if err != nil {
		return Result{}, m.createdSessionFailure(name, fmt.Errorf("verify new idle-shell session: %w", err))
	}
	result.Status = StatusCreated
	result.Executed = true
	return result, nil
}

// Resume previews or launches one explicitly named Claude/Codex provider
// session in an exact local tmux session. The provider session id is never
// discovered from pane text or other local state.
func (m *Manager) Resume(name, dir, agent, sessionID string, execute bool) (Result, error) {
	if err := validateSessionName(name); err != nil {
		return Result{}, err
	}
	command, err := ProviderResumeCommand(agent, sessionID)
	if err != nil {
		return Result{}, err
	}
	canonical, err := canonicalDirectory(dir)
	if err != nil {
		return Result{}, fmt.Errorf("resolve dir: %w", err)
	}
	result := Result{
		Action: "resume", Status: StatusPlanned, Session: name, CWD: canonical,
		Runtime: agent, SessionID: sessionID,
	}
	panes, err := m.liveSessionPanes(name)
	if err != nil {
		return Result{}, err
	}
	var pane tmux.Pane
	if len(panes) != 0 {
		pane, err = m.reusableIdleShell(name, canonical, panes)
		if err != nil {
			return Result{}, err
		}
		result.SessionExisted = true
		result.Target = pane.PaneID
	}
	if !execute {
		return result, nil
	}

	created := false
	if len(panes) == 0 {
		if m.Deps.NewSession == nil {
			return Result{}, errors.New("session resume is unavailable")
		}
		if err := m.Deps.NewSession(name, "", canonical, nil); err != nil {
			return Result{}, fmt.Errorf("create session %q for resume: %w", name, err)
		}
		created = true
		panes, err = m.liveSessionPanes(name)
		if err == nil {
			pane, err = m.reusableIdleShell(name, canonical, panes)
		}
		if err != nil {
			return Result{}, m.resumeFailure(name, created, fmt.Errorf("inspect new session before resume: %w", err))
		}
		result.Target = pane.PaneID
	}
	if m.Deps.PasteText == nil {
		return Result{}, m.resumeFailure(name, created, errors.New("session resume launch is unavailable"))
	}
	if err := m.Deps.PasteText(pane.PaneID, command, true); err != nil {
		return Result{}, m.resumeFailure(name, created, fmt.Errorf("resume %s session %q: %w", agent, sessionID, err))
	}
	result.Status = StatusResumed
	result.Executed = true
	return result, nil
}

// ProviderResumeCommand constructs the fixed provider-specific launcher used
// by Resume. Session ids are deliberately restricted to one non-option shell
// word so they cannot add flags or shell syntax.
func ProviderResumeCommand(agent, sessionID string) (string, error) {
	if agent != panestatus.RuntimeClaude && agent != panestatus.RuntimeCodex {
		return "", fmt.Errorf("unsupported resume agent %q; want claude or codex", agent)
	}
	if !providerSessionIDPattern.MatchString(sessionID) {
		return "", errors.New("session id must be a non-option identifier containing only letters, digits, '.', '_', ':', or '-'")
	}
	if agent == panestatus.RuntimeClaude {
		return "claude --resume " + sessionID, nil
	}
	return "codex resume " + sessionID, nil
}

func (m *Manager) reusableIdleShell(name, dir string, panes []tmux.Pane) (tmux.Pane, error) {
	if len(panes) != 1 {
		return tmux.Pane{}, fmt.Errorf("session %q has %d panes; refusing to choose a resume target", name, len(panes))
	}
	pane := panes[0]
	if pane.PaneID == "" {
		return tmux.Pane{}, fmt.Errorf("session %q pane has no exact pane id", name)
	}
	if pane.InMode {
		return tmux.Pane{}, fmt.Errorf("session %q pane is in tmux mode; refusing to send input", name)
	}
	paneDir, err := canonicalDirectory(pane.CurrentPath)
	if err != nil {
		return tmux.Pane{}, fmt.Errorf("resolve session %q pane cwd %q: %w", name, pane.CurrentPath, err)
	}
	if paneDir != dir {
		return tmux.Pane{}, fmt.Errorf("session %q pane cwd %s does not exactly match requested directory %s", name, paneDir, dir)
	}
	if m.Deps.CapturePane == nil {
		return tmux.Pane{}, errors.New("pane capture is unavailable")
	}
	raw, err := m.Deps.CapturePane(pane.PaneID, guardCaptureLines)
	if err != nil {
		return tmux.Pane{}, fmt.Errorf("capture session %q pane: %w", name, err)
	}
	classified := panestate.Classify(raw)
	if classified.Asking {
		kind := "interactive prompt"
		if classified.InteractivePrompt != nil && classified.InteractivePrompt.Type != "" {
			kind = classified.InteractivePrompt.Type
		}
		return tmux.Pane{}, fmt.Errorf("session %q is waiting on a prompt (%s); resolve it first", name, kind)
	}
	runtime := panestatus.RuntimeUnknown
	if m.Deps.ProcessRuntime != nil {
		detected := m.Deps.ProcessRuntime(pane.PanePID)
		runtime = detected.Runtime
	}
	if runtime == "" || runtime == panestatus.RuntimeUnknown {
		runtime = panestatus.ClassifyRuntime(pane, raw).Runtime
	}
	if !isKnownShellCommand(pane.CurrentCommand) && runtime == panestatus.RuntimeShell {
		runtime = panestatus.RuntimeUnknown
	}
	if runtime != panestatus.RuntimeShell {
		if runtime == panestatus.RuntimeClaude || runtime == panestatus.RuntimeCodex || runtime == panestatus.RuntimeGemini {
			return tmux.Pane{}, fmt.Errorf("session %q is already running a different runtime (%s); expected an idle shell", name, runtime)
		}
		return tmux.Pane{}, fmt.Errorf("session %q is busy with runtime %q; expected an idle shell", name, runtime)
	}
	if classified.State == panestate.StateWorking || classified.State == panestate.StateDraftInput || classified.State == panestate.StateBlocked {
		return tmux.Pane{}, fmt.Errorf("session %q shell state is %s; expected an idle shell", name, classified.State)
	}
	return pane, nil
}

func isKnownShellCommand(command string) bool {
	switch strings.ToLower(filepath.Base(strings.TrimSpace(command))) {
	case "bash", "fish", "ksh", "sh", "tcsh", "zsh":
		return true
	default:
		return false
	}
}

func (m *Manager) resumeFailure(name string, created bool, cause error) error {
	if !created {
		return cause
	}
	return m.createdSessionFailure(name, cause)
}

func (m *Manager) createdSessionFailure(name string, cause error) error {
	if m.Deps.KillSession == nil {
		return fmt.Errorf("%v (cleanup of newly created session %q is unavailable)", cause, name)
	}
	if err := m.Deps.KillSession(name); err != nil {
		return fmt.Errorf("%v (cleanup of newly created session %q failed: %v)", cause, name, err)
	}
	return cause
}

func canonicalDirectory(path string) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", errors.New("directory is required")
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(abs)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", fmt.Errorf("%s is not a directory", abs)
	}
	resolved, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return "", err
	}
	return filepath.Clean(resolved), nil
}
