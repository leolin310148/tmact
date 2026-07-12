package tmux

import (
	"fmt"
	"strconv"
	"strings"
)

// SessionStatePane is the tmux state needed to rebuild one pane. It omits
// commands and process details intentionally: session restore must never
// restart unattended programs.
type SessionStatePane struct {
	Session      string
	WindowIndex  int
	WindowName   string
	WindowLayout string
	WindowWidth  int
	WindowHeight int
	WindowActive bool
	PaneIndex    int
	CurrentPath  string
	Active       bool
}

const sessionStateFormat = "#{session_name}\t#{window_index}\t#{window_name}\t#{window_layout}\t#{window_width}\t#{window_height}\t#{window_active}\t#{pane_index}\t#{pane_current_path}\t#{pane_active}"

// ListSessionState returns every local tmux pane with its window layout. A
// missing tmux server, or a running server with no sessions at all, is a
// successful empty result; all other failures remain errors so callers cannot
// confuse capture failure with zero sessions.
func ListSessionState() ([]SessionStatePane, error) {
	output, err := outputTmux("list-panes", "-a", "-F", sessionStateFormat)
	if err != nil {
		if isNoServerError(err) || isEmptyServerError(err) {
			return []SessionStatePane{}, nil
		}
		return nil, err
	}
	return ParseSessionState(output)
}

func ParseSessionState(output string) ([]SessionStatePane, error) {
	var panes []SessionStatePane
	for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "\t", 10)
		if len(parts) != 10 {
			return nil, fmt.Errorf("invalid tmux session-state row %q", line)
		}
		windowIndex, err := strconv.Atoi(parts[1])
		if err != nil {
			return nil, fmt.Errorf("invalid window index %q: %w", parts[1], err)
		}
		windowWidth, err := strconv.Atoi(parts[4])
		if err != nil {
			return nil, fmt.Errorf("invalid window width %q: %w", parts[4], err)
		}
		windowHeight, err := strconv.Atoi(parts[5])
		if err != nil {
			return nil, fmt.Errorf("invalid window height %q: %w", parts[5], err)
		}
		paneIndex, err := strconv.Atoi(parts[7])
		if err != nil {
			return nil, fmt.Errorf("invalid pane index %q: %w", parts[7], err)
		}
		panes = append(panes, SessionStatePane{
			Session:      parts[0],
			WindowIndex:  windowIndex,
			WindowName:   parts[2],
			WindowLayout: parts[3],
			WindowWidth:  windowWidth,
			WindowHeight: windowHeight,
			WindowActive: parts[6] == "1",
			PaneIndex:    paneIndex,
			CurrentPath:  parts[8],
			Active:       parts[9] == "1",
		})
	}
	return panes, nil
}

// RestoreClient contains the tmux side effects used by statusd restore. The
// concrete implementation below always passes saved values as argv and never
// asks a shell to evaluate them.
type RestoreClient interface {
	NewSession(session, window, cwd string, windowIndex int) error
	NewWindow(session, window, cwd string, windowIndex int) error
	SplitWindow(session string, windowIndex int, cwd string) error
	ResizeWindow(session string, windowIndex, width, height int) error
	SelectLayout(session string, windowIndex int, layout string) error
	SelectPane(session string, windowIndex, paneIndex int) error
	SelectWindow(session string, windowIndex int) error
	KillSession(session string) error
}

type LiveRestoreClient struct{}

func (LiveRestoreClient) NewSession(session, window, cwd string, windowIndex int) error {
	args := []string{"new-session", "-d", "-s", session, "-n", window}
	if cwd != "" {
		args = append(args, "-c", cwd)
	}
	if err := runTmux(args...); err != nil {
		return err
	}
	cleanup := func(err error) error {
		_ = KillSession(session)
		return err
	}
	// A freshly created detached session has exactly one window, placed at the
	// server's base-index. Read that index with list-windows rather than
	// display-message: with no client attached, display-message resolves
	// window-scoped formats against no "current window" and returns an empty
	// string, which would spuriously trigger the move-window below.
	current, err := outputTmux("list-windows", "-t", exactSessionTarget(session), "-F", "#{window_index}")
	if err != nil {
		return cleanup(err)
	}
	currentIndex := strings.TrimSpace(current)
	if i := strings.IndexByte(currentIndex, '\n'); i >= 0 {
		currentIndex = currentIndex[:i]
	}
	if currentIndex == strconv.Itoa(windowIndex) {
		return nil
	}
	src, err := strconv.Atoi(currentIndex)
	if err != nil {
		return cleanup(fmt.Errorf("parse window index %q: %w", currentIndex, err))
	}
	if err := runTmux("move-window", "-s", windowTarget(session, src), "-t", windowTarget(session, windowIndex)); err != nil {
		return cleanup(err)
	}
	return nil
}

func (LiveRestoreClient) NewWindow(session, window, cwd string, windowIndex int) error {
	args := []string{"new-window", "-d", "-t", windowTarget(session, windowIndex), "-n", window}
	if cwd != "" {
		args = append(args, "-c", cwd)
	}
	return runTmux(args...)
}

func (LiveRestoreClient) SplitWindow(session string, windowIndex int, cwd string) error {
	args := []string{"split-window", "-d", "-t", windowTarget(session, windowIndex)}
	if cwd != "" {
		args = append(args, "-c", cwd)
	}
	return runTmux(args...)
}

func (LiveRestoreClient) ResizeWindow(session string, windowIndex, width, height int) error {
	return ResizeWindow(windowTarget(session, windowIndex), width, height)
}

func (LiveRestoreClient) SelectLayout(session string, windowIndex int, layout string) error {
	return runTmux("select-layout", "-t", windowTarget(session, windowIndex), layout)
}

func (LiveRestoreClient) SelectPane(session string, windowIndex, paneIndex int) error {
	return runTmux("select-pane", "-t", paneTarget(session, windowIndex, paneIndex))
}

func (LiveRestoreClient) SelectWindow(session string, windowIndex int) error {
	return runTmux("select-window", "-t", windowTarget(session, windowIndex))
}

func (LiveRestoreClient) KillSession(session string) error {
	return KillSession(session)
}

func exactSessionTarget(session string) string {
	return "=" + session
}

func windowTarget(session string, windowIndex int) string {
	return fmt.Sprintf("=%s:%d", session, windowIndex)
}

func paneTarget(session string, windowIndex, paneIndex int) string {
	return fmt.Sprintf("=%s:%d.%d", session, windowIndex, paneIndex)
}
