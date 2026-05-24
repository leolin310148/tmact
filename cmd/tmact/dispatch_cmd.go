package main

import (
	"errors"
	"flag"
	"os"
	"strings"
	"time"

	"github.com/leolin310148/tmact/internal/dispatch"
)

func runDispatch(args []string) error {
	if wantsHelp(args) {
		return printCommandHelp("dispatch-work")
	}

	var session string
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		session = args[0]
		args = args[1:]
	}

	fs := flag.NewFlagSet("dispatch-work", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	dir := fs.String("dir", "", "working directory; sets cwd when the session is created")
	agent := fs.String("agent", "", "agent to launch: "+strings.Join(dispatch.SupportedAgents(), "|"))
	promptText := fs.String("prompt", "", "prompt text to send to the agent")
	readyTimeout := fs.Duration("ready-timeout", 30*time.Second, "max wait for the agent to become ready")
	readySettle := fs.Duration("ready-settle", dispatch.DefaultReadySettleDelay, "stable idle time after ready before sending the prompt")
	execute := fs.Bool("execute", false, "actually create, launch, and send; default is dry-run")
	jsonOutput := fs.Bool("json", false, "print JSON output")

	if err := fs.Parse(args); err != nil {
		return err
	}
	if session == "" && fs.NArg() > 0 {
		session = fs.Arg(0)
	}
	if session == "" {
		return errors.New("dispatch-work requires a session name as the first argument")
	}
	if *dir == "" {
		return errors.New("dispatch-work requires --dir")
	}
	if *agent == "" {
		return errors.New("dispatch-work requires --agent")
	}
	if *promptText == "" {
		return errors.New("dispatch-work requires --prompt")
	}

	report, err := dispatch.Run(dispatch.Options{
		Session:      session,
		Dir:          *dir,
		Agent:        *agent,
		Prompt:       *promptText,
		Execute:      *execute,
		ReadyTimeout: *readyTimeout,
		ReadySettle:  *readySettle,
	})
	if err != nil {
		return err
	}
	if *jsonOutput {
		return printJSON(report)
	}
	printDispatchReport(report)
	return nil
}
