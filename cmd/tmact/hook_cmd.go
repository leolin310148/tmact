package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/leolin310148/tmact/internal/shellhook"
	"github.com/leolin310148/tmact/internal/statusd"
	"github.com/leolin310148/tmact/internal/tmux"
)

// sendHookEvent and fetchHookStates are injection points for tests.
var (
	sendHookEvent   = shellhook.Send
	fetchHookStates = shellhook.FetchStates
	lookupSessionID = tmux.PaneSessionID
)

// hookSocketEnv overrides the emit socket path without flags so the
// init-script emits stay short; the --socket-path flag still wins.
const hookSocketEnv = "TMACT_HOOK_SOCKET"

func runHook(args []string) error {
	if wantsHelp(args) {
		return printCommandHelp("hook")
	}
	if len(args) == 0 {
		return errors.New("hook requires a subcommand: init, emit, state, doctor")
	}
	switch args[0] {
	case "init":
		return runHookInit(args[1:])
	case "emit":
		return runHookEmit(args[1:])
	case "state":
		return runHookState(args[1:])
	case "doctor":
		return runHookDoctor(args[1:])
	case "help", "-h", "--help":
		return printCommandHelp("hook")
	default:
		return fmt.Errorf("unknown hook subcommand %q", args[0])
	}
}

// resolveHookSocket applies the shared emit/read socket precedence:
// explicit flag, then $TMACT_HOOK_SOCKET, then the default statusd socket.
func resolveHookSocket(flagValue string) string {
	if flagValue != "" {
		return flagValue
	}
	if env := os.Getenv(hookSocketEnv); env != "" {
		return env
	}
	return statusd.DefaultSocketPath
}

func runHookInit(args []string) error {
	if wantsHelp(args) {
		return printCommandHelp("hook init")
	}
	fs := flag.NewFlagSet("hook init", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return fmt.Errorf("hook init requires exactly one shell argument: %s", strings.Join(shellhook.Shells, ", "))
	}
	script, err := shellhook.InitScript(fs.Arg(0))
	if err != nil {
		return err
	}
	fmt.Fprint(os.Stdout, script)
	return nil
}

func runHookEmit(args []string) error {
	if wantsHelp(args) {
		return printCommandHelp("hook emit")
	}
	fs := flag.NewFlagSet("hook emit", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	eventType := fs.String("type", "", "event type: preexec or precmd")
	paneID := fs.String("pane-id", "", "tmux pane id such as %5; defaults to $TMUX_PANE")
	sessionID := fs.String("session-id", "", "tmux session id; optional")
	commandID := fs.String("command-id", "", "id correlating a preexec with its precmd")
	cwd := fs.String("cwd", "", "working directory; defaults to $PWD")
	command := fs.String("command", "", "command line being run (preexec)")
	exitCode := fs.String("exit-code", "", "command exit status (precmd)")
	socketPath := fs.String("socket-path", "", "statusd IPC unix socket; defaults to $TMACT_HOOK_SOCKET, then "+statusd.DefaultSocketPath)
	timeout := fs.Duration("timeout", shellhook.DefaultSendTimeout, "max time for the send round-trip")
	stdin := fs.Bool("stdin", false, "read one complete event as JSON from stdin instead of flags")
	quiet := fs.Bool("quiet", false, "exit 0 silently on any failure; for use from shell hooks")
	jsonOutput := fs.Bool("json", false, "print JSON output")

	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return hookEmitFail(*quiet, fmt.Errorf("unexpected argument %q", fs.Arg(0)))
	}

	event, err := buildHookEvent(hookEmitInputs{
		Type:      *eventType,
		PaneID:    *paneID,
		SessionID: *sessionID,
		CommandID: *commandID,
		CWD:       *cwd,
		Command:   *command,
		ExitCode:  *exitCode,
		Stdin:     *stdin,
	}, os.Getenv, os.Stdin, tmactNow())
	if err != nil {
		return hookEmitFail(*quiet, err)
	}
	// Pane ids can be reused after panes or the tmux server are recreated.
	// Attach the owning session id so statusd can distinguish a current event
	// from stale in-memory state for an earlier owner of the same pane id.
	// Keep this best-effort: shell hooks must never break the user's prompt if
	// tmux disappears during the lookup.
	if event.SessionID == "" && event.PaneID != "" {
		if sessionID, lookupErr := lookupSessionID(event.PaneID); lookupErr == nil {
			event.SessionID = sessionID
		}
	}

	socket := resolveHookSocket(*socketPath)

	sendErr := sendHookEvent(socket, event, *timeout)
	if *jsonOutput {
		result := map[string]any{"delivered": sendErr == nil, "event": event}
		if sendErr != nil {
			result["error"] = sendErr.Error()
		}
		if printErr := printJSON(result); printErr != nil && sendErr == nil {
			sendErr = printErr
		}
	}
	if sendErr != nil {
		return hookEmitFail(*quiet, sendErr)
	}
	return nil
}

