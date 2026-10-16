package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"

	"github.com/leolin310148/tmact/internal/credreq"
	"github.com/leolin310148/tmact/internal/credstore"
	"github.com/leolin310148/tmact/internal/statusd"
)

const (
	credExitDeclined = 3
	credExitExpired  = 4
)

// credWaitSlack lets the CLI outlive the server-side expiry slightly so the
// daemon's "expired" answer arrives instead of a client-side timeout.
const credWaitSlack = 10 * time.Second

var (
	defaultCredStore = credstore.Default
	credExec         = syscall.Exec
	credLookPath     = exec.LookPath
)

type credRequestReport struct {
	Name   string `json:"name"`
	State  string `json:"state"`
	Path   string `json:"path,omitempty"`
	ID     string `json:"id,omitempty"`
	Reused bool   `json:"reused,omitempty"`
}

func runCred(args []string) error {
	if wantsHelp(args) {
		return printCommandHelp("cred")
	}
	if len(args) == 0 {
		return errors.New("cred requires a subcommand: request, path, ls, rm, run")
	}
	switch args[0] {
	case "request":
		return runCredRequest(args[1:])
	case "path":
		return runCredPath(args[1:])
	case "ls":
		return runCredList(args[1:])
	case "rm":
		return runCredRemove(args[1:])
	case "run":
		return runCredRun(args[1:])
	case "help", "-h", "--help":
		return printCommandHelp("cred")
	default:
		return fmt.Errorf("unknown cred subcommand %q", args[0])
	}
}

