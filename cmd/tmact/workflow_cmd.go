package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/leolin310148/tmact/internal/agents"
	"github.com/leolin310148/tmact/internal/runmeta"
	"github.com/leolin310148/tmact/internal/workflow"
)

func runWorkflow(args []string) error {
	if wantsHelp(args) {
		if len(args) > 1 {
			return printCommandHelp("workflow " + strings.Join(args[1:], " "))
		}
		return printCommandHelp("workflow")
	}
	if len(args) == 0 {
		return errors.New("workflow requires a subcommand: discuss, implement, report, example, status, stop")
	}
	switch args[0] {
	case "discuss":
		return runWorkflowDiscuss(args[1:])
	case "implement":
		return runWorkflowImplement(args[1:])
	case "report":
		return runWorkflowReport(args[1:])
	case "example":
		return runWorkflowExample(args[1:])
	case "status":
		return runWorkflowStatus(args[1:])
	case "stop":
		return runRuntimeStop("workflow", args[1:])
	default:
		return fmt.Errorf("unknown workflow subcommand %q", args[0])
	}
}

func runWorkflowReport(args []string) error {
	if wantsHelp(args) {
		if len(args) > 1 {
			return printCommandHelp("workflow report " + strings.Join(args[1:], " "))
		}
		return printCommandHelp("workflow report")
	}
	if len(args) == 0 {
		return errors.New("workflow report requires a subcommand: review, implementation")
	}
	switch args[0] {
	case "review":
		return runWorkflowReportReview(args[1:])
	case "implementation":
		return runWorkflowReportImplementation(args[1:])
	default:
		return fmt.Errorf("unknown workflow report subcommand %q", args[0])
	}
}