// hookEmitFail implements the emit failure contract: hooks in live shells
// pass --quiet so a missing daemon (or anything else) can never break the
// prompt; without it failures surface normally for debugging.
func hookEmitFail(quiet bool, err error) error {
	if quiet {
		return nil
	}
	return err
}

type hookEmitInputs struct {
	Type      string
	PaneID    string
	SessionID string
	CommandID string
	CWD       string
	Command   string
	ExitCode  string
	Stdin     bool
}

func buildHookEvent(in hookEmitInputs, getenv func(string) string, stdin io.Reader, now time.Time) (shellhook.Event, error) {
	if in.Stdin {
		raw, err := io.ReadAll(io.LimitReader(stdin, 16<<10))
		if err != nil {
			return shellhook.Event{}, fmt.Errorf("read event from stdin: %w", err)
		}
		return shellhook.ParseEvent(raw, now)
	}
	event := shellhook.Event{
		Version:   shellhook.EventVersion,
		Type:      in.Type,
		PaneID:    in.PaneID,
		SessionID: in.SessionID,
		CommandID: in.CommandID,
		CWD:       in.CWD,
		Command:   in.Command,
		Timestamp: now,
	}
	if event.PaneID == "" {
		event.PaneID = getenv("TMUX_PANE")
	}
	if event.CWD == "" {
		event.CWD = getenv("PWD")
	}
	if in.ExitCode != "" {
		code, err := strconv.Atoi(in.ExitCode)
		if err != nil {
			return shellhook.Event{}, fmt.Errorf("invalid --exit-code %q", in.ExitCode)
		}
		event.ExitCode = &code
	}
	if err := event.Validate(); err != nil {
		return shellhook.Event{}, err
	}
	return event, nil
}

func runHookState(args []string) error {
	if wantsHelp(args) {
		return printCommandHelp("hook state")
	}
	fs := flag.NewFlagSet("hook state", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	paneID := fs.String("pane-id", "", "limit to one tmux pane id such as %5; defaults to all panes")
	socketPath := fs.String("socket-path", "", "statusd IPC unix socket; defaults to $TMACT_HOOK_SOCKET, then "+statusd.DefaultSocketPath)
	timeout := fs.Duration("timeout", shellhook.DefaultFetchTimeout, "max time for the fetch round-trip")
	jsonOutput := fs.Bool("json", false, "print JSON output")

	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("unexpected argument %q", fs.Arg(0))
	}

	socket := resolveHookSocket(*socketPath)
	states, err := fetchHookStates(socket, *paneID, *timeout)
	if err != nil {
		return err
	}
	if *jsonOutput {
		return printJSON(shellhook.StatesResponse{Panes: states})
	}
	printHookStates(os.Stdout, socket, states, tmactNow())
	return nil
}

