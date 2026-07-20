package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/leolin310148/tmact/internal/statusd"
	"github.com/leolin310148/tmact/internal/web"
)

// fetchHumanActive is an injection point for tests.
var fetchHumanActive = web.FetchHumanActive

func runHumanActive(args []string) error {
	if wantsHelp(args) {
		return printCommandHelp("human-active")
	}
	fs := flag.NewFlagSet("human-active", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	socketPath := fs.String("socket-path", statusd.DefaultSocketPath, "daemon IPC unix socket")
	threshold := fs.Duration("threshold", 0, "inactivity cutoff; 0 uses the server default of "+web.DefaultHumanActiveThreshold.String())
	jsonOutput := fs.Bool("json", false, "print JSON output")
	quiet := fs.Bool("quiet", false, "no output; the exit code alone reports active (0) or inactive (1)")

	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("unexpected argument %q", fs.Arg(0))
	}

	status, err := fetchHumanActive(*socketPath, *threshold)
	if err != nil {
		return err
	}

	switch {
	case *quiet:
	case *jsonOutput:
		if err := printJSON(status); err != nil {
			return err
		}
	default:
		fmt.Printf("active: %t\n", status.Active)
		if status.LastActivity != nil {
			fmt.Printf("last_activity: %s\n", status.LastActivity.Format(time.RFC3339))
			fmt.Printf("idle: %s\n", (time.Duration(*status.IdleSeconds * float64(time.Second))).Round(time.Second))
		} else {
			fmt.Println("last_activity: never (no web UI action since statusd started)")
		}
		fmt.Printf("threshold: %s\n", time.Duration(status.ThresholdSeconds*float64(time.Second)))
	}
	if !status.Active {
		// Non-zero exit makes human-active scriptable in loops and cron
		// guards; --quiet keeps stderr clean for exactly that use.
		if *quiet {
			os.Exit(1)
		}
		return fmt.Errorf("human inactive")
	}
	return nil
}
