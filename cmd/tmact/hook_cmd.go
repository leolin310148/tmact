package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/leolin310148/tmact/internal/shellhook"
	"github.com/leolin310148/tmact/internal/statusd"
)

// sendHookEvent is an injection point for tests.
var sendHookEvent = shellhook.Send

// hookSocketEnv overrides the emit socket path without flags so the
// init-script emits stay short; the --socket-path flag still wins.
const hookSocketEnv = "TMACT_HOOK_SOCKET"

func runHook(args []string) error {
	if wantsHelp(args) {
		return printCommandHelp("hook")
	}
	if len(args) == 0 {
		return errors.New("hook requires a subcommand: init, emit")
	}
	switch args[0] {
	case "init":
		return runHookInit(args[1:])
	case "emit":
		return runHookEmit(args[1:])
	case "help", "-h", "--help":
		return printCommandHelp("hook")
	default:
		return fmt.Errorf("unknown hook subcommand %q", args[0])
	}
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

	socket := *socketPath
	if socket == "" {
		socket = os.Getenv(hookSocketEnv)
	}
	if socket == "" {
		socket = statusd.DefaultSocketPath
	}

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
