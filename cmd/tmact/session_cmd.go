package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	sessionlife "github.com/leolin310148/tmact/internal/session"
	"github.com/leolin310148/tmact/internal/statusd"
)

type sessionLifecycle interface {
	Close(name string, execute bool) (sessionlife.Result, error)
	Closed() []statusd.ClosedSession
	Reopen(name string, execute bool) (sessionlife.Result, error)
}

var newSessionLifecycle = func() (sessionLifecycle, error) {
	path := statusd.DefaultClosedSessionsPath()
	if path == "" {
		return nil, errors.New("cannot determine closed-session history path")
	}
	history := statusd.NewClosedSessionLog(path, statusd.DefaultClosedSessionsMax)
	return sessionlife.NewManager(history), nil
}

func runSession(args []string) error {
	if len(args) == 0 || wantsHelp(args) {
		return printCommandHelp("session")
	}
	switch args[0] {
	case "close":
		return runSessionClose(args[1:])
	case "closed":
		return runSessionClosed(args[1:])
	case "reopen":
		return runSessionReopen(args[1:])
	case "help":
		return printCommandHelp("session")
	default:
		return fmt.Errorf("unknown session subcommand %q", args[0])
	}
}

func runSessionClose(args []string) error {
	if wantsHelp(args) {
		return printCommandHelp("session close")
	}
	fs := flag.NewFlagSet("session close", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	execute := fs.Bool("execute", false, "close the exact session; default is dry-run")
	jsonOutput := fs.Bool("json", false, "print JSON output")
	name, err := parseSessionActionArgs(fs, args, "close")
	if err != nil {
		return err
	}
	manager, err := newSessionLifecycle()
	if err != nil {
		return err
	}
	result, err := manager.Close(name, *execute)
	if err != nil {
		return err
	}
	return printSessionResult(result, *jsonOutput)
}

func runSessionClosed(args []string) error {
	if wantsHelp(args) {
		return printCommandHelp("session closed")
	}
	fs := flag.NewFlagSet("session closed", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	jsonOutput := fs.Bool("json", false, "print JSON output")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("session closed does not accept positional arguments: %q", fs.Arg(0))
	}
	manager, err := newSessionLifecycle()
	if err != nil {
		return err
	}
	entries := manager.Closed()
	if *jsonOutput {
		return printJSON(map[string]any{"sessions": entries})
	}
	if len(entries) == 0 {
		fmt.Fprintln(os.Stdout, "No closed sessions.")
		return nil
	}
	w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintln(w, "SESSION\tRUNTIME\tCWD\tCLOSED AT")
	for _, entry := range entries {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", entry.Session, entry.Runtime, entry.CWD, entry.ClosedAt.Format("2006-01-02T15:04:05Z07:00"))
	}
	return w.Flush()
}

func runSessionReopen(args []string) error {
	if wantsHelp(args) {
		return printCommandHelp("session reopen")
	}
	fs := flag.NewFlagSet("session reopen", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	execute := fs.Bool("execute", false, "reopen the recorded session; default is dry-run")
	jsonOutput := fs.Bool("json", false, "print JSON output")
	name, err := parseSessionActionArgs(fs, args, "reopen")
	if err != nil {
		return err
	}
	manager, err := newSessionLifecycle()
	if err != nil {
		return err
	}
	result, err := manager.Reopen(name, *execute)
	if err != nil {
		return err
	}
	return printSessionResult(result, *jsonOutput)
}

func parseSessionActionArgs(fs *flag.FlagSet, args []string, action string) (string, error) {
	if len(args) == 0 || len(args) > 1 && !strings.HasPrefix(args[1], "-") {
		return "", fmt.Errorf("session %s requires exactly one session name", action)
	}
	name := args[0]
	if strings.HasPrefix(name, "-") {
		return "", fmt.Errorf("session %s requires the session name before flags", action)
	}
	if err := fs.Parse(args[1:]); err != nil {
		return "", err
	}
	if fs.NArg() != 0 {
		return "", fmt.Errorf("session %s requires exactly one session name", action)
	}
	return name, nil
}

func printSessionResult(result sessionlife.Result, jsonOutput bool) error {
	if jsonOutput {
		return printJSON(result)
	}
	mode := "dry-run"
	if result.Executed {
		mode = "execute"
	}
	fmt.Fprintf(os.Stdout, "%s session %s %s", mode, result.Action, result.Session)
	if result.CWD != "" {
		fmt.Fprintf(os.Stdout, " [cwd=%s]", result.CWD)
	}
	if result.Runtime != "" {
		fmt.Fprintf(os.Stdout, " [runtime=%s]", result.Runtime)
	}
	if result.Action == "reopen" && !result.RuntimeRestored {
		fmt.Fprint(os.Stdout, " [runtime=shell-fallback]")
	}
	fmt.Fprintln(os.Stdout)
	return nil
}
