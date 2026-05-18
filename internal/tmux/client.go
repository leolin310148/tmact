package tmux

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

type Layout struct {
	Sessions map[string]bool
	Windows  map[string]map[string]bool
}

type Pane struct {
	Session        string
	SessionID      string
	WindowIndex    int
	WindowName     string
	WindowActive   bool
	PaneIndex      int
	PaneID         string
	PanePID        int
	CurrentCommand string
	CurrentPath    string
	Active         bool
	InMode         bool
}

func ListLayout() (Layout, error) {
	layout := Layout{
		Sessions: map[string]bool{},
		Windows:  map[string]map[string]bool{},
	}

	output, err := outputTmux("list-windows", "-a", "-F", "#{session_name}\t#{window_name}")
	if err != nil {
		return layout, err
	}
	for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "\t", 2)
		if len(parts) != 2 {
			continue
		}
		session, window := parts[0], parts[1]
		layout.Sessions[session] = true
		if layout.Windows[session] == nil {
			layout.Windows[session] = map[string]bool{}
		}
		layout.Windows[session][window] = true
	}
	return layout, nil
}

func ListAllPanes() ([]Pane, error) {
	return listPanes([]string{"list-panes", "-a", "-F", paneListFormat})
}

func ListPanes(target string) ([]Pane, error) {
	if target == "" {
		return nil, fmt.Errorf("target cannot be empty")
	}
	return listPanes([]string{"list-panes", "-t", target, "-F", paneListFormat})
}

func ListSessionPanes(session string) ([]Pane, error) {
	if session == "" {
		return nil, fmt.Errorf("session cannot be empty")
	}
	return listPanes([]string{"list-panes", "-s", "-t", session, "-F", paneListFormat})
}

func NewSession(session string, window string, cwd string, command []string) error {
	args := []string{"new-session", "-d", "-s", session}
	if window != "" {
		args = append(args, "-n", window)
	}
	if cwd != "" {
		args = append(args, "-c", cwd)
	}
	if len(command) > 0 {
		args = append(args, strings.Join(command, " "))
	}
	return runTmux(args...)
}

func NewWindow(session string, window string, cwd string, command []string) error {
	args := []string{"new-window", "-d", "-t", session + ":"}
	if window != "" {
		args = append(args, "-n", window)
	}
	if cwd != "" {
		args = append(args, "-c", cwd)
	}
	if len(command) > 0 {
		args = append(args, strings.Join(command, " "))
	}
	return runTmux(args...)
}

const paneListFormat = "#{session_name}|#{session_id}|#{window_index}|#{window_name}|#{pane_index}|#{pane_id}|#{pane_pid}|#{pane_current_command}|#{pane_current_path}|#{pane_active}|#{pane_in_mode}|#{window_active}"

func listPanes(args []string) ([]Pane, error) {
	output, err := outputTmux(args...)
	if err != nil {
		return nil, err
	}
	return ParsePanes(output)
}

func ParsePanes(output string) ([]Pane, error) {
	var panes []Pane
	for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
		if line == "" {
			continue
		}
		parts := strings.Split(line, "|")
		if len(parts) == 1 {
			parts = strings.Split(line, "\t")
		}
		if len(parts) != 10 && len(parts) != 11 && len(parts) != 12 {
			return nil, fmt.Errorf("invalid tmux pane row %q", line)
		}
		sessionID := ""
		offset := 0
		if len(parts) == 12 {
			sessionID = parts[1]
			offset = 1
		}
		windowIndex, err := strconv.Atoi(parts[1+offset])
		if err != nil {
			return nil, fmt.Errorf("invalid window index %q: %w", parts[1+offset], err)
		}
		paneIndex, err := strconv.Atoi(parts[3+offset])
		if err != nil {
			return nil, fmt.Errorf("invalid pane index %q: %w", parts[3+offset], err)
		}
		panePID, err := strconv.Atoi(parts[5+offset])
		if err != nil {
			return nil, fmt.Errorf("invalid pane pid %q: %w", parts[5+offset], err)
		}
		windowActive := true
		if len(parts) == 11 || len(parts) == 12 {
			windowActive = parts[10+offset] == "1"
		}
		panes = append(panes, Pane{
			Session:        parts[0],
			SessionID:      sessionID,
			WindowIndex:    windowIndex,
			WindowName:     parts[2+offset],
			WindowActive:   windowActive,
			PaneIndex:      paneIndex,
			PaneID:         parts[4+offset],
			PanePID:        panePID,
			CurrentCommand: parts[6+offset],
			CurrentPath:    parts[7+offset],
			Active:         parts[8+offset] == "1",
			InMode:         parts[9+offset] == "1",
		})
	}
	return panes, nil
}

