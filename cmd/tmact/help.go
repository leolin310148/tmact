package main

import (
	"fmt"
	"os"
	"strings"
	"text/tabwriter"
)

type helpFlag struct {
	Name        string `json:"name"`
	Value       string `json:"value,omitempty"`
	Description string `json:"description"`
	Required    bool   `json:"required,omitempty"`
}

type commandHelp struct {
	Command     string     `json:"command"`
	Summary     string     `json:"summary"`
	Usage       []string   `json:"usage,omitempty"`
	Subcommands []string   `json:"subcommands,omitempty"`
	Flags       []helpFlag `json:"flags,omitempty"`
	Examples    []string   `json:"examples,omitempty"`
	Safety      []string   `json:"safety,omitempty"`
	Notes       []string   `json:"notes,omitempty"`
}

type helpManifest struct {
	Name        string        `json:"name"`
	Summary     string        `json:"summary"`
	GlobalFlags []helpFlag    `json:"global_flags,omitempty"`
	Commands    []commandHelp `json:"commands"`
}

func runHelp(args []string) error {
	jsonOutput := false
	filtered := make([]string, 0, len(args))
	for _, arg := range args {
		if arg == "--json" {
			jsonOutput = true
			continue
		}
		filtered = append(filtered, arg)
	}
	if len(filtered) == 0 || wantsHelp(filtered) {
		if jsonOutput {
			return printJSON(commandManifest())
		}
		return usage()
	}
	name := strings.Join(filtered, " ")
	if jsonOutput {
		help, ok := commandHelpFor(name)
		if !ok {
			return fmt.Errorf("unknown help topic %q", name)
		}
		return printJSON(help)
	}
	return printCommandHelp(name)
}

func usage() error {
	fmt.Print(usageText())
	return nil
}

func usageText() string {
	return `tmact - local tmux automation for agent panes

Usage:
  tmact ls [--json]
  tmact -t 0 send --command "go test ./..." [--execute]
  tmact -t 0 send --text "summarize progress" [--enter] [--execute]
  tmact -t 0 send --key Enter [--execute]
  tmact -t 0 send --keys C-u,Enter [--execute]
  tmact detect [--target sample:0.0] [--lines 120] [--json]
  tmact inspect [--target sample:0.0 | --session sample | --all] [--sample 2 --interval 1s] [--json]
  tmact status [--config examples/agents.yaml] [--agent sample-codex] [--role maintenance] [--json]
  tmact statusd start|once|read|status [--state-path /tmp/tmact-status.json]
  tmact stt-set --provider openai --api-key KEY [--model gpt-4o-transcribe]
  tmact inbox [--config examples/agents.yaml] [--agent sample-codex] [--role maintenance] [--json]
  tmact summarize [--config examples/agents.yaml] [--agent sample-codex] [--json]
  tmact broadcast [--config examples/agents.yaml] --agent sample-codex --text "summarize progress" [--enter] [--execute]
  tmact panels plan [--config examples/agents.yaml] [--session sample-team] [--json]
  tmact panels ensure [--config examples/agents.yaml] [--session sample-team] [--execute]
  tmact loop --config examples/night-loop.yaml [--dry-run] [--once] [--assume-idle-on-start]
  tmact loop status [--run-dir .tmact/runs] [--json]
  tmact loop stop (--id ID | --config path)
  tmact workflow discuss --config examples/openspec-workflow.yaml [--dry-run] [--once] [--execute]
  tmact workflow implement --config examples/openspec-implementation.yaml [--dry-run] [--once] [--execute]
  tmact workflow report review --config examples/openspec-workflow.yaml --role qa --kind accept --change-hash sha256:...
  tmact workflow example
  tmact workflow status [--config examples/openspec-workflow.yaml] [--json]
  tmact workflow stop (--id ID | --config path)
  tmact watch --config examples/accept-question-watch.yaml [--dry-run] [--once]
  tmact dispatch-work SESSION --dir DIR --agent claude --prompt "..." [--ready-timeout 30s] [--ready-settle 1.5s] [--execute]
  tmact help [command] [--json]
  tmact commands [--json]
  tmact version [--json]

Commands:
  ls        list tmux panes and cache numbered targets for -t
  send      send text, a command, or keys to a selected tmux target
  detect    capture a tmux pane and detect a directory-access prompt
  inspect   detect runtime and idle/running state for tmux panes
  status    summarize configured agent panes
  statusd   maintain a cached tmux pane status snapshot
  stt-set   configure statusd web UI voice transcription
  inbox     list agent panes that need human intervention
  summarize summarize recent pane and git activity
  broadcast safely send text to selected agent panes
  panels    plan or ensure configured agent tmux panels
  loop      run, inspect, or stop a configurable tmux automation loop
  workflow  run, inspect, or stop serialized OpenSpec review and implementation workflows
  watch     watch a pane and answer allowlisted prompts
  dispatch-work create or reuse a session, launch an agent, and send it a prompt
  commands  print a machine-readable command catalog for tools and LLMs
  version   print the tmact build version

Safety:
  send, broadcast, and panels ensure default to dry-run. For loop and watch,
  validate with --dry-run --once before running a live automation.

More help:
  tmact help loop
  tmact help loop status
  tmact commands --json
`
}