func runWorkflowReportReview(args []string) error {
	if wantsHelp(args) {
		return printCommandHelp("workflow report review")
	}
	fs := flag.NewFlagSet("workflow report review", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	configPath := fs.String("config", "", "path to workflow YAML config")
	role := fs.String("role", "", "reporting review role")
	kind := fs.String("kind", "", "report kind: accept, request_changes, reject, withdraw_accept, decision")
	changeHash := fs.String("change-hash", "", "expected OpenSpec artifact hash")
	openspecValid := fs.Bool("openspec-valid", false, "whether OpenSpec validation is passing")
	blocking := fs.Bool("blocking", false, "whether this report blocks the gate")
	replyTo := fs.String("reply-to", "", "comment ID this report replies to")
	body := fs.String("body", "", "short report body")
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
	comment, err := workflow.WriteReviewReport(cfg, workflow.ReviewReport{
		Role:          *role,
		Kind:          *kind,
		ChangeHash:    *changeHash,
		OpenSpecValid: *openspecValid,
		Blocking:      *blocking,
		ReplyTo:       *replyTo,
		Body:          *body,
		Timestamp:     tmactNow(),
	})
	if err != nil {
		return err
	}
	fmt.Printf("review_report: %s\n", comment.ID)
	return nil
}

func runWorkflowReportImplementation(args []string) error {
	if wantsHelp(args) {
		return printCommandHelp("workflow report implementation")
	}
	fs := flag.NewFlagSet("workflow report implementation", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	configPath := fs.String("config", "", "path to implementation workflow YAML config")
	role := fs.String("role", "", "reporting implementation role")
	stage := fs.String("stage", "", "implementation stage: apply, verify, archive")
	kind := fs.String("kind", "", "report kind: complete, pass, fail, request_changes, blocked, decision, withdraw")
	changeHash := fs.String("change-hash", "", "accepted OpenSpec artifact hash")
	blocking := fs.Bool("blocking", false, "whether this report blocks the stage")
	replyTo := fs.String("reply-to", "", "comment ID this report replies to")
	body := fs.String("body", "", "short report body")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *configPath == "" {
		return errors.New("--config is required")
	}
	cfg, err := workflow.LoadImplementationConfig(*configPath)
	if err != nil {
		return err
	}
	comment, err := workflow.WriteImplementationReport(cfg, workflow.ImplementationReport{
		Role:       *role,
		Stage:      *stage,
		Kind:       *kind,
		ChangeHash: *changeHash,
		Blocking:   *blocking,
		ReplyTo:    *replyTo,
		Body:       *body,
		Timestamp:  tmactNow(),
	})
	if err != nil {
		return err
	}
	fmt.Printf("implementation_report: %s\n", comment.ID)
	return nil
}

func runWorkflowDiscuss(args []string) error {
	if wantsHelp(args) {
		return printCommandHelp("workflow discuss")
	}
	fs := flag.NewFlagSet("workflow discuss", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	configPath := fs.String("config", "", "path to workflow YAML config")
	dryRun := fs.Bool("dry-run", false, "print the next prompt without sending to tmux; this is the default")
	execute := fs.Bool("execute", false, "send prompts to tmux panes")
	once := fs.Bool("once", false, "run one observe/validate/gate/prompt pass and exit")
	runDir := fs.String("run-dir", runmeta.DefaultDir, "directory for runtime metadata")

	if err := fs.Parse(args); err != nil {
		return err
	}
	if *configPath == "" {
		return errors.New("--config is required")
	}
	if *dryRun && *execute {
		return errors.New("--dry-run and --execute are mutually exclusive")
	}

	cfg, err := workflow.LoadConfig(*configPath)
	if err != nil {
		return err
	}
	agentCfg, err := agents.LoadConfig(cfg.AgentsConfig)
	if err != nil {
		return err
	}
	bindings, err := workflow.ResolveRoles(cfg, agentCfg)
	if err != nil {
		return err
	}
	if *once {
		runner, err := workflow.NewRunner(cfg, agentCfg, workflow.Options{
			DryRun:     !*execute,
			Once:       true,
			ConfigPath: *configPath,
		})
		if err != nil {
			return err
		}
		return runner.Run(context.Background())
	}
	return runManagedWorkflowRunner(*runDir, *configPath, workflow.TargetSummary(bindings), cfg.LogPath, func(stopRequested func() bool) (managedWorkflowRunner, error) {
		return workflow.NewRunner(cfg, agentCfg, workflow.Options{
			DryRun:        !*execute,
			Once:          false,
			ConfigPath:    *configPath,
			StopRequested: stopRequested,
		})
	})
}

func runWorkflowImplement(args []string) error {
	if wantsHelp(args) {
		return printCommandHelp("workflow implement")
	}
	fs := flag.NewFlagSet("workflow implement", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	configPath := fs.String("config", "", "path to implementation workflow YAML config")
	dryRun := fs.Bool("dry-run", false, "print the next prompt without sending to tmux; this is the default")
	execute := fs.Bool("execute", false, "send prompts to tmux panes")
	once := fs.Bool("once", false, "run one observe/validate/gate/prompt pass and exit")
	runDir := fs.String("run-dir", runmeta.DefaultDir, "directory for runtime metadata")

	if err := fs.Parse(args); err != nil {
		return err
	}
	if *configPath == "" {
		return errors.New("--config is required")
	}
	if *dryRun && *execute {
		return errors.New("--dry-run and --execute are mutually exclusive")
	}

	cfg, err := workflow.LoadImplementationConfig(*configPath)
	if err != nil {
		return err
	}
	agentCfg, err := agents.LoadConfig(cfg.AgentsConfig)
	if err != nil {
		return err
	}
	bindings, err := workflow.ResolveImplementationRoles(cfg, agentCfg)
	if err != nil {
		return err
	}
	if *once {
		runner, err := workflow.NewImplementationRunner(cfg, agentCfg, workflow.Options{
			DryRun:     !*execute,
			Once:       true,
			ConfigPath: *configPath,
		})
		if err != nil {
			return err
		}
		return runner.Run(context.Background())
	}
	return runManagedWorkflowRunner(*runDir, *configPath, workflow.TargetSummary(bindings), cfg.LogPath, func(stopRequested func() bool) (managedWorkflowRunner, error) {
		return workflow.NewImplementationRunner(cfg, agentCfg, workflow.Options{
			DryRun:        !*execute,
			Once:          false,
			ConfigPath:    *configPath,
			StopRequested: stopRequested,
		})
	})
}

func runWorkflowExample(args []string) error {
	if wantsHelp(args) {
		return printCommandHelp("workflow example")
	}
	fs := flag.NewFlagSet("workflow example", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return errors.New("workflow example does not accept positional arguments")
	}
	fmt.Print(workflowExampleYAML)
	return nil
}

func runWorkflowStatus(args []string) error {
	if wantsHelp(args) {
		return printCommandHelp("workflow status")
	}
	fs := flag.NewFlagSet("workflow status", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	runDir := fs.String("run-dir", runmeta.DefaultDir, "directory for runtime metadata")
	configPath := fs.String("config", "", "workflow config path to include local state")
	jsonOutput := fs.Bool("json", false, "print JSON output")

	if err := fs.Parse(args); err != nil {
		return err
	}
	statuses, err := runmeta.List(*runDir, "workflow", tmactNow())
	if err != nil {
		return err
	}
	report := struct {
		Runs                []runmeta.Status              `json:"runs"`
		State               *workflow.State               `json:"state,omitempty"`
		ImplementationState *workflow.ImplementationState `json:"implementation_state,omitempty"`
	}{
		Runs: statuses,
	}
	if *configPath != "" {
		cfg, err := loadAnyWorkflowConfig(*configPath)
		if err != nil {
			return err
		}
		changeDir, err := workflow.ChangeDir(cfg.Change)
		if err != nil {
			return err
		}
		state, err := workflow.LoadState(workflow.StatePath(changeDir))
		if err != nil {
			return err
		}
		report.State = &state
		implementationState, err := workflow.LoadImplementationState(workflow.Phase2StatePath(changeDir))
		if err != nil {
			return err
		}
		report.ImplementationState = &implementationState
	}
	if *jsonOutput {
		return printJSON(report)
	}
	printRuntimeStatuses(statuses)
	if report.State != nil && report.State.Change != "" {
		printWorkflowState(*report.State)
	}
	if report.ImplementationState != nil && report.ImplementationState.Change != "" {
		printImplementationWorkflowState(*report.ImplementationState)
	}
	return nil
}

func loadAnyWorkflowConfig(path string) (workflow.Config, error) {
	cfg, err := workflow.LoadConfig(path)
	if err == nil {
		return cfg, nil
	}
	implementationCfg, implementationErr := workflow.LoadImplementationConfig(path)
	if implementationErr == nil {
		return implementationCfg, nil
	}
	return workflow.Config{}, err
}
