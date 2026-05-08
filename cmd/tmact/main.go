package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"

	"tmact/internal/loop"
	"tmact/internal/prompt"
	"tmact/internal/tmux"
	"tmact/internal/watch"
)

type detectResult struct {
	Target string                  `json:"target"`
	Found  bool                    `json:"found"`
	Prompt *prompt.DirectoryAccess `json:"prompt,omitempty"`
	Error  string                  `json:"error,omitempty"`
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		return usage()
	}

	switch args[0] {
	case "detect":
		return runDetect(args[1:])
	case "loop":
		return runLoop(args[1:])
	case "watch":
		return runWatch(args[1:])
	case "help", "-h", "--help":
		return usage()
	default:
		return fmt.Errorf("unknown command %q\n\n%s", args[0], usageText())
	}
}

func runDetect(args []string) error {
	fs := flag.NewFlagSet("detect", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	target := fs.String("target", "z_sample-project_sample:0.0", "tmux target pane/window/session to capture")
	lines := fs.Int("lines", 120, "number of pane history lines to capture")
	jsonOutput := fs.Bool("json", false, "print JSON output")

	if err := fs.Parse(args); err != nil {
		return err
	}
	if *target == "" {
		return errors.New("--target cannot be empty")
	}

	captured, err := tmux.CapturePane(*target, *lines)
	result := detectResult{Target: *target}
	if err != nil {
		result.Error = err.Error()
		printDetectResult(result, *jsonOutput)
		return err
	}

	detected := prompt.DetectDirectoryAccess(captured)
	if detected != nil {
		result.Found = true
		result.Prompt = detected
	}

	printDetectResult(result, *jsonOutput)
	if !result.Found {
		return nil
	}
	return nil
}

func runLoop(args []string) error {
	fs := flag.NewFlagSet("loop", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	configPath := fs.String("config", "", "path to loop YAML config")
	dryRun := fs.Bool("dry-run", false, "print actions without sending anything to tmux")
	once := fs.Bool("once", false, "run one observe/action pass and exit")
	assumeIdleOnStart := fs.Bool("assume-idle-on-start", false, "treat the pane as already idle when the loop starts")

	if err := fs.Parse(args); err != nil {
		return err
	}
	if *configPath == "" {
		return errors.New("--config is required")
	}

	cfg, err := loop.LoadConfig(*configPath)
	if err != nil {
		return err
	}
	if *assumeIdleOnStart {
		cfg.AssumeIdleOnStart = true
	}

	runner := loop.NewRunner(cfg, loop.Options{
		DryRun: *dryRun,
		Once:   *once,
	})
	return runner.Run(context.Background())
}

func runWatch(args []string) error {
	fs := flag.NewFlagSet("watch", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	configPath := fs.String("config", "", "path to watch YAML config")
	dryRun := fs.Bool("dry-run", false, "print decisions without sending anything to tmux")
	once := fs.Bool("once", false, "run one watch pass and exit")

	if err := fs.Parse(args); err != nil {
		return err
	}
	if *configPath == "" {
		return errors.New("--config is required")
	}

	cfg, err := watch.LoadConfig(*configPath)
	if err != nil {
		return err
	}

	runner := watch.NewRunner(cfg, watch.Options{
		DryRun: *dryRun,
		Once:   *once,
	})
	return runner.Run(context.Background())
}

func printDetectResult(result detectResult, jsonOutput bool) {
	if jsonOutput {
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		_ = encoder.Encode(result)
		return
	}

	if result.Error != "" {
		fmt.Printf("target: %s\nerror: %s\n", result.Target, result.Error)
		return
	}
	if !result.Found || result.Prompt == nil {
		fmt.Printf("target: %s\nfound: false\n", result.Target)
		return
	}

	fmt.Printf("target: %s\nfound: true\n", result.Target)
	fmt.Printf("title: %s\n", result.Prompt.Title)
	if result.Prompt.Path != "" {
		fmt.Printf("path: %s\n", result.Prompt.Path)
	}
	if len(result.Prompt.Paths) > 1 {
		fmt.Printf("paths: %s\n", strings.Join(result.Prompt.Paths, ", "))
	}
	if result.Prompt.Question != "" {
		fmt.Printf("question: %s\n", result.Prompt.Question)
	}
	if result.Prompt.SelectedOption != nil {
		fmt.Printf("selected: %d. %s\n", result.Prompt.SelectedOption.Number, result.Prompt.SelectedOption.Label)
	}
}

func usage() error {
	fmt.Fprint(os.Stderr, usageText())
	return nil
}

func usageText() string {
	return `tmact minimal POC

Usage:
  tmact detect [--target z_sample-project_sample:0.0] [--lines 120] [--json]
  tmact loop --config examples/night-loop.yaml [--dry-run] [--once] [--assume-idle-on-start]
  tmact watch --config examples/accept-question-watch.yaml [--dry-run] [--once]

Commands:
  detect    capture a tmux pane and detect a directory-access prompt
  loop      run a configurable tmux automation loop
  watch     watch a pane and answer allowlisted prompts
`
}