// CapturePane returns the pane's text with escape sequences stripped — the
// form classifiers and pattern matchers expect.
func CapturePane(target string, lines int) (string, error) {
	return capturePane(target, lines, false)
}

// CapturePaneANSI is CapturePane plus tmux's -e flag, keeping colour and
// attribute escape sequences. Use it only where the consumer renders escapes
// (the web UI); classifiers should stay on the plain CapturePane.
func CapturePaneANSI(target string, lines int) (string, error) {
	return capturePane(target, lines, true)
}

func capturePane(target string, lines int, escapes bool) (string, error) {
	if lines <= 0 {
		lines = 120
	}

	args := []string{"capture-pane", "-t", target, "-p", "-J", "-S", "-" + strconv.Itoa(lines)}
	if escapes {
		args = append(args, "-e")
	}
	cmd := exec.Command("tmux", args...)

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	output, err := cmd.Output()
	if err != nil {
		if stderr.Len() > 0 {
			return "", fmt.Errorf("tmux capture-pane failed: %s", stderr.String())
		}
		return "", fmt.Errorf("tmux capture-pane failed: %w", err)
	}
	return string(output), nil
}

func PasteText(target string, text string, enter bool) error {
	trimmed := strings.TrimRight(text, "\n")
	if trimmed != "" && !strings.Contains(trimmed, "\n") {
		if err := SendLiteral(target, trimmed); err != nil {
			return err
		}
		if enter {
			time.Sleep(100 * time.Millisecond)
			return SendKeys(target, []string{"Enter"})
		}
		return nil
	}

	bufferName := fmt.Sprintf("tmact-paste-%d-%d", os.Getpid(), time.Now().UnixNano())
	load := exec.Command("tmux", "load-buffer", "-b", bufferName, "-")
	load.Stdin = bytes.NewBufferString(text)

	var loadStderr bytes.Buffer
	load.Stderr = &loadStderr
	if err := load.Run(); err != nil {
		if loadStderr.Len() > 0 {
			return fmt.Errorf("tmux load-buffer failed: %s", loadStderr.String())
		}
		return fmt.Errorf("tmux load-buffer failed: %w", err)
	}
	defer func() {
		_ = runTmux("delete-buffer", "-b", bufferName)
	}()

	if err := runTmux("paste-buffer", "-t", target, "-b", bufferName); err != nil {
		return err
	}
	if enter {
		time.Sleep(100 * time.Millisecond)
		return SendKeys(target, []string{"Enter"})
	}
	return nil
}

func SendLiteral(target string, text string) error {
	return runTmux("send-keys", "-t", target, "-l", text)
}

func SendKeys(target string, keys []string) error {
	if len(keys) == 0 {
		return nil
	}
	args := append([]string{"send-keys", "-t", target}, keys...)
	return runTmux(args...)
}

func SetSessionOption(session string, key string, value string) error {
	if session == "" {
		return fmt.Errorf("session cannot be empty")
	}
	if key == "" {
		return fmt.Errorf("option key cannot be empty")
	}
	return runTmux("set-option", "-q", "-t", session, key, value)
}

func runTmux(args ...string) error {
	cmd := exec.Command("tmux", args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if stderr.Len() > 0 {
			return fmt.Errorf("tmux %s failed: %s", args[0], stderr.String())
		}
		return fmt.Errorf("tmux %s failed: %w", args[0], err)
	}
	return nil
}

func outputTmux(args ...string) (string, error) {
	cmd := exec.Command("tmux", args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	output, err := cmd.Output()
	if err != nil {
		if stderr.Len() > 0 {
			return "", fmt.Errorf("tmux %s failed: %s", args[0], stderr.String())
		}
		return "", fmt.Errorf("tmux %s failed: %w", args[0], err)
	}
	return string(output), nil
}