const workflowExampleYAML = `change: your-change-id
agents_config: examples/openspec-workflow-agents.yaml
roles:
  pm: pm-agent
  swe: swe-agent
  qa: qa-agent
  reviewer: reviewer-agent
prompt_dispatch:
  clear_before_prompt: true
  clear_command: /clear
  clear_delay: 5s
  legacy_marker_fallback: false
discussion:
  role_order: [pm, swe, qa, reviewer]
  max_turns: 24
  max_runtime: 8h
  poll_interval: 15s
  idle_after: 30s
  capture_lines: 180
  create_missing_proposal: false
implementation:
  stage_order: [swe_apply, qa_verify, pm_archive]
  max_turns: 12
  max_runtime: 8h
  poll_interval: 15s
  idle_after: 30s
  capture_lines: 180
  require_phase1_agreed: true
  allow_dry_run_without_phase1: true
  apply_instructions:
    command: openspec
    args: ["instructions", "apply", "--change", "{{change}}"]
  verify_commands:
    - command: openspec
      args: ["validate", "{{change}}", "--strict"]
    - command: go
      args: ["test", "./..."]
  archive_command:
    command: openspec
    args: ["archive", "{{change}}", "--yes"]
log_path: .tmact/openspec-full-workflow.jsonl
`

func printCommandHelp(name string) error {
	help, ok := commandHelpFor(name)
	if !ok {
		return fmt.Errorf("unknown help topic %q", name)
	}
	fmt.Printf("%s\n\n%s\n", help.Command, help.Summary)
	if len(help.Usage) > 0 {
		fmt.Println("\nUsage:")
		for _, usage := range help.Usage {
			fmt.Printf("  %s\n", usage)
		}
	}
	if len(help.Subcommands) > 0 {
		fmt.Println("\nSubcommands:")
		for _, subcommand := range help.Subcommands {
			fmt.Printf("  %s\n", subcommand)
		}
	}
	if len(help.Flags) > 0 {
		fmt.Println("\nFlags:")
		writer := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		for _, flag := range help.Flags {
			name := flag.Name
			if flag.Value != "" {
				name += " " + flag.Value
			}
			required := ""
			if flag.Required {
				required = " required"
			}
			fmt.Fprintf(writer, "  %s\t%s%s\n", name, flag.Description, required)
		}
		_ = writer.Flush()
	}
	if len(help.Examples) > 0 {
		fmt.Println("\nExamples:")
		for _, example := range help.Examples {
			fmt.Printf("  %s\n", example)
		}
	}
	if len(help.Safety) > 0 {
		fmt.Println("\nSafety:")
		for _, note := range help.Safety {
			fmt.Printf("  %s\n", note)
		}
	}
	if len(help.Notes) > 0 {
		fmt.Println("\nNotes:")
		for _, note := range help.Notes {
			fmt.Printf("  %s\n", note)
		}
	}
	return nil
}

func printCommandTable(commands []commandHelp) {
	writer := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(writer, "command\tsummary")
	for _, command := range commands {
		if strings.Contains(command.Command, " ") {
			continue
		}
		fmt.Fprintf(writer, "%s\t%s\n", command.Command, command.Summary)
	}
	_ = writer.Flush()
}

func commandHelpFor(name string) (commandHelp, bool) {
	normalized := strings.Join(strings.Fields(name), " ")
	for _, help := range commandHelpCatalog() {
		if help.Command == normalized {
			return help, true
		}
	}
	return commandHelp{}, false
}

func commandManifest() helpManifest {
	return helpManifest{
		Name:    "tmact",
		Summary: "Local tmux automation CLI for inspecting panes, sending guarded input, and running loop daemons.",
		GlobalFlags: []helpFlag{
			{Name: "-t, --target", Value: "TARGET", Description: "target selector for send; may be a tmux target or a numbered index from tmact ls"},
		},
		Commands: commandHelpCatalog(),
	}
}

