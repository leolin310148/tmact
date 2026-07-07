package main

func hookCommandHelpCatalog() []commandHelp {
	return []commandHelp{
		{
			Command:     "hook",
			Summary:     "Opt-in shell preexec/precmd hooks that sharpen statusd running/idle state.",
			Usage:       []string{"tmact hook init zsh|bash|fish", "tmact hook emit --type preexec|precmd [flags]", "tmact hook doctor [--pane-id %5]", "tmact hook state [--pane-id %5] [--json]"},
			Subcommands: []string{"init", "emit", "state", "doctor"},
			Examples: []string{
				`eval "$(tmact hook init zsh)"`,
				"tmact hook emit --type precmd --pane-id %5 --exit-code 0",
				"tmact hook doctor",
			},
			Notes: []string{
				"Hooks are opt-in: tmact never edits shell rc files; source the init script yourself.",
				"Panes without hook events keep the existing capture-based classification.",
				"hook doctor / hook state read the daemon over the local IPC socket to verify emits are landing.",
			},
		},
		{
			Command: "hook init",
			Summary: "Print a sourceable shell script that emits preexec/precmd events.",
			Usage:   []string{"tmact hook init zsh|bash|fish"},
			Examples: []string{
				`eval "$(tmact hook init zsh)"`,
				"tmact hook init fish | source",
			},
			Safety: []string{
				"The script only activates inside tmux ($TMUX_PANE set) and backgrounds every emit with output discarded, so a missing daemon never blocks the prompt.",
			},
			Notes: []string{
				"bash support is best-effort (DEBUG trap + PROMPT_COMMAND); with other PROMPT_COMMAND entries the reported exit code may be theirs.",
				"Set TMACT_HOOK_SOCKET to point emits at a non-default statusd socket.",
			},
		},
		{
			Command: "hook emit",
			Summary: "Send one shell hook event to the local statusd over its unix socket.",
			Usage: []string{
				"tmact hook emit --type preexec [--pane-id %5] [--command-id ID] [--command TEXT] [--cwd DIR] [--quiet] [--json]",
				"tmact hook emit --type precmd [--pane-id %5] [--command-id ID] [--exit-code N] [--quiet] [--json]",
				"tmact hook emit --stdin < event.json",
			},
			Flags: []helpFlag{
				{Name: "--type", Value: "TYPE", Description: "event type: preexec or precmd"},
				{Name: "--pane-id", Value: "ID", Description: "tmux pane id such as %5; defaults to $TMUX_PANE"},
				{Name: "--session-id", Value: "ID", Description: "tmux session id; optional"},
				{Name: "--command-id", Value: "ID", Description: "id correlating a preexec with its precmd"},
				{Name: "--command", Value: "TEXT", Description: "command line being run (preexec)"},
				{Name: "--exit-code", Value: "N", Description: "command exit status (precmd)"},
				{Name: "--cwd", Value: "DIR", Description: "working directory; defaults to $PWD"},
				{Name: "--socket-path", Value: "PATH", Description: "statusd IPC unix socket; defaults to $TMACT_HOOK_SOCKET, then the standard socket"},
				{Name: "--timeout", Value: "DURATION", Description: "max time for the send round-trip"},
				{Name: "--stdin", Description: "read one complete event as JSON from stdin instead of flags"},
				{Name: "--quiet", Description: "exit 0 silently on any failure; for use from shell hooks"},
				{Name: "--json", Description: "print JSON output"},
			},
			Examples: []string{
				"tmact hook emit --type preexec --pane-id %5 --command-id c1 --command 'make test'",
				"tmact hook emit --type precmd --pane-id %5 --command-id c1 --exit-code 0",
			},
			Notes: []string{
				"When statusd is not running, emit fails fast; --quiet turns that into a silent no-op.",
			},
		},
		{
			Command: "hook state",
			Summary: "Read the daemon's recorded per-pane shell hook state over the local IPC socket.",
			Usage: []string{
				"tmact hook state [--pane-id %5] [--socket-path PATH] [--json]",
			},
			Flags: []helpFlag{
				{Name: "--pane-id", Value: "ID", Description: "limit to one tmux pane id such as %5; defaults to all panes"},
				{Name: "--socket-path", Value: "PATH", Description: "statusd IPC unix socket; defaults to $TMACT_HOOK_SOCKET, then the standard socket"},
				{Name: "--timeout", Value: "DURATION", Description: "max time for the fetch round-trip"},
				{Name: "--json", Description: "print JSON output"},
			},
			Examples: []string{
				"tmact hook state",
				"tmact hook state --pane-id %5 --json",
			},
			Notes: []string{
				"Read-only: it never emits events into panes. Served only over the local IPC socket, never TCP.",
			},
		},
		{
			Command: "hook doctor",
			Summary: "Diagnose the shell hook pipeline: tmux, socket, daemon reachability, and per-pane emits.",
			Usage: []string{
				"tmact hook doctor [--pane-id %5] [--socket-path PATH] [--json]",
			},
			Flags: []helpFlag{
				{Name: "--pane-id", Value: "ID", Description: "pane to check for recorded events; defaults to $TMUX_PANE"},
				{Name: "--socket-path", Value: "PATH", Description: "statusd IPC unix socket; defaults to $TMACT_HOOK_SOCKET, then the standard socket"},
				{Name: "--timeout", Value: "DURATION", Description: "max time for the fetch round-trip"},
				{Name: "--json", Description: "print JSON output"},
			},
			Examples: []string{
				"tmact hook doctor",
				"tmact hook doctor --pane-id %5 --json",
			},
			Safety: []string{
				"Read-only diagnostics: never edits rc files, never sends keys or events into panes.",
			},
			Notes: []string{
				"Exits non-zero only when the statusd daemon is unreachable; missing per-pane events are warnings.",
			},
		},
	}
}
