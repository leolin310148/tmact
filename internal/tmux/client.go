package tmux

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
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

type CapturePaneInfo struct {
	Target      string
	PaneID      string
	HistorySize int
}

func ListLayout() (Layout, error) {
	layout := Layout{
		Sessions: map[string]bool{},
		Windows:  map[string]map[string]bool{},
	}

	output, err := outputTmux("list-windows", "-a", "-F", "#{session_name}\t#{window_name}")
	if err != nil {
		if isNoServerError(err) {
			return layout, nil
		}
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

func isNoServerError(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	// "no server running" / "failed to connect to server": socket exists but no
	// server answers. "error connecting to ... (no such file or directory)":
	// the socket file is absent, which is what macOS tmux reports on a fresh
	// boot before any server has started. All three mean "no sessions", so the
	// headless startup restore must treat them as an empty result, not a
	// failure, or it would skip restoring on exactly the boot it exists for.
	return strings.Contains(message, "no server running") ||
		strings.Contains(message, "failed to connect to server") ||
		strings.Contains(message, "error connecting to")
}

// isEmptyServerError reports whether err is tmux refusing a `list-panes -a`
// (or similar) because the server is running but holds no windows at all. tmux
// answers "no current target" in that case. For session capture that means
// zero sessions, not a failure — the same empty result as no server. This
// state is reachable when a restore attempt cleans up after itself or every
// session is closed while exit-empty is off, and must not be treated as a hard
// error or it would wedge periodic saves and startup restore.
func isEmptyServerError(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(strings.ToLower(err.Error()), "no current target")
}

// IsTargetGoneError reports whether tmux says that a target can no longer be
// resolved. Long-lived read-only commands use this to distinguish a vanished
// pane from an operational capture failure.
func IsTargetGoneError(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return isNoServerError(err) || isEmptyServerError(err) ||
		strings.Contains(message, "can't find pane") ||
		strings.Contains(message, "can't find window") ||
		strings.Contains(message, "can't find session") ||
		strings.Contains(message, "no such pane") ||
		strings.Contains(message, "no such window") ||
		strings.Contains(message, "no such session")
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

// PaneStartCommand returns the shell command tmux used to create the pane. It
// is useful as a runtime hint when the pane directly execs an agent and process
// inspection is unavailable.
func PaneStartCommand(target string) (string, error) {
	if target == "" {
		return "", fmt.Errorf("target cannot be empty")
	}
	output, err := outputTmux("display-message", "-p", "-t", target, "#{pane_start_command}")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(output), nil
}

// PaneSessionID returns the stable tmux session id (for example "$3") that
// currently owns target. Unlike a pane id, this lets long-lived consumers
// reject hook state left behind when tmux reuses a pane id.
func PaneSessionID(target string) (string, error) {
	if target == "" {
		return "", fmt.Errorf("target cannot be empty")
	}
	output, err := outputTmux("display-message", "-p", "-t", target, "#{session_id}")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(output), nil
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
		args = append(args, shellJoin(command))
	}
	return runTmux(args...)
}

// KillSession stops one exact tmux session.
func KillSession(session string) error {
	if strings.TrimSpace(session) == "" {
		return fmt.Errorf("session cannot be empty")
	}
	return runTmux("kill-session", "-t", exactSessionTarget(session))
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
		args = append(args, shellJoin(command))
	}
	return runTmux(args...)
}

// RunCommandInSession runs command in the fixed "command" window belonging to
// target's tmux session. The window is created in the target pane's current
// directory on first use, then reused so its shell history, output, and cwd are
// preserved across commands.
func RunCommandInSession(target string, command string) error {
	if strings.TrimSpace(target) == "" {
		return fmt.Errorf("target cannot be empty")
	}
	if strings.TrimSpace(command) == "" {
		return fmt.Errorf("command cannot be empty")
	}

	location, err := outputTmux("display-message", "-p", "-t", target, "#{session_name}\t#{pane_current_path}")
	if err != nil {
		return err
	}
	parts := strings.SplitN(strings.TrimSuffix(location, "\n"), "\t", 2)
	if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" {
		return fmt.Errorf("tmux returned an invalid session location for %q", target)
	}
	session := parts[0]

	panes, err := ListSessionPanes(session)
	if err != nil {
		return err
	}
	if pane, ok := reusableCommandPane(panes); ok {
		if pane.InMode {
			return fmt.Errorf("command window is in tmux mode; exit it before running another command")
		}
		shellState, err := outputTmux("display-message", "-p", "-t", pane.PaneID, "#{pane_current_command}\t#{default-shell}")
		if err != nil {
			return err
		}
		shellParts := strings.SplitN(strings.TrimSuffix(shellState, "\n"), "\t", 2)
		if len(shellParts) != 2 || shellParts[0] == "" || shellParts[0] != filepath.Base(shellParts[1]) {
			current := pane.CurrentCommand
			if len(shellParts) > 0 && shellParts[0] != "" {
				current = shellParts[0]
			}
			return fmt.Errorf("command window is busy running %q; wait for it to finish", current)
		}
		return PasteText(pane.PaneID, command, true)
	}

	args := []string{"new-window", "-d", "-P", "-F", "#{pane_id}", "-t", session + ":", "-n", "command"}
	if parts[1] != "" {
		args = append(args, "-c", parts[1])
	}
	pane, err := outputTmux(args...)
	if err != nil {
		return err
	}
	pane = strings.TrimSpace(pane)
	if pane == "" {
		return fmt.Errorf("tmux new-window returned no pane id")
	}
	return PasteText(pane, command, true)
}