func commandHelpCatalog() []commandHelp {
	return []commandHelp{
		{
			Command: "ls",
			Summary: "List tmux panes and refresh the numbered target cache used by -t.",
			Usage:   []string{"tmact ls [--json]"},
			Flags: []helpFlag{
				{Name: "--json", Description: "print JSON output"},
			},
			Examples: []string{"tmact ls", "tmact ls --json"},
			Notes:    []string{"Run this before using a numeric target such as -t 0."},
		},
		{
			Command: "send",
			Summary: "Send text, a command, or tmux keys to one selected pane.",
			Usage: []string{
				`tmact -t TARGET send --text TEXT [--enter] [--clear-line] [--execute]`,
				`tmact -t TARGET send --command COMMAND [--clear-line] [--execute]`,
				`tmact -t TARGET send --key KEY [--key KEY...] [--execute]`,
				`tmact -t TARGET send --keys C-u,Enter [--execute]`,
			},
			Flags: []helpFlag{
				{Name: "--text", Value: "TEXT", Description: "text to paste without Enter unless --enter is set"},
				{Name: "--command", Value: "COMMAND", Description: "command to paste followed by Enter"},
				{Name: "--key", Value: "KEY", Description: "tmux key to send; may be repeated"},
				{Name: "--keys", Value: "CSV", Description: "comma-separated tmux keys"},
				{Name: "--enter", Description: "press Enter after --text"},
				{Name: "--clear-line", Description: "send C-u before text or command"},
				{Name: "--execute", Description: "actually send to tmux; default is dry-run"},
				{Name: "--json", Description: "print JSON output"},
			},
			Examples: []string{
				`tmact ls`,
				`tmact -t 0 send --command "go test ./..."`,
				`tmact -t work:0.0 send --text "summarize progress" --enter --execute`,
			},
			Safety: []string{"Without --execute this prints the planned send and does not touch tmux."},
		},
		{
			Command: "dispatch-work",
			Summary: "Create or reuse a tmux session, launch an agent, and send it a prompt.",
			Usage: []string{
				"tmact dispatch-work SESSION --dir DIR --agent claude|codex|gemini|copilot --prompt TEXT [--ready-timeout 30s] [--ready-settle 1.5s] [--execute] [--json]",
			},
			Flags: []helpFlag{
				{Name: "--dir", Value: "DIR", Description: "working directory; sets cwd when the session is created", Required: true},
				{Name: "--agent", Value: "NAME", Description: "agent to launch: claude, codex, gemini, or copilot", Required: true},
				{Name: "--prompt", Value: "TEXT", Description: "prompt text sent to the agent followed by Enter", Required: true},
				{Name: "--ready-timeout", Value: "DURATION", Description: "max wait for the agent to become ready before sending"},
				{Name: "--ready-settle", Value: "DURATION", Description: "stable idle time after ready before sending the prompt"},
				{Name: "--execute", Description: "actually create, launch, and send; default is dry-run"},
				{Name: "--json", Description: "print JSON output"},
			},
			Examples: []string{
				`tmact dispatch-work work --dir . --agent claude --prompt "review the diff"`,
				`tmact dispatch-work work --dir ~/proj --agent claude --prompt "run the tests" --execute`,
			},
			Safety: []string{
				"Without --execute this prints the plan and does not touch tmux.",
				"Fails if the session already runs a different agent or the agent is busy working.",
				"Refuses to auto-confirm trust or permission prompts shown during agent startup.",
			},
			Notes: []string{
				"The session name is the first positional argument.",
				"A new session starts a shell and launches the agent into it, so quitting the agent drops back to a shell instead of closing the session.",
				"Reusing a session that already runs the agent sends /clear before the prompt.",
			},
		},
		{
			Command: "detect",
			Summary: "Capture a pane and detect a directory-access prompt.",
			Usage:   []string{"tmact detect [--target TARGET] [--lines 120] [--json]"},
			Flags: []helpFlag{
				{Name: "--target", Value: "TARGET", Description: "tmux target pane/window/session to capture"},
				{Name: "--lines", Value: "N", Description: "number of pane history lines to capture"},
				{Name: "--json", Description: "print JSON output"},
			},
			Examples: []string{"tmact detect --target session:0.0 --json"},
		},
		{
			Command: "inspect",
			Summary: "Classify tmux panes by runtime and idle/running/asking state.",
			Usage:   []string{"tmact inspect [--target TARGET | --session SESSION [--window WINDOW] | --all] [--sample 2 --interval 1s] [--json]"},
			Flags: []helpFlag{
				{Name: "--target", Value: "TARGET", Description: "tmux target pane/window to inspect"},
				{Name: "--session", Value: "SESSION", Description: "tmux session to inspect"},
				{Name: "--window", Value: "WINDOW", Description: "tmux window to inspect; combine with --session to avoid ambiguity"},
				{Name: "--all", Description: "inspect every tmux pane"},
				{Name: "--lines", Value: "N", Description: "number of pane history lines to capture"},
				{Name: "--sample", Value: "N", Description: "number of captures per pane for idle/running detection"},
				{Name: "--interval", Value: "DURATION", Description: "delay between samples, for example 1s"},
				{Name: "--idle-ignore", Value: "REGEXP", Description: "line regexp ignored by sample hashing; may be repeated"},
				{Name: "--json", Description: "print JSON output"},
			},
			Examples: []string{"tmact inspect --all", "tmact inspect --target session:0.0 --sample 2 --interval 1s --json"},
			Notes:    []string{"This inspects tmux panes. Use tmact loop status to inspect registered loop daemons."},
		},
		{
			Command: "status",
			Summary: "Summarize configured agent panes from agents.yaml.",
			Usage:   []string{"tmact status [--config examples/agents.yaml] [--agent NAME] [--role ROLE] [--json]"},
			Flags: []helpFlag{
				{Name: "--config", Value: "PATH", Description: "path to agent registry YAML config"},
				{Name: "--agent", Value: "NAME", Description: "agent name to include"},
				{Name: "--role", Value: "ROLE", Description: "role to include"},
				{Name: "--json", Description: "print JSON output"},
			},
			Examples: []string{"tmact status --config examples/agents.yaml", "tmact status --agent sample-codex --json"},
		},
		{
			Command:     "statusd",
			Summary:     "Maintain or read a cached tmux pane status snapshot.",
			Usage:       []string{"tmact statusd start|once|read|status [flags]"},
			Subcommands: []string{"start", "once", "read", "status"},
			Examples:    []string{"tmact statusd once --json", "tmact statusd start --interval 1s --state-path /tmp/tmact-status.json"},
			Notes:       []string{"Use tmact help statusd start for daemon flags."},
		},
		{
			Command:  "statusd start",
			Summary:  "Run the pane status daemon until interrupted.",
			Usage:    []string{"tmact statusd start [--interval 1s] [--state-path PATH] [--no-tmux-options] [--web-addr ADDR]"},
			Flags:    statusdStartHelpFlags(),
			Examples: []string{"tmact statusd start --interval 1s", "tmact statusd start --once --json", "tmact statusd start --web-addr 0.0.0.0:7890"},
		},
		{
			Command:  "statusd once",
			Summary:  "Run one statusd scan and exit.",
			Usage:    []string{"tmact statusd once [--json] [--state-path PATH] [--initial-samples 2]"},
			Flags:    statusdHelpFlags(),
			Examples: []string{"tmact statusd once", "tmact statusd once --json"},
		},
		{
			Command: "statusd read",
			Summary: "Read the latest statusd JSON snapshot from disk.",
			Usage:   []string{"tmact statusd read [--json] [--state-path PATH]"},
			Flags: []helpFlag{
				{Name: "--state-path", Value: "PATH", Description: "latest JSON snapshot path"},
				{Name: "--json", Description: "print JSON output"},
			},
			Examples: []string{"tmact statusd read --state-path /tmp/tmact-status.json"},
		},
		{
			Command: "statusd status",
			Summary: "Print statusd snapshot freshness and summary counts.",
			Usage:   []string{"tmact statusd status [--json] [--state-path PATH]"},
			Flags: []helpFlag{
				{Name: "--state-path", Value: "PATH", Description: "latest JSON snapshot path"},
				{Name: "--json", Description: "print JSON output"},
			},
			Examples: []string{"tmact statusd status"},
		},
		{
			Command: "stt-set",
			Summary: "Configure the statusd web UI speech-to-text provider.",
			Usage:   []string{"tmact stt-set --provider openai --api-key KEY [--model gpt-4o-transcribe] [--endpoint URL]"},
			Flags: []helpFlag{
				{Name: "--provider", Value: "NAME", Description: "speech-to-text provider; currently openai"},
				{Name: "--api-key", Value: "KEY", Description: "provider API key stored in ~/.tmact/stt_provider.json", Required: true},
				{Name: "--model", Value: "MODEL", Description: "speech-to-text model"},
				{Name: "--endpoint", Value: "URL", Description: "transcription API endpoint"},
				{Name: "--config", Value: "PATH", Description: "provider config path"},
				{Name: "--json", Description: "print JSON output without the API key"},
			},
			Examples: []string{"tmact stt-set --provider openai --api-key sk-...", "tmact stt-set --provider openai --api-key sk-... --model whisper-1"},
			Notes:    []string{"The config file is written with 0600 permissions and the API key is not printed."},
		},
		{
			Command:  "inbox",
			Summary:  "List configured agent panes that need human intervention.",
			Usage:    []string{"tmact inbox [--config examples/agents.yaml] [--agent NAME] [--role ROLE] [--json]"},
			Flags:    agentFilterHelpFlags(),
			Examples: []string{"tmact inbox", "tmact inbox --role library-maintenance --json"},
		},
		{
			Command: "summarize",
			Summary: "Summarize recent pane output and git activity for configured agents.",
			Usage:   []string{"tmact summarize [--config examples/agents.yaml] [--agent NAME] [--lines 12] [--commits 5] [--json]"},
			Flags: append(agentSummaryHelpFlags(),
				helpFlag{Name: "--lines", Value: "N", Description: "number of recent pane lines to include"},
				helpFlag{Name: "--commits", Value: "N", Description: "number of recent git commits to include"},
			),
			Examples: []string{"tmact summarize --agent sample-codex", "tmact summarize --json"},
		},
		{
			Command: "broadcast",
			Summary: "Safely send text to selected configured agent panes.",
			Usage:   []string{`tmact broadcast [--config examples/agents.yaml] (--agent NAME | --role ROLE | --all) --text TEXT [--enter] [--only-idle] [--execute] [--json]`},
			Flags: append(agentFilterHelpFlags(),
				helpFlag{Name: "--all", Description: "send to every configured agent"},
				helpFlag{Name: "--text", Value: "TEXT", Description: "text to send", Required: true},
				helpFlag{Name: "--enter", Description: "press Enter after sending text"},
				helpFlag{Name: "--only-idle", Description: "skip agents that do not appear idle"},
				helpFlag{Name: "--execute", Description: "actually send text to tmux; default is dry-run"},
			),
			Examples: []string{`tmact broadcast --agent sample-codex --text "summarize progress"`, `tmact broadcast --all --text "status?" --enter --only-idle --execute`},
			Safety:   []string{"Without --execute this prints the planned sends and does not touch tmux."},
		},
		{
			Command:     "panels",
			Summary:     "Plan or reconcile configured agent panes in tmux.",
			Usage:       []string{"tmact panels plan [flags]", "tmact panels ensure [flags]"},
			Subcommands: []string{"plan", "ensure"},
			Examples:    []string{"tmact panels plan --config examples/agents.yaml", "tmact panels ensure --session sample-team --execute"},
			Safety:      []string{"panels plan never changes tmux. panels ensure requires --execute before it applies changes."},
		},
		{
			Command: "panels plan",
			Summary: "Print the tmux panel operations that would be needed.",
			Usage:   []string{"tmact panels plan [--config examples/agents.yaml] [--agent NAME] [--role ROLE] [--session SESSION] [--json]"},
			Flags: append(agentFilterHelpFlags(),
				helpFlag{Name: "--session", Value: "SESSION", Description: "override target tmux session for selected agents"},
			),
			Examples: []string{"tmact panels plan --json"},
		},
		{
			Command: "panels ensure",
			Summary: "Reconcile configured tmux panes, optionally executing the plan.",
			Usage:   []string{"tmact panels ensure [--config examples/agents.yaml] [--agent NAME] [--role ROLE] [--session SESSION] [--execute] [--json]"},
			Flags: append(agentFilterHelpFlags(),
				helpFlag{Name: "--session", Value: "SESSION", Description: "override target tmux session for selected agents"},
				helpFlag{Name: "--execute", Description: "apply planned tmux panel changes"},
			),
			Examples: []string{"tmact panels ensure --session sample-team", "tmact panels ensure --session sample-team --execute"},
			Safety:   []string{"Without --execute this prints the planned changes and does not touch tmux."},
		},
		{
			Command:     "loop",
			Summary:     "Run, inspect, or stop a configurable single-pane automation loop.",
			Usage:       []string{"tmact loop --config PATH [--dry-run] [--once] [--assume-idle-on-start]", "tmact loop status [--run-dir .tmact/runs] [--json]", "tmact loop stop (--id ID | --config PATH)"},
			Subcommands: []string{"status", "stop"},
			Flags: []helpFlag{
				{Name: "--config", Value: "PATH", Description: "path to loop YAML config", Required: true},
				{Name: "--dry-run", Description: "print actions without sending anything to tmux"},
				{Name: "--once", Description: "run one observe/action pass and exit"},
				{Name: "--assume-idle-on-start", Description: "treat the pane as already idle when the loop starts"},
				{Name: "--run-dir", Value: "PATH", Description: "directory for runtime metadata"},
			},
			Examples: []string{"tmact loop --config examples/night-loop.yaml --dry-run --once", "tmact loop status", "tmact loop stop --config examples/night-loop.yaml"},
			Safety:   []string{"Loops should stop on permission prompts rather than auto-confirming them. Validate with --dry-run --once first."},
			Notes:    []string{"Long-running loop metadata is stored under .tmact/runs by default."},
		},
		runtimeStatusHelp("loop"),
		runtimeStopHelp("loop"),
		{
			Command:     "workflow",
			Summary:     "Run, inspect, or stop serialized OpenSpec review and implementation workflows.",
			Usage:       []string{"tmact workflow example", "tmact workflow discuss --config PATH [--dry-run] [--once] [--execute]", "tmact workflow implement --config PATH [--dry-run] [--once] [--execute]", "tmact workflow report review --config PATH --role ROLE --kind KIND --change-hash HASH", "tmact workflow report implementation --config PATH --role ROLE --stage STAGE --kind KIND --change-hash HASH", "tmact workflow status [--config PATH] [--run-dir .tmact/runs] [--json]", "tmact workflow stop (--id ID | --config PATH)"},
			Subcommands: []string{"example", "discuss", "implement", "report", "status", "stop"},
			Examples:    []string{"tmact workflow example", "tmact workflow discuss --config examples/openspec-full-workflow.yaml --dry-run --once", "tmact workflow implement --config examples/openspec-full-workflow.yaml --dry-run --once", "tmact workflow report review --config examples/openspec-full-workflow.yaml --role qa --kind accept --change-hash sha256:abc --openspec-valid", "tmact workflow status --config examples/openspec-full-workflow.yaml", "tmact workflow stop --config examples/openspec-full-workflow.yaml"},
			Safety:      []string{"Workflow prompts are dry-run by default. Use --execute only after inspecting the planned prompt and target roles."},
			Notes:       []string{"Discussion uses serialized PM -> SWE -> QA -> reviewer review. Implementation uses SWE apply -> QA verify -> PM archive."},
		},
		{
			Command:  "workflow example",
			Summary:  "Print a combined OpenSpec workflow YAML example.",
			Usage:    []string{"tmact workflow example"},
			Examples: []string{"tmact workflow example > examples/openspec-full-workflow.yaml"},
			Notes:    []string{"The output includes both discussion and implementation sections, so the same config can drive both phases."},
		},
		{
			Command: "workflow discuss",
			Summary: "Run one or more serialized OpenSpec artifact review passes.",
			Usage:   []string{"tmact workflow discuss --config PATH [--dry-run] [--once] [--execute]"},
			Flags: []helpFlag{
				{Name: "--config", Value: "PATH", Description: "path to workflow YAML config", Required: true},
				{Name: "--dry-run", Description: "print planned prompts without sending to tmux; default behavior"},
				{Name: "--execute", Description: "send prompts to configured tmux panes"},
				{Name: "--once", Description: "run one observe/validate/gate/prompt pass and exit"},
				{Name: "--run-dir", Value: "PATH", Description: "directory for runtime metadata"},
			},
			Examples: []string{"tmact workflow discuss --config examples/openspec-workflow.yaml --dry-run --once", "tmact workflow discuss --config examples/openspec-workflow.yaml --execute"},
			Safety:   []string{"The workflow stops on permission prompts and does not auto-approve tools or filesystem access."},
		},
		{
			Command: "workflow implement",
			Summary: "Run one or more serialized OpenSpec implementation passes.",
			Usage:   []string{"tmact workflow implement --config PATH [--dry-run] [--once] [--execute]"},
			Flags: []helpFlag{
				{Name: "--config", Value: "PATH", Description: "path to implementation workflow YAML config", Required: true},
				{Name: "--dry-run", Description: "print planned prompts without sending to tmux; default behavior"},
				{Name: "--execute", Description: "send prompts to configured tmux panes"},
				{Name: "--once", Description: "run one observe/validate/gate/prompt pass and exit"},
				{Name: "--run-dir", Value: "PATH", Description: "directory for runtime metadata"},
			},
			Examples: []string{"tmact workflow implement --config examples/openspec-implementation.yaml --dry-run --once", "tmact workflow implement --config examples/openspec-implementation.yaml --execute"},
			Safety:   []string{"The implementation workflow requires phase 1 agreement before live execution and does not auto-approve tools or archive prompts."},
		},
		{
			Command:     "workflow report",
			Summary:     "Record workflow progress through durable JSONL reports.",
			Usage:       []string{"tmact workflow report review --config PATH --role ROLE --kind KIND --change-hash HASH [--openspec-valid] [--blocking=true|false] [--reply-to ID] [--body TEXT]", "tmact workflow report implementation --config PATH --role ROLE --stage STAGE --kind KIND --change-hash HASH [--blocking=true|false] [--reply-to ID] [--body TEXT]"},
			Subcommands: []string{"review", "implementation"},
			Examples:    []string{"tmact workflow report review --config examples/openspec-full-workflow.yaml --role pm --kind accept --change-hash sha256:abc --openspec-valid --body \"accepted current artifacts\"", "tmact workflow report implementation --config examples/openspec-full-workflow.yaml --role qa --stage verify --kind pass --change-hash sha256:abc --body \"tests passed\""},
			Safety:      []string{"Reports only write workflow state for the configured OpenSpec change and do not send tmux input."},
		},
		{
			Command: "workflow report review",
			Summary: "Append a phase 1 OpenSpec review report.",
			Usage:   []string{"tmact workflow report review --config PATH --role ROLE --kind accept|request_changes|reject|withdraw_accept|decision --change-hash HASH [--openspec-valid] [--blocking=true|false] [--reply-to ID] [--body TEXT]"},
			Flags: []helpFlag{
				{Name: "--config", Value: "PATH", Description: "path to workflow YAML config", Required: true},
				{Name: "--role", Value: "ROLE", Description: "review role reporting status", Required: true},
				{Name: "--kind", Value: "KIND", Description: "accept, request_changes, reject, withdraw_accept, or decision", Required: true},
				{Name: "--change-hash", Value: "HASH", Description: "OpenSpec artifact hash", Required: true},
				{Name: "--openspec-valid", Description: "mark the report as based on passing OpenSpec validation"},
				{Name: "--blocking", Value: "BOOL", Description: "whether this report blocks the review gate"},
				{Name: "--reply-to", Value: "ID", Description: "comment ID this report resolves or answers"},
				{Name: "--body", Value: "TEXT", Description: "short report body"},
			},
		},
		{
			Command: "workflow report implementation",
			Summary: "Append a phase 2 implementation stage report.",
			Usage:   []string{"tmact workflow report implementation --config PATH --role swe|qa|pm --stage apply|verify|archive --kind complete|pass|fail|request_changes|blocked|decision|withdraw --change-hash HASH [--blocking=true|false] [--reply-to ID] [--body TEXT]"},
			Flags: []helpFlag{
				{Name: "--config", Value: "PATH", Description: "path to implementation workflow YAML config", Required: true},
				{Name: "--role", Value: "ROLE", Description: "implementation role reporting status", Required: true},
				{Name: "--stage", Value: "STAGE", Description: "apply, verify, or archive", Required: true},
				{Name: "--kind", Value: "KIND", Description: "complete, pass, fail, request_changes, blocked, decision, or withdraw", Required: true},
				{Name: "--change-hash", Value: "HASH", Description: "accepted OpenSpec artifact hash", Required: true},
				{Name: "--blocking", Value: "BOOL", Description: "whether this report blocks the stage"},
				{Name: "--reply-to", Value: "ID", Description: "comment ID this report resolves or answers"},
				{Name: "--body", Value: "TEXT", Description: "short report body"},
			},
		},
		{
			Command: "workflow status",
			Summary: "Inspect workflow run metadata and optional local OpenSpec workflow state.",
			Usage:   []string{"tmact workflow status [--config PATH] [--run-dir .tmact/runs] [--json]"},
			Flags: []helpFlag{
				{Name: "--config", Value: "PATH", Description: "workflow config path; include phase state details"},
				{Name: "--run-dir", Value: "PATH", Description: "directory for runtime metadata"},
				{Name: "--json", Description: "print JSON output"},
			},
			Examples: []string{"tmact workflow status", "tmact workflow status --config examples/openspec-workflow.yaml --json"},
		},
		runtimeStopHelp("workflow"),
		{
			Command: "watch",
			Summary: "Run a narrow prompt watcher for allowlisted answers.",
			Usage:   []string{"tmact watch --config PATH [--dry-run] [--once]"},
			Flags: []helpFlag{
				{Name: "--config", Value: "PATH", Description: "path to watch YAML config", Required: true},
				{Name: "--dry-run", Description: "print decisions without sending anything to tmux"},
				{Name: "--once", Description: "run one watch pass and exit"},
			},
			Examples: []string{"tmact watch --config examples/accept-question-watch.yaml --dry-run --once"},
			Safety:   []string{"Watcher configs must keep allow_paths or allow_path_patterns checks in place."},
		},
		{
			Command: "commands",
			Summary: "Print the command catalog for humans or LLM/tooling consumers.",
			Usage:   []string{"tmact commands [--json]"},
			Flags: []helpFlag{
				{Name: "--json", Description: "print machine-readable command metadata"},
			},
			Examples: []string{"tmact commands", "tmact commands --json", "tmact help loop --json"},
		},
		{
			Command: "version",
			Summary: "Print the tmact build version, including VCS revision when built from Git.",
			Usage:   []string{"tmact version [--json]", "tmact -v | --version"},
			Flags: []helpFlag{
				{Name: "--json", Description: "print machine-readable version metadata"},
			},
			Examples: []string{"tmact version", "tmact --version", "tmact version --json"},
		},
	}
}

