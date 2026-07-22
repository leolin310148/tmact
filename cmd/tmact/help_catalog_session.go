package main

func sessionCommandHelpCatalog() []commandHelp {
	return []commandHelp{
		{
			Command:     "session",
			Summary:     "Close, list, and reopen recoverable local tmux sessions.",
			Usage:       []string{"tmact session close SESSION [--execute] [--json]", "tmact session closed [--json]", "tmact session reopen SESSION [--execute] [--json]"},
			Subcommands: []string{"close", "closed", "reopen"},
			Examples:    []string{"tmact session close work", "tmact session closed --json", "tmact session reopen work --execute"},
			Safety:      []string{"close and reopen are dry-run by default and require --execute to change tmux.", "Every mutation resolves one exact local session name; peer and pane/window targets are refused."},
			Notes:       []string{"History is shared with statusd and the web UI at ~/.tmact/closed-sessions.json.", "Reopen restores the saved name and cwd. Recorded claude, codex, and gemini runtime intent is relaunched through the new shell; all other runtime values safely fall back to a plain shell."},
		},
		{
			Command: "session close",
			Summary: "Preview or close one exact local tmux session and save its reopen intent.",
			Usage:   []string{"tmact session close SESSION [--execute] [--json]"},
			Flags: []helpFlag{
				{Name: "--execute", Description: "close the exact session; default is dry-run"},
				{Name: "--json", Description: "print the plan/result as JSON"},
			},
			Examples: []string{"tmact session close work", "tmact session close work --execute --json"},
			Safety:   []string{"Without --execute, tmux and closed-session history are unchanged.", "SESSION must be one exact local session name, never a pane, window, peer, glob, or broad selector."},
			Notes:    []string{"The saved entry contains only session name, cwd, detected runtime intent, and close time; pane contents are never persisted."},
		},
		{
			Command:  "session closed",
			Summary:  "List recoverable sessions from the shared statusd/web history.",
			Usage:    []string{"tmact session closed [--json]"},
			Flags:    []helpFlag{{Name: "--json", Description: "print {sessions:[...]} JSON matching the web history shape"}},
			Examples: []string{"tmact session closed", "tmact session closed --json"},
			Safety:   []string{"Read-only; listing never changes tmux."},
		},
		{
			Command: "session reopen",
			Summary: "Preview or reopen one exact session from closed history.",
			Usage:   []string{"tmact session reopen SESSION [--execute] [--json]"},
			Flags: []helpFlag{
				{Name: "--execute", Description: "recreate the recorded session; default is dry-run"},
				{Name: "--json", Description: "print the plan/result as JSON"},
			},
			Examples: []string{"tmact session reopen work", "tmact session reopen work --execute --json"},
			Safety:   []string{"Reopen refuses an existing session name and a missing or non-absolute recorded cwd.", "Only exact allowlisted claude, codex, and gemini runtime values are launched; custom values are never executed."},
			Notes:    []string{"A runtime launch failure removes only the just-created session and retains its history entry for another attempt.", "Reopen never accepts trust, permission, approval, or other prompts."},
		},
	}
}