func printHookStates(w io.Writer, socket string, states map[string]shellhook.PaneState, now time.Time) {
	fmt.Fprintf(w, "socket: %s\n", socket)
	fmt.Fprintf(w, "panes: %d\n", len(states))
	if len(states) == 0 {
		fmt.Fprintln(w, "no shell hook events recorded")
		return
	}
	ids := make([]string, 0, len(states))
	for id := range states {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		state := states[id]
		fmt.Fprintf(w, "\n%s", id)
		if state.SessionID != "" {
			fmt.Fprintf(w, "  session=%s", state.SessionID)
		}
		fmt.Fprintf(w, "  updated %s ago\n", formatAge(now.Sub(state.UpdatedAt)))
		fmt.Fprintf(w, "  %s\n", describePaneState(state, now))
	}
}

// describePaneState renders one pane's active-or-completed command in one line.
func describePaneState(state shellhook.PaneState, now time.Time) string {
	switch {
	case state.Active != nil:
		c := state.Active
		return fmt.Sprintf("active     %s started %s ago", describeCommand(c.CommandID, c.Command, c.CWD), formatAge(now.Sub(c.StartedAt)))
	case state.Completed != nil:
		c := state.Completed
		exit := "exit=?"
		if c.ExitCode != nil {
			exit = fmt.Sprintf("exit=%d", *c.ExitCode)
		}
		match := "unmatched"
		if c.Matched {
			match = "matched"
		}
		return fmt.Sprintf("completed  %s %s %s ended %s ago", describeCommand(c.CommandID, c.Command, c.CWD), exit, match, formatAge(now.Sub(c.EndedAt)))
	default:
		return "no command recorded"
	}
}

func describeCommand(id, command, cwd string) string {
	var b strings.Builder
	if id != "" {
		fmt.Fprintf(&b, "%s ", id)
	}
	if command != "" {
		fmt.Fprintf(&b, "%q", command)
	} else {
		b.WriteString("(no command text)")
	}
	if cwd != "" {
		fmt.Fprintf(&b, " cwd=%s", cwd)
	}
	return b.String()
}

// hookDoctorInputs are the resolved environment facts a doctor run reads;
// splitting them out keeps buildHookDoctor pure and unit-testable.
type hookDoctorInputs struct {
	Socket       string
	SocketExists bool
	PaneID       string // pane to check; empty means "no pane to check"
	InTmux       bool
	Timeout      time.Duration
}

type hookDoctorCheck struct {
	Name   string `json:"name"`
	Status string `json:"status"` // ok | warn | fail
	Detail string `json:"detail"`
}

type hookDoctorReport struct {
	Socket          string               `json:"socket"`
	SocketExists    bool                 `json:"socket_exists"`
	DaemonReachable bool                 `json:"daemon_reachable"`
	InTmux          bool                 `json:"in_tmux"`
	PaneID          string               `json:"pane_id,omitempty"`
	PaneHasEvents   bool                 `json:"pane_has_events"`
	Pane            *shellhook.PaneState `json:"pane,omitempty"`
	PaneCount       int                  `json:"pane_count"`
	Checks          []hookDoctorCheck    `json:"checks"`
	Healthy         bool                 `json:"healthy"`
}

