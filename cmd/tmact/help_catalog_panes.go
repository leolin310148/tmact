package main

func paneCommandHelpCatalog() []commandHelp {
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
	}
}

func paneUtilityCommandHelpCatalog() []commandHelp {
	return []commandHelp{
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
