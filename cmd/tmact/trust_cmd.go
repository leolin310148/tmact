package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/leolin310148/tmact/internal/foldertrust"
)

var trustFolderRun = foldertrust.Run

func runTrustFolder(args []string) error {
	if wantsHelp(args) {
		return printCommandHelp("trust-folder")
	}
	fs := flag.NewFlagSet("trust-folder", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	target := fs.String("target", "", "exact tmux pane target")
	dir := fs.String("dir", "", "exact directory allowed by the trust decision")
	agent := fs.String("agent", "", "agent runtime: claude or codex")
	timeout := fs.Duration("timeout", foldertrust.DefaultTimeout, "maximum wait for a trust prompt or ready runtime")
	execute := fs.Bool("execute", false, "accept the matched trust prompt; default is dry-run")
	jsonOutput := fs.Bool("json", false, "print JSON output")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *target == "" {
		return errors.New("trust-folder requires --target")
	}
	if *dir == "" {
		return errors.New("trust-folder requires --dir")
	}
	if *agent == "" {
		return errors.New("trust-folder requires --agent claude|codex")
	}
	if *timeout <= 0 {
		return errors.New("trust-folder --timeout must be greater than zero")
	}
	result, err := trustFolderRun(context.Background(), foldertrust.Options{
		Target:  *target,
		Dir:     *dir,
		Agent:   strings.ToLower(*agent),
		Timeout: *timeout,
		DryRun:  !*execute,
	})
	if err != nil {
		return err
	}
	if *jsonOutput {
		return printJSON(result)
	}
	mode := "dry-run"
	if *execute {
		mode = "execute"
	}
	fmt.Printf("%s trust-folder %s [agent=%s dir=%s]\n", mode, result.Target, result.Agent, result.Dir)
	switch {
	case result.Accepted:
		fmt.Printf("accepted option %d: %s\n", result.OptionNumber, result.OptionLabel)
	case result.PromptFound:
		fmt.Printf("would accept option %d: %s\n", result.OptionNumber, result.OptionLabel)
	default:
		fmt.Println("no trust prompt was needed; agent became ready")
	}
	return nil
}