// buildHookDoctor runs the read-only diagnostic checklist. It only reads:
// it never emits events into panes or edits shell rc files. The daemon fetch
// goes through fetch (shellhook.FetchStates in production, stubbed in tests).
func buildHookDoctor(in hookDoctorInputs, fetch func(string, string, time.Duration) (map[string]shellhook.PaneState, error)) hookDoctorReport {
	r := hookDoctorReport{
		Socket:       in.Socket,
		SocketExists: in.SocketExists,
		InTmux:       in.InTmux,
		PaneID:       in.PaneID,
	}

	switch {
	case in.InTmux:
		if in.PaneID != "" {
			r.Checks = append(r.Checks, hookDoctorCheck{"tmux", "ok", "inside tmux; checking pane " + in.PaneID})
		} else {
			r.Checks = append(r.Checks, hookDoctorCheck{"tmux", "ok", "inside tmux"})
		}
	case in.PaneID != "":
		r.Checks = append(r.Checks, hookDoctorCheck{"tmux", "warn", "TMUX_PANE unset; checking --pane-id " + in.PaneID})
	default:
		r.Checks = append(r.Checks, hookDoctorCheck{"tmux", "warn", "not inside tmux (TMUX_PANE unset); hooks only emit inside tmux"})
	}

	if in.SocketExists {
		r.Checks = append(r.Checks, hookDoctorCheck{"socket", "ok", in.Socket})
	} else {
		r.Checks = append(r.Checks, hookDoctorCheck{"socket", "fail", in.Socket + " does not exist; is statusd running?"})
	}

	states, err := fetch(in.Socket, "", in.Timeout)
	if err != nil {
		detail := err.Error()
		if errors.Is(err, shellhook.ErrDaemonUnavailable) {
			detail = "statusd not reachable at " + in.Socket
		}
		r.Checks = append(r.Checks, hookDoctorCheck{"daemon", "fail", detail})
		r.Healthy = false
		return r
	}
	r.DaemonReachable = true
	r.PaneCount = len(states)
	r.Checks = append(r.Checks, hookDoctorCheck{"daemon", "ok", fmt.Sprintf("reachable; %d pane(s) with hook state", len(states))})

	switch {
	case in.PaneID == "":
		r.Checks = append(r.Checks, hookDoctorCheck{"pane_events", "warn", "no pane to check (not inside tmux; pass --pane-id to check a specific pane)"})
	default:
		if st, ok := states[in.PaneID]; ok {
			state := st
			r.Pane = &state
			r.PaneHasEvents = true
			r.Checks = append(r.Checks, hookDoctorCheck{"pane_events", "ok", "hooks are emitting for " + in.PaneID})
		} else {
			r.Checks = append(r.Checks, hookDoctorCheck{"pane_events", "warn", "no events recorded for " + in.PaneID + "; source `tmact hook init <shell>` and run a command"})
		}
	}

	r.Healthy = true
	for _, c := range r.Checks {
		if c.Status == "fail" {
			r.Healthy = false
		}
	}
	return r
}

func runHookDoctor(args []string) error {
	if wantsHelp(args) {
		return printCommandHelp("hook doctor")
	}
	fs := flag.NewFlagSet("hook doctor", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	paneID := fs.String("pane-id", "", "pane to check for recorded events; defaults to $TMUX_PANE")
	socketPath := fs.String("socket-path", "", "statusd IPC unix socket; defaults to $TMACT_HOOK_SOCKET, then "+statusd.DefaultSocketPath)
	timeout := fs.Duration("timeout", shellhook.DefaultFetchTimeout, "max time for the fetch round-trip")
	jsonOutput := fs.Bool("json", false, "print JSON output")

	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("unexpected argument %q", fs.Arg(0))
	}

	socket := resolveHookSocket(*socketPath)
	_, statErr := os.Stat(socket)
	pane := *paneID
	inTmux := os.Getenv("TMUX_PANE") != ""
	if pane == "" {
		pane = os.Getenv("TMUX_PANE")
	}
	report := buildHookDoctor(hookDoctorInputs{
		Socket:       socket,
		SocketExists: statErr == nil,
		PaneID:       pane,
		InTmux:       inTmux,
		Timeout:      *timeout,
	}, fetchHookStates)

	if *jsonOutput {
		if err := printJSON(report); err != nil {
			return err
		}
	} else {
		printHookDoctor(os.Stdout, report)
	}
	if !report.Healthy {
		return fmt.Errorf("hook doctor: statusd daemon unreachable at %s", report.Socket)
	}
	return nil
}

func printHookDoctor(w io.Writer, report hookDoctorReport) {
	for _, c := range report.Checks {
		fmt.Fprintf(w, "[%s] %-11s %s\n", hookDoctorMark(c.Status), c.Name, c.Detail)
	}
	if report.Healthy {
		fmt.Fprintln(w, "\nhook pipeline looks healthy")
	} else {
		fmt.Fprintln(w, "\nhook pipeline has problems (see fail lines above)")
	}
}

func hookDoctorMark(status string) string {
	switch status {
	case "ok":
		return "ok"
	case "warn":
		return "--"
	default:
		return "!!"
	}
}