// reusableCommandPane returns the active pane from the newest window named
// "command". Choosing the newest lets an installation upgraded from the old
// create-per-command behavior converge on one window without deleting any of
// the user's existing windows.
func reusableCommandPane(panes []Pane) (Pane, bool) {
	var selected Pane
	found := false
	for _, pane := range panes {
		if pane.WindowName != "command" {
			continue
		}
		if !found || pane.WindowIndex > selected.WindowIndex ||
			(pane.WindowIndex == selected.WindowIndex && pane.Active && !selected.Active) {
			selected = pane
			found = true
		}
	}
	return selected, found
}

func shellJoin(args []string) string {
	quoted := make([]string, 0, len(args))
	for _, arg := range args {
		if arg == "" {
			quoted = append(quoted, "''")
			continue
		}
		quoted = append(quoted, "'"+strings.ReplaceAll(arg, "'", "'\\''")+"'")
	}
	return strings.Join(quoted, " ")
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
		parts := splitPaneRow(line)
		if len(parts) != 10 && len(parts) != 11 && len(parts) != 12 {
			return nil, fmt.Errorf("invalid tmux pane row %q", line)
		}
		sessionID := ""
		offset := paneRowOffset(parts)
		if offset == 1 {
			sessionID = parts[1]
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
		if len(parts) > 10+offset {
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

func splitPaneRow(line string) []string {
	parts := strings.Split(line, "|")
	delimiter := "|"
	if len(parts) == 1 {
		parts = strings.Split(line, "\t")
		delimiter = "\t"
	}
	return normalizePaneRowFields(parts, delimiter)
}

func normalizePaneRowFields(parts []string, delimiter string) []string {
	suffixCount := paneRowSuffixCount(parts)
	if suffixCount == 0 {
		return parts
	}
	body := parts[:len(parts)-suffixCount]
	suffix := parts[len(parts)-suffixCount:]

	sessionEnd := paneRowSessionEnd(body)
	if sessionEnd < 0 {
		return parts
	}
	windowIndex := sessionEnd
	if isPaneRowSessionID(body[sessionEnd]) {
		windowIndex = sessionEnd + 1
	}
	paneIndex := paneRowPaneIndex(body, windowIndex+2)
	if paneIndex < 0 {
		return parts
	}

	normalized := make([]string, 0, 9+suffixCount)
	normalized = append(normalized, strings.Join(body[:sessionEnd], delimiter))
	if isPaneRowSessionID(body[sessionEnd]) {
		normalized = append(normalized, body[sessionEnd])
	}
	normalized = append(normalized,
		body[windowIndex],
		strings.Join(body[windowIndex+1:paneIndex], delimiter),
		body[paneIndex],
		body[paneIndex+1],
		body[paneIndex+2],
	)
	currentCommand, currentPath := paneRowCommandAndPath(body[paneIndex+3:], delimiter)
	normalized = append(normalized, currentCommand, currentPath)
	normalized = append(normalized, suffix...)
	return normalized
}

func paneRowCommandAndPath(fields []string, delimiter string) (string, string) {
	if len(fields) == 0 {
		return "", ""
	}
	if len(fields) == 1 {
		return fields[0], ""
	}
	for i := 1; i < len(fields); i++ {
		if looksLikePaneCurrentPath(fields[i]) {
			return strings.Join(fields[:i], delimiter), strings.Join(fields[i:], delimiter)
		}
	}
	return fields[0], strings.Join(fields[1:], delimiter)
}

func looksLikePaneCurrentPath(value string) bool {
	return strings.HasPrefix(value, "/") ||
		strings.HasPrefix(value, "~/") ||
		strings.HasPrefix(value, "./") ||
		strings.HasPrefix(value, "../")
}

func paneRowSuffixCount(parts []string) int {
	if len(parts) >= 11 &&
		isPaneRowFlag(parts[len(parts)-1]) &&
		isPaneRowFlag(parts[len(parts)-2]) &&
		isPaneRowFlag(parts[len(parts)-3]) {
		return 3
	}
	if len(parts) >= 10 &&
		isPaneRowFlag(parts[len(parts)-1]) &&
		isPaneRowFlag(parts[len(parts)-2]) {
		return 2
	}
	return 0
}

func paneRowSessionEnd(parts []string) int {
	for i := 1; i+1 < len(parts); i++ {
		if isPaneRowSessionID(parts[i]) && isPaneRowInt(parts[i+1]) {
			return i
		}
	}
	for i := 1; i < len(parts); i++ {
		if isPaneRowInt(parts[i]) {
			return i
		}
	}
	return -1
}

func paneRowPaneIndex(parts []string, start int) int {
	for i := start; i+4 < len(parts); i++ {
		if isPaneRowInt(parts[i]) && isPaneRowID(parts[i+1]) && isPaneRowInt(parts[i+2]) {
			return i
		}
	}
	return -1
}

func paneRowOffset(parts []string) int {
	if len(parts) > 2 && !isPaneRowInt(parts[1]) {
		return 1
	}
	return 0
}

func isPaneRowSessionID(value string) bool {
	if !strings.HasPrefix(value, "$") {
		return false
	}
	return isPaneRowInt(strings.TrimPrefix(value, "$"))
}

func isPaneRowID(value string) bool {
	if !strings.HasPrefix(value, "%") {
		return false
	}
	return isPaneRowInt(strings.TrimPrefix(value, "%"))
}

func isPaneRowInt(value string) bool {
	if value == "" {
		return false
	}
	_, err := strconv.Atoi(value)
	return err == nil
}

func isPaneRowFlag(value string) bool {
	return value == "0" || value == "1"
}

// CapturePane returns the pane's text with escape sequences stripped — the
// form classifiers and pattern matchers expect.
func CapturePane(target string, lines int) (string, error) {
	return capturePaneContext(context.Background(), target, lines, false)
}

// CapturePaneInfoForTarget resolves one tmux target to its canonical pane
// target and stable pane id, and reports the available scrollback size.
func CapturePaneInfoForTarget(target string) (CapturePaneInfo, error) {
	if target == "" {
		return CapturePaneInfo{}, fmt.Errorf("target cannot be empty")
	}
	output, err := outputTmux("display-message", "-p", "-t", target, "#{session_name}:#{window_index}.#{pane_index}\t#{pane_id}\t#{history_size}")
	if err != nil {
		return CapturePaneInfo{}, err
	}
	return parseCapturePaneInfo(output)
}

func parseCapturePaneInfo(output string) (CapturePaneInfo, error) {
	parts := strings.Split(strings.TrimSpace(output), "\t")
	if len(parts) != 3 || parts[0] == "" || parts[1] == "" {
		return CapturePaneInfo{}, fmt.Errorf("invalid tmux pane metadata %q", strings.TrimSpace(output))
	}
	historySize, err := strconv.Atoi(parts[2])
	if err != nil || historySize < 0 {
		return CapturePaneInfo{}, fmt.Errorf("invalid tmux pane history size %q", parts[2])
	}
	return CapturePaneInfo{Target: parts[0], PaneID: parts[1], HistorySize: historySize}, nil
}

// CapturePaneContext is CapturePane with cancellation support for long-lived
// callers such as the web pane stream.
func CapturePaneContext(ctx context.Context, target string, lines int) (string, error) {
	return capturePaneContext(ctx, target, lines, false)
}

// CapturePaneANSI is CapturePane with tmux's -e flag (keeping colour and
// attribute escape sequences) and without -J (so full-width input-box borders
// don't get joined onto the next line — see capturePane). Use it only where the
// consumer renders escapes (the web UI); classifiers stay on the plain CapturePane.
func CapturePaneANSI(target string, lines int) (string, error) {
	return capturePaneContext(context.Background(), target, lines, true)
}

func capturePaneContext(ctx context.Context, target string, lines int, escapes bool) (string, error) {
	if lines <= 0 {
		lines = 120
	}

	args := []string{"capture-pane", "-t", target, "-p", "-S", "-" + strconv.Itoa(lines)}
	if escapes {
		// ANSI path = the web UI. Deliberately omit -J: an agent input-box
		// border is exactly the pane width, so tmux marks that full-width row
		// as wrap-continued and -J would glue it onto the following prompt line
		// into one double-width logical line that soft-wraps and mangles the box
		// in the browser. Leaving each terminal row on its own line is what the
		// web renderer expects (joinWrappedFrames handles real box-art tables).
		args = append(args, "-e")
	} else {
		// Classifiers/pattern matchers want wrapped long lines rejoined.
		args = append(args, "-J")
	}
	cmd := exec.CommandContext(ctx, "tmux", args...)

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	output, err := cmd.Output()
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return "", fmt.Errorf("tmux capture-pane failed: %w", ctxErr)
		}
		if stderr.Len() > 0 {
			return "", fmt.Errorf("tmux capture-pane failed: %s", stderr.String())
		}
		return "", fmt.Errorf("tmux capture-pane failed: %w", err)
	}
	return string(output), nil
}

func PasteText(target string, text string, enter bool) error {
	trimmed := strings.TrimRight(text, "\n")
	if trimmed != "" && !strings.Contains(trimmed, "\n") && canSendLiteral(trimmed) {
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

	if err := runTmux(pasteBufferArgs(target, bufferName)...); err != nil {
		return err
	}
	if enter {
		time.Sleep(100 * time.Millisecond)
		return SendKeys(target, []string{"Enter"})
	}
	return nil
}

func canSendLiteral(text string) bool {
	// tmux parses a standalone semicolon argument as a command separator, even
	// when argv came from exec.Command. Use paste-buffer for that literal.
	return text != ";"
}

func pasteBufferArgs(target string, bufferName string) []string {
	return []string{"paste-buffer", "-p", "-t", target, "-b", bufferName}
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

func ClearPane(target string) error {
	if err := SendKeys(target, []string{"C-l"}); err != nil {
		return err
	}
	time.Sleep(100 * time.Millisecond)
	return runTmux("clear-history", "-t", target)
}

// ResizeWindow resizes the window containing target to cols x rows. tmux
// accepts a pane id as target and applies the size to the enclosing window.
func ResizeWindow(target string, cols, rows int) error {
	if target == "" {
		return fmt.Errorf("target cannot be empty")
	}
	if cols <= 0 || rows <= 0 {
		return fmt.Errorf("invalid size %dx%d", cols, rows)
	}
	return runTmux("resize-window", "-t", target, "-x", strconv.Itoa(cols), "-y", strconv.Itoa(rows))
}

// WindowSize describes one tmux window for the fixed-width sweep. Attached
// tracks whether any tmux client is attached to the owning session — resizing
// such a window would fight the attached client on the next render tick.
type WindowSize struct {
	WindowID string
	Width    int
	Height   int
	Attached bool
}

// ListWindowSizes returns the current size of every tmux window across all
// sessions, plus whether the owning session has any attached client.
func ListWindowSizes() ([]WindowSize, error) {
	out, err := outputTmux("list-windows", "-a", "-F", "#{window_id}\t#{window_width}\t#{window_height}\t#{session_attached}")
	if err != nil {
		return nil, err
	}
	var sizes []WindowSize
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		if line == "" {
			continue
		}
		parts := strings.Split(line, "\t")
		if len(parts) != 4 {
			return nil, fmt.Errorf("invalid window row %q", line)
		}
		w, err := strconv.Atoi(parts[1])
		if err != nil {
			return nil, fmt.Errorf("invalid window width %q: %w", parts[1], err)
		}
		h, err := strconv.Atoi(parts[2])
		if err != nil {
			return nil, fmt.Errorf("invalid window height %q: %w", parts[2], err)
		}
		attached, err := strconv.Atoi(parts[3])
		if err != nil {
			return nil, fmt.Errorf("invalid session_attached %q: %w", parts[3], err)
		}
		sizes = append(sizes, WindowSize{
			WindowID: parts[0],
			Width:    w,
			Height:   h,
			Attached: attached > 0,
		})
	}
	return sizes, nil
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

// tmuxArgs prepends the global `-u` flag so tmux always treats the client as
// UTF-8 capable. Without it, a tmux command run from a process with no
// LC_ALL/LC_CTYPE/LANG in its environment (notably the statusd daemon launched
// by launchd / systemd) is seen as a non-UTF-8 client, and the server then
// replaces every unprintable byte in format output — including the literal TAB
// used as a field delimiter — with "_". That silently broke ListWindowSizes
// (the fixed-width sweep) and would corrupt captured CJK/box-drawing content.
func tmuxArgs(args []string) []string {
	return append([]string{"-u"}, args...)
}

func runTmux(args ...string) error {
	cmd := exec.Command("tmux", tmuxArgs(args)...)
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
	cmd := exec.Command("tmux", tmuxArgs(args)...)
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