func statusdHelpFlags() []helpFlag {
	return []helpFlag{
		{Name: "--interval", Value: "DURATION", Description: "scan interval"},
		{Name: "--state-path", Value: "PATH", Description: "latest JSON snapshot path"},
		{Name: "--log-path", Value: "PATH", Description: "optional JSONL daemon log path"},
		{Name: "--tmux-options", Description: "write @ai-* tmux options"},
		{Name: "--no-tmux-options", Description: "only write the state file"},
		{Name: "--capture-lines", Value: "N", Description: "number of pane history lines to capture"},
		{Name: "--initial-samples", Value: "N", Description: "captures per pane before statusd has history"},
		{Name: "--running-debounce", Value: "DURATION", Description: "keep running indicator after changes"},
		{Name: "--stale-after", Value: "DURATION", Description: "mark snapshot stale after this age"},
		{Name: "--idle-ignore", Value: "REGEXP", Description: "line regexp ignored by sample hashing; may be repeated"},
		{Name: "--session", Value: "GLOB", Description: "include sessions matching glob; may be repeated"},
		{Name: "--exclude-session", Value: "GLOB", Description: "exclude sessions matching glob; may be repeated"},
		{Name: "--json", Description: "print JSON output where supported"},
	}
}

func statusdStartHelpFlags() []helpFlag {
	return append([]helpFlag{
		{Name: "--once", Description: "run one scan then exit"},
		{Name: "--web-addr", Value: "ADDR", Description: "serve the read-only web UI on this address (e.g. 0.0.0.0:7890)"},
	}, statusdHelpFlags()...)
}