func runCredRequest(args []string) error {
	if wantsHelp(args) {
		return printCommandHelp("cred request")
	}
	fs := flag.NewFlagSet("cred request", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	reason := fs.String("reason", "", "why the credential is needed; shown in the notification")
	timeout := fs.Duration("timeout", credreq.DefaultTimeout, "how long to wait for the user")
	refresh := fs.Bool("refresh", false, "ask again even if the credential is already stored")
	socketPath := fs.String("socket-path", statusd.DefaultSocketPath, "statusd IPC unix socket")
	jsonOut := fs.Bool("json", false, "print JSON output")
	name, err := parseCredNameArgs(fs, args)
	if err != nil {
		return err
	}
	if *timeout <= 0 || *timeout > credreq.MaxTimeout {
		return fmt.Errorf("--timeout must be between 1s and %s", credreq.MaxTimeout)
	}
	store, err := defaultCredStore()
	if err != nil {
		return err
	}
	if !*refresh {
		ok, err := store.Exists(name)
		if err != nil {
			return err
		}
		if ok {
			path, _ := store.Path(name)
			return printCredRequestReport(credRequestReport{Name: name, State: "stored", Path: path}, *jsonOut)
		}
	}

	in := credreq.CreateInput{Name: name, Reason: *reason, PaneID: os.Getenv("TMUX_PANE"), Timeout: *timeout}
	in.CWD, _ = os.Getwd()
	in.Host, _ = os.Hostname()
	client := credreq.Client{SocketPath: *socketPath}
	ctx, cancel := context.WithTimeout(context.Background(), *timeout+credWaitSlack)
	defer cancel()
	req, created, err := client.Create(ctx, in)
	if err != nil {
		return fmt.Errorf("request credential %s: %w", name, err)
	}
	if !*jsonOut {
		verb := "requested"
		if !created {
			verb = "already requested"
		}
		fmt.Fprintf(os.Stderr, "credential %s %s; waiting up to %s for the user to answer the notification...\n",
			name, verb, time.Until(req.ExpiresAt).Round(time.Second))
	}
	resolved, err := client.Wait(ctx, req.ID)
	switch {
	case errors.Is(err, credreq.ErrUnknown), errors.Is(err, credreq.ErrDaemonUnavailable):
		return fmt.Errorf("credential request %s was lost (statusd restarted?); run the command again: %w", req.ID, err)
	case errors.Is(err, context.DeadlineExceeded):
		resolved = credreq.Request{ID: req.ID, Name: name, State: credreq.StateExpired}
	case err != nil:
		return err
	}
	report := credRequestReport{Name: name, State: resolved.State, Path: resolved.Path, ID: req.ID, Reused: !created}
	if err := printCredRequestReport(report, *jsonOut); err != nil {
		return err
	}
	switch resolved.State {
	case credreq.StateSubmitted:
		return nil
	case credreq.StateDeclined:
		return exitCodeError{code: credExitDeclined, err: fmt.Errorf("credential %s was declined", name), reported: *jsonOut}
	default:
		return exitCodeError{code: credExitExpired, err: fmt.Errorf("credential %s was not provided before the request expired", name), reported: *jsonOut}
	}
}

func printCredRequestReport(report credRequestReport, jsonOut bool) error {
	if jsonOut {
		return printJSON(report)
	}
	if report.Path != "" {
		fmt.Println(report.Path)
	}
	return nil
}

func runCredPath(args []string) error {
	if wantsHelp(args) {
		return printCommandHelp("cred path")
	}
	fs := flag.NewFlagSet("cred path", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	name, err := parseCredNameArgs(fs, args)
	if err != nil {
		return err
	}
	store, err := defaultCredStore()
	if err != nil {
		return err
	}
	ok, err := store.Exists(name)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("credential %s is not stored; ask for it with: tmact cred request %s --reason ...", name, name)
	}
	path, _ := store.Path(name)
	fmt.Println(path)
	return nil
}

func runCredList(args []string) error {
	if wantsHelp(args) {
		return printCommandHelp("cred ls")
	}
	fs := flag.NewFlagSet("cred ls", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	jsonOut := fs.Bool("json", false, "print JSON output")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return errors.New("cred ls takes no arguments")
	}
	store, err := defaultCredStore()
	if err != nil {
		return err
	}
	entries, err := store.List()
	if err != nil {
		return err
	}
	if *jsonOut {
		if entries == nil {
			entries = []credstore.Entry{}
		}
		return printJSON(entries)
	}
	for _, e := range entries {
		fmt.Printf("%s\t%s\n", e.Name, e.ModTime.Local().Format(time.RFC3339))
	}
	return nil
}

func runCredRemove(args []string) error {
	if wantsHelp(args) {
		return printCommandHelp("cred rm")
	}
	fs := flag.NewFlagSet("cred rm", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	name, err := parseCredNameArgs(fs, args)
	if err != nil {
		return err
	}
	store, err := defaultCredStore()
	if err != nil {
		return err
	}
	return store.Remove(name)
}

// runCredRun execs a command with credentials in its environment so the
// values never pass through the caller's terminal or transcript.
func runCredRun(args []string) error {
	if wantsHelp(args) {
		return printCommandHelp("cred run")
	}
	fs := flag.NewFlagSet("cred run", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	var envs repeatedStrings
	fs.Var(&envs, "env", "VAR=NAME: set environment variable VAR to credential NAME (repeatable)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	command := fs.Args()
	if len(envs) == 0 {
		return errors.New("cred run needs at least one --env VAR=NAME")
	}
	if len(command) == 0 {
		return errors.New("cred run needs a command after --")
	}
	store, err := defaultCredStore()
	if err != nil {
		return err
	}
	env := os.Environ()
	for _, spec := range envs {
		variable, name, ok := strings.Cut(spec, "=")
		if !ok || !validEnvName(variable) {
			return fmt.Errorf("invalid --env %q, expected VAR=NAME", spec)
		}
		value, err := store.Read(name)
		if errors.Is(err, credstore.ErrNotFound) {
			return fmt.Errorf("credential %s is not stored; ask for it with: tmact cred request %s --reason ...", name, name)
		}
		if err != nil {
			return err
		}
		if strings.ContainsRune(string(value), 0) {
			return fmt.Errorf("credential %s contains a NUL byte and cannot be passed in the environment", name)
		}
		env = append(env, variable+"="+string(value))
	}
	path, err := credLookPath(command[0])
	if err != nil {
		return err
	}
	return credExec(path, command, env)
}

func parseCredNameArgs(fs *flag.FlagSet, args []string) (string, error) {
	// Accept the name before or after flags.
	var name string
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		name, args = args[0], args[1:]
	}
	if err := fs.Parse(args); err != nil {
		return "", err
	}
	if name == "" && fs.NArg() > 0 {
		name = fs.Arg(0)
		if err := fs.Parse(fs.Args()[1:]); err != nil {
			return "", err
		}
	}
	if name == "" || fs.NArg() != 0 {
		return "", fmt.Errorf("%s takes exactly one credential NAME", fs.Name())
	}
	if !credstore.ValidName(name) {
		_, err := credstore.Store{}.Path(name)
		return "", err
	}
	return name, nil
}

func validEnvName(name string) bool {
	if name == "" {
		return false
	}
	for i, r := range name {
		if r == '_' || (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') || (i > 0 && r >= '0' && r <= '9') {
			continue
		}
		return false
	}
	return true
}

// exitCodeError ends the process with a specific status. reported means the
// outcome was already printed (as JSON), so main prints nothing more.
type exitCodeError struct {
	code     int
	err      error
	reported bool
}

func (e exitCodeError) Error() string { return e.err.Error() }
func (e exitCodeError) Unwrap() error { return e.err }
