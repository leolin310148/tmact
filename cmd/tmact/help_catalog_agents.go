package main

func agentCommandHelpCatalog() []commandHelp {
	return []commandHelp{
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