func agentFilterHelpFlags() []helpFlag {
	return []helpFlag{
		{Name: "--config", Value: "PATH", Description: "path to agent registry YAML config"},
		{Name: "--agent", Value: "NAME", Description: "agent name to include"},
		{Name: "--role", Value: "ROLE", Description: "role to include"},
		{Name: "--json", Description: "print JSON output"},
	}
}

func agentSummaryHelpFlags() []helpFlag {
	return []helpFlag{
		{Name: "--config", Value: "PATH", Description: "path to agent registry YAML config"},
		{Name: "--agent", Value: "NAME", Description: "agent name to summarize; omit for all agents"},
		{Name: "--json", Description: "print JSON output"},
	}
}

func runtimeStatusHelp(kind string) commandHelp {
	return commandHelp{
		Command:  kind + " status",
		Summary:  "Inspect registered " + kind + " run metadata.",
		Usage:    []string{"tmact " + kind + " status [--run-dir .tmact/runs] [--json]"},
		Flags:    []helpFlag{{Name: "--run-dir", Value: "PATH", Description: "directory for runtime metadata"}, {Name: "--json", Description: "print JSON output"}},
		Examples: []string{"tmact " + kind + " status", "tmact " + kind + " status --json"},
		Notes:    []string{"Shows id, runtime status, pid, target, config path, last event, tmux pane, and recent problems."},
	}
}

func runtimeStopHelp(kind string) commandHelp {
	sampleID := kind + "-night-loop-123"
	sampleConfig := "examples/night-loop.yaml"
	return commandHelp{
		Command: kind + " stop",
		Summary: "Stop a registered " + kind + " by id or config path.",
		Usage:   []string{"tmact " + kind + " stop (--id ID | --config PATH) [--run-dir .tmact/runs] [--json]"},
		Flags: []helpFlag{
			{Name: "--id", Value: "ID", Description: "runtime id to stop"},
			{Name: "--config", Value: "PATH", Description: "stop the runtime registered for this config"},
			{Name: "--run-dir", Value: "PATH", Description: "directory for runtime metadata"},
			{Name: "--json", Description: "print JSON output"},
		},
		Examples: []string{"tmact " + kind + " stop --id " + sampleID, "tmact " + kind + " stop --config " + sampleConfig},
		Safety:   []string{"Stops by sending C-c to the recorded tmux pane when available, otherwise interrupts the recorded process."},
	}
}
