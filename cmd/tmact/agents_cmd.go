package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"

	"github.com/leolin310148/tmact/internal/agents"
)

func runStatus(args []string) error {
	if wantsHelp(args) {
		return printCommandHelp("status")
	}
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
	if wantsHelp(args) {
		return printCommandHelp("inbox")
	}
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
	if wantsHelp(args) {
		return printCommandHelp("summarize")
	}
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
	if wantsHelp(args) {
		return printCommandHelp("broadcast")
	}
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
	if wantsHelp(args) {
		return printCommandHelp("panels")
	}
	if len(args) == 0 {
		return errors.New("panels requires a subcommand: plan or ensure")
	}
	action := args[0]
	if action != "plan" && action != "ensure" {
		return fmt.Errorf("unknown panels subcommand %q", action)
	}
	if wantsHelp(args[1:]) {
		return printCommandHelp("panels " + action)
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
