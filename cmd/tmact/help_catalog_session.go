package main

func sessionCommandHelpCatalog() []commandHelp {
	return []commandHelp{
		{
			Command:     "session",
			Summary:     "Create, close, reopen, and explicitly resume guarded local tmux sessions.",
			Usage:       []string{"tmact session create SESSION --dir DIR [--execute] [--json]", "tmact session close SESSION [--execute] [--json]", "tmact session closed [--json]", "tmact session reopen SESSION [--execute] [--json]", "tmact session resume SESSION --dir DIR --agent claude|codex --session-id ID [--execute] [--json]"},
			Subcommands: []string{"create", "close", "closed", "reopen", "resume"},
			Examples:    []string{"tmact session create work --dir .", "tmact session close work", "tmact session closed --json", "tmact session reopen work --execute", "tmact session resume work --dir . --agent codex --session-id 019c-example"},
			Safety:      []string{"create, close, reopen, and resume are dry-run by default and require --execute to change tmux.", "Every mutation resolves one exact local session name and canonical cwd; peer and pane/window targets are refused.", "Resume refuses busy panes, different runtimes, tmux modes, and all interactive prompts."},
			Notes:       []string{"History is shared with statusd and the web UI at ~/.tmact/closed-sessions.json.", "Reopen restores saved runtime intent; session resume instead requires an explicit Claude/Codex provider session id and never infers one from pane text."},
		},
		{
			Command: "session create",
			Summary: "Preview or create one exact local tmux session containing an idle shell.",
			Usage:   []string{"tmact session create SESSION --dir DIR [--execute] [--json]"},
			Flags: []helpFlag{
				{Name: "--dir", Value: "DIR", Description: "existing directory; canonicalized before validation", Required: true},
				{Name: "--execute", Description: "create the exact session; default is dry-run"},
				{Name: "--json", Description: "print the plan/result as JSON"},
			},
			Examples: []string{"tmact session create work --dir .", "tmact session create work --dir /repo --execute --json"},
			Safety:   []string{"Without --execute, tmux is unchanged.", "An existing session is reused only when it contains one idle shell at the exact canonical cwd; busy panes, agents, tmux modes, and prompts are refused."},
			Notes:    []string{"SESSION is one exact local session name. This command never launches an agent."},
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
		{
			Command: "session resume",
			Summary: "Preview or explicitly resume one Claude/Codex provider session in a guarded local tmux session.",
			Usage:   []string{"tmact session resume SESSION --dir DIR --agent claude|codex --session-id ID [--execute] [--json]"},
			Flags: []helpFlag{
				{Name: "--dir", Value: "DIR", Description: "existing directory; must exactly match an existing pane after canonicalization", Required: true},
				{Name: "--agent", Value: "claude|codex", Description: "provider whose resume command is launched", Required: true},
				{Name: "--session-id", Value: "ID", Description: "explicit provider session id; never read or inferred from pane text", Required: true},
				{Name: "--execute", Description: "launch the resume command; default is dry-run"},
				{Name: "--json", Description: "print the plan/result as JSON"},
			},
			Examples: []string{"tmact session resume work --dir . --agent claude --session-id 01234567-89ab-cdef-0123-456789abcdef", "tmact session resume work --dir . --agent codex --session-id 019c-example --execute --json"},
			Safety:   []string{"Without --execute, tmux is unchanged.", "Only a single idle shell at the exact canonical cwd may be used; busy/different runtimes, tmux modes, and every detected prompt are refused.", "Provider session ids are explicit non-option identifiers and cannot add flags or shell syntax."},
			Notes:    []string{"If SESSION does not exist, execute creates an idle shell there before launching the fixed provider resume command.", "A launch failure removes only a session created by this invocation; an existing session is never killed."},
		},
	}
}
