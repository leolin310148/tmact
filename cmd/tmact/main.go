package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"

	"tmact/internal/agents"
	"tmact/internal/loop"
	"tmact/internal/panestatus"
	"tmact/internal/prompt"
	"tmact/internal/tmux"
	"tmact/internal/watch"
	"tmact/internal/workflow"
)

type detectResult struct {
	Target string                  `json:"target"`
	Found  bool                    `json:"found"`
	Prompt *prompt.DirectoryAccess `json:"prompt,omitempty"`
	Error  string                  `json:"error,omitempty"`
}

type repeatedStrings []string

func (r *repeatedStrings) String() string {
	return strings.Join(*r, ",")
}

func (r *repeatedStrings) Set(value string) error {
	if value == "" {
		return errors.New("value cannot be empty")
	}
	*r = append(*r, value)
	return nil
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
	case "inspect":
		return runInspect(args[1:])
	case "status":
		return runStatus(args[1:])
	case "inbox":
		return runInbox(args[1:])
	case "summarize":
		return runSummarize(args[1:])
	case "broadcast":
		return runBroadcast(args[1:])
	case "panels":
		return runPanels(args[1:])
	case "loop":
		return runLoop(args[1:])
	case "watch":
		return runWatch(args[1:])
	case "workflow":
		return runWorkflow(args[1:])
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

func runInspect(args []string) error {
	fs := flag.NewFlagSet("inspect", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	target := fs.String("target", "", "tmux target pane/window to inspect")
	session := fs.String("session", "", "tmux session to inspect")
	window := fs.String("window", "", "tmux window to inspect; combine with --session to avoid ambiguity")
	all := fs.Bool("all", false, "inspect every tmux pane")
	lines := fs.Int("lines", 120, "number of pane history lines to capture")
	samples := fs.Int("sample", 1, "number of captures per pane for idle/running detection")
	interval := fs.Duration("interval", 0, "delay between samples, for example 1s")
	jsonOutput := fs.Bool("json", false, "print JSON output")
	var idleIgnore repeatedStrings
	fs.Var(&idleIgnore, "idle-ignore", "regexp for lines ignored by sample hashing; may be repeated")

	if err := fs.Parse(args); err != nil {
		return err
	}
	if *lines <= 0 {
		return errors.New("--lines must be positive")
	}
	if *samples <= 0 {
		return errors.New("--sample must be positive")
	}
	if *samples == 1 && *interval != 0 {
		return errors.New("--interval is only useful when --sample is greater than 1")
	}
	if *interval < 0 {
		return errors.New("--interval cannot be negative")
	}
	selectors := 0
	for _, selected := range []bool{*target != "", *session != "" || *window != "", *all} {
		if selected {
			selectors++
		}
	}
	if selectors > 1 {
		return errors.New("choose only one selector: --target, --session/--window, or --all")
	}

	report, err := panestatus.Inspect(panestatus.Options{
		Target:             *target,
		Session:            *session,
		Window:             *window,
		All:                *all,
		Lines:              *lines,
		Samples:            *samples,
		Interval:           *interval,
		IdleIgnorePatterns: idleIgnore,
	})
	if err != nil {
		return err
	}
	if *jsonOutput {
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		return encoder.Encode(report)
	}

	printInspectReport(report)
	return nil
}

func runStatus(args []string) error {
	fs := flag.NewFlagSet("status", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	configPath := fs.String("config", "", "path to agent registry YAML config")
	agentName := fs.String("agent", "", "agent name to include")
	role := fs.String("role", "", "role to include")
	jsonOutput := fs.Bool("json", false, "print JSON output")

	if err := fs.Parse(args); err != nil {
		return err
	}

	cfg, err := loadAgentConfig(*configPath)
	if err != nil {
		return err
	}
	cfg, err = agents.FilterConfig(cfg, agents.Filter{Agent: *agentName, Role: *role})
	if err != nil {
		return err
	}
	report := agents.Collect(cfg)
	if *jsonOutput {
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		return encoder.Encode(report)
	}

	printStatusReport(report)
	return nil
}

func runInbox(args []string) error {
	fs := flag.NewFlagSet("inbox", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	configPath := fs.String("config", "", "path to agent registry YAML config")
	agentName := fs.String("agent", "", "agent name to include")
	role := fs.String("role", "", "role to include")
	jsonOutput := fs.Bool("json", false, "print JSON output")

	if err := fs.Parse(args); err != nil {
		return err
	}

	cfg, err := loadAgentConfig(*configPath)
	if err != nil {
		return err
	}
	cfg, err = agents.FilterConfig(cfg, agents.Filter{Agent: *agentName, Role: *role})
	if err != nil {
		return err
	}
	inbox := agents.InboxFromReport(agents.Collect(cfg))
	if *jsonOutput {
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		return encoder.Encode(inbox)
	}

	printInbox(inbox)
	return nil
}

func runSummarize(args []string) error {
	fs := flag.NewFlagSet("summarize", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	configPath := fs.String("config", "", "path to agent registry YAML config")
	agentName := fs.String("agent", "", "agent name to summarize; omit for all agents")
	lines := fs.Int("lines", 12, "number of recent pane lines to include")
	commits := fs.Int("commits", 5, "number of recent git commits to include")
	jsonOutput := fs.Bool("json", false, "print JSON output")

	if err := fs.Parse(args); err != nil {
		return err
	}

	cfg, err := loadAgentConfig(*configPath)
	if err != nil {
		return err
	}
	report, err := agents.Summarize(cfg, *agentName, *lines, *commits)
	if err != nil {
		return err
	}
	if *jsonOutput {
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		return encoder.Encode(report)
	}

	printSummary(report)
	return nil
}

func runBroadcast(args []string) error {
	fs := flag.NewFlagSet("broadcast", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	configPath := fs.String("config", "", "path to agent registry YAML config")
	agentName := fs.String("agent", "", "agent name to send to")
	role := fs.String("role", "", "role to send to")
	all := fs.Bool("all", false, "send to every configured agent")
	text := fs.String("text", "", "text to send")
	enter := fs.Bool("enter", false, "press Enter after sending text")
	execute := fs.Bool("execute", false, "actually send text to tmux; default is dry-run")
	onlyIdle := fs.Bool("only-idle", false, "skip agents that do not appear idle")
	jsonOutput := fs.Bool("json", false, "print JSON output")

	if err := fs.Parse(args); err != nil {
		return err
	}

	cfg, err := loadAgentConfig(*configPath)
	if err != nil {
		return err
	}
	report, err := agents.Broadcast(cfg, agents.BroadcastOptions{
		Agent:    *agentName,
		Role:     *role,
		All:      *all,
		Text:     *text,
		Enter:    *enter,
		Execute:  *execute,
		OnlyIdle: *onlyIdle,
	})
	if err != nil {
		return err
	}
	if *jsonOutput {
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		return encoder.Encode(report)
	}

	printBroadcast(report)
	return nil
}

func runPanels(args []string) error {
	if len(args) == 0 {
		return errors.New("panels requires a subcommand: plan or ensure")
	}
	action := args[0]
	if action != "plan" && action != "ensure" {
		return fmt.Errorf("unknown panels subcommand %q", action)
	}

	fs := flag.NewFlagSet("panels "+action, flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	configPath := fs.String("config", "", "path to agent registry YAML config")
	agentName := fs.String("agent", "", "agent name to include")
	role := fs.String("role", "", "role to include")
	session := fs.String("session", "", "override target tmux session for selected agents")
	execute := fs.Bool("execute", false, "apply planned tmux panel changes")
	jsonOutput := fs.Bool("json", false, "print JSON output")

	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	if action == "plan" && *execute {
		return errors.New("--execute is only valid with panels ensure")
	}

	cfg, err := loadAgentConfig(*configPath)
	if err != nil {
		return err
	}
	cfg, err = agents.FilterConfig(cfg, agents.Filter{Agent: *agentName, Role: *role})
	if err != nil {
		return err
	}

	opts := agents.PanelOptions{Session: *session, Execute: action == "ensure" && *execute}
	var report agents.PanelReport
	if action == "ensure" {
		report, err = agents.EnsurePanels(cfg, opts)
	} else {
		report, err = agents.PlanPanels(cfg, opts)
	}
	if err != nil {
		return err
	}
	if *jsonOutput {
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		return encoder.Encode(report)
	}

	printPanelReport(report)
	return nil
}

func loadAgentConfig(configPath string) (agents.Config, error) {
	resolved, err := agents.ResolveConfigPath(configPath)
	if err != nil {
		return agents.Config{}, err
	}
	return agents.LoadConfig(resolved)
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

func runWorkflow(args []string) error {
	fs := flag.NewFlagSet("workflow", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	configPath := fs.String("config", "", "path to workflow YAML config")
	dryRun := fs.Bool("dry-run", false, "print workflow actions without sending anything to tmux")
	once := fs.Bool("once", false, "run one workflow observe/action pass and exit")
	assumeIdleOnStart := fs.Bool("assume-idle-on-start", false, "treat the pane as already idle when the workflow starts")
	startStage := fs.String("start-stage", "", "start from the named workflow stage")

	if err := fs.Parse(args); err != nil {
		return err
	}
	if *configPath == "" {
		return errors.New("--config is required")
	}

	cfg, err := workflow.LoadConfig(*configPath)
	if err != nil {
		return err
	}

	runner := workflow.NewRunner(cfg, workflow.Options{
		DryRun:            *dryRun,
		Once:              *once,
		AssumeIdleOnStart: *assumeIdleOnStart,
		StartStage:        *startStage,
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

func printStatusReport(report agents.Report) {
	fmt.Printf("ts: %s\n", report.Timestamp)
	for _, agent := range report.Agents {
		fmt.Printf("%s\t%s\t%s", agent.Name, agent.Target, agent.State)
		if agent.Git != nil {
			dirty := "clean"
			if agent.Git.Dirty {
				dirty = "dirty"
			}
			if agent.Git.Error != "" {
				fmt.Printf("\tgit:%s", agent.Git.Error)
			} else if agent.Git.Branch != "" {
				fmt.Printf("\tgit:%s/%s", agent.Git.Branch, dirty)
			} else {
				fmt.Printf("\tgit:%s", dirty)
			}
		}
		if agent.LastLine != "" {
			fmt.Printf("\t%s", agent.LastLine)
		}
		if agent.Error != "" {
			fmt.Printf("\terror:%s", agent.Error)
		}
		fmt.Println()
	}
}

func printInspectReport(report panestatus.Report) {
	fmt.Printf("ts: %s\n", report.Timestamp)
	for _, pane := range report.Panes {
		fmt.Printf("%s\t%s\t%s\tidle:%t", pane.Target, pane.Runtime, pane.State, pane.Idle)
		if pane.CurrentCommand != "" {
			fmt.Printf("\tcmd:%s", pane.CurrentCommand)
		}
		if pane.CWD != "" {
			fmt.Printf("\tcwd:%s", pane.CWD)
		}
		if pane.LastLine != "" {
			fmt.Printf("\t%s", pane.LastLine)
		}
		if pane.Error != "" {
			fmt.Printf("\terror:%s", pane.Error)
		}
		fmt.Println()
	}
}

func printInbox(inbox agents.Inbox) {
	fmt.Printf("ts: %s\n", inbox.Timestamp)
	if len(inbox.Items) == 0 {
		fmt.Println("inbox: empty")
		return
	}
	for _, item := range inbox.Items {
		fmt.Printf("%s\t%s\t%s\t%s", item.Agent, item.Target, item.Kind, item.Severity)
		if item.Summary != "" {
			fmt.Printf("\t%s", item.Summary)
		}
		fmt.Println()
	}
}

func printSummary(report agents.SummaryReport) {
	fmt.Printf("ts: %s\n", report.Timestamp)
	for _, summary := range report.Summaries {
		fmt.Printf("\n%s\t%s\t%s\n", summary.Name, summary.Target, summary.State)
		if summary.Role != "" || summary.Type != "" {
			fmt.Printf("role: %s\ttype: %s\n", summary.Role, summary.Type)
		}
		if summary.Git != nil {
			printGitSummary(summary.Git)
		}
		if summary.Error != "" {
			fmt.Printf("error: %s\n", summary.Error)
		}
		if summary.NextAction != "" {
			fmt.Printf("next: %s\n", summary.NextAction)
		}
		if len(summary.LastLines) > 0 {
			fmt.Println("recent:")
			for _, line := range summary.LastLines {
				fmt.Printf("  %s\n", line)
			}
		}
	}
}

func printGitSummary(git *agents.GitSummary) {
	if git.Error != "" {
		fmt.Printf("git: %s\n", git.Error)
		return
	}
	dirty := "clean"
	if git.Dirty {
		dirty = "dirty"
	}
	if git.Branch != "" {
		fmt.Printf("git: %s/%s\n", git.Branch, dirty)
	} else {
		fmt.Printf("git: %s\n", dirty)
	}
	if len(git.ChangedFiles) > 0 {
		fmt.Printf("changed: %d files\n", len(git.ChangedFiles))
	}
	if len(git.RecentCommits) > 0 {
		fmt.Println("commits:")
		for _, commit := range git.RecentCommits {
			fmt.Printf("  %s %s\n", commit.Hash, commit.Subject)
		}
	}
}

func printBroadcast(report agents.BroadcastReport) {
	fmt.Printf("ts: %s\n", report.Timestamp)
	if report.DryRun {
		fmt.Println("mode: dry-run")
	} else {
		fmt.Println("mode: execute")
	}
	for _, result := range report.Results {
		fmt.Printf("%s\t%s\t%s", result.Agent, result.Target, result.Status)
		if result.State != "" {
			fmt.Printf("\tstate:%s", result.State)
		}
		if result.Reason != "" {
			fmt.Printf("\treason:%s", result.Reason)
		}
		if result.Error != "" {
			fmt.Printf("\terror:%s", result.Error)
		}
		fmt.Println()
	}
}

func printPanelReport(report agents.PanelReport) {
	fmt.Printf("ts: %s\n", report.Timestamp)
	if report.DryRun {
		fmt.Println("mode: dry-run")
	} else {
		fmt.Println("mode: execute")
	}
	for _, op := range report.Operations {
		fmt.Printf("%s\t%s\t%s\t%s", op.Agent, op.Action, op.Target, op.Status)
		if len(op.Command) > 0 {
			fmt.Printf("\tcmd:%s", strings.Join(op.Command, " "))
		}
		if op.Error != "" {
			fmt.Printf("\terror:%s", op.Error)
		}
		fmt.Println()
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
  tmact inspect [--target z_sample-project:0.0 | --session z_sample-project | --all] [--sample 2 --interval 1s] [--json]
  tmact status [--config examples/agents.yaml] [--agent z-sample-project] [--role library-maintenance] [--json]
  tmact inbox [--config examples/agents.yaml] [--agent z-sample-project] [--role library-maintenance] [--json]
  tmact summarize [--config examples/agents.yaml] [--agent z-sample-project] [--json]
  tmact broadcast [--config examples/agents.yaml] --agent z-sample-project --text "summarize progress" [--enter] [--execute]
  tmact panels plan [--config examples/agents.yaml] [--session IDLL] [--json]
  tmact panels ensure [--config examples/agents.yaml] [--session IDLL] [--execute]
  tmact loop --config examples/night-loop.yaml [--dry-run] [--once] [--assume-idle-on-start]
  tmact watch --config examples/accept-question-watch.yaml [--dry-run] [--once]
  tmact workflow --config examples/simple-improvement-workflow.yaml [--dry-run] [--once] [--assume-idle-on-start] [--start-stage name]

Commands:
  detect    capture a tmux pane and detect a directory-access prompt
  inspect   detect runtime and idle/running state for tmux panes
  status    summarize configured agent panes
  inbox     list agent panes that need human intervention
  summarize summarize recent pane and git activity
  broadcast safely send text to selected agent panes
  panels    plan or ensure configured agent tmux panels
  loop      run a configurable tmux automation loop
  watch     watch a pane and answer allowlisted prompts
  workflow  run a staged prompt workflow such as agent-inbox feature work
`
}
