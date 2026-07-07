package main

func hookCommandHelpCatalog() []commandHelp {
	return []commandHelp{
		{
			Command:     "hook",
			Summary:     "Opt-in shell preexec/precmd hooks that sharpen statusd running/idle state.",
			Usage:       []string{"tmact hook init zsh|bash|fish", "tmact hook emit --type preexec|precmd [flags]"},
			Subcommands: []string{"init", "emit"},
			Examples: []string{
				`eval "$(tmact hook init zsh)"`,
				"tmact hook emit --type precmd --pane-id %5 --exit-code 0",
			},
			Notes: []string{
				"Hooks are opt-in: tmact never edits shell rc files; source the init script yourself.",
				"Panes without hook events keep the existing capture-based classification.",
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
	}
}
