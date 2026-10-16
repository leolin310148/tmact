package main

func credCommandHelpCatalog() []commandHelp {
	return []commandHelp{
		{
			Command:     "cred",
			Summary:     "Ask the user for a named credential by web push and use it without printing it.",
			Usage:       []string{"tmact cred request NAME --reason TEXT", "tmact cred run --env VAR=NAME -- COMMAND [ARGS...]", "tmact cred path|rm NAME", "tmact cred ls"},
			Subcommands: []string{"request", "run", "path", "ls", "rm"},
			Examples: []string{
				`tmact cred request github-pat --reason "push the release tag"`,
				"tmact cred run --env GITHUB_TOKEN=github-pat -- gh release create v1.2.0",
			},
			Safety: []string{
				"Credentials live as 0600 files under ~/.tmact/.cred/. No command or endpoint prints a value.",
				"Never cat, echo, or otherwise print a credential file: pane text is visible to statusd, the web UI, and peers.",
			},
			Notes: []string{
				"Requests go through the local statusd; the push hub (studio) notifies the phone, including for requests made on a peer.",
			},
		},
		{
			Command: "cred request",
			Summary: "Push a credential request to the user and wait for the answer.",
			Usage:   []string{"tmact cred request NAME --reason TEXT [--timeout 10m] [--refresh] [--json]"},
			Flags: []helpFlag{
				{Name: "--reason", Value: "TEXT", Description: "why it is needed; shown in the notification (max 200 characters)"},
				{Name: "--timeout", Value: "DURATION", Description: "how long to wait, up to 1h (default 10m)"},
				{Name: "--refresh", Description: "ask again even if NAME is already stored"},
				{Name: "--socket-path", Value: "PATH", Description: "statusd IPC unix socket"},
				{Name: "--json", Description: "print JSON output"},
			},
			Examples: []string{`tmact cred request npm-token --reason "publish @acme/cli 2.0"`},
			Notes: []string{
				"Prints only the stored file path. Exit status: 0 provided (or already stored), 3 declined, 4 expired, 1 other errors.",
				"A second request for the same NAME while one is pending joins it instead of sending another push.",
				"If NAME is already stored, returns its path without a push unless --refresh is given.",
			},
		},
		{
			Command: "cred run",
			Summary: "Run a command with credentials in its environment.",
			Usage:   []string{"tmact cred run --env VAR=NAME [--env VAR=NAME ...] -- COMMAND [ARGS...]"},
			Flags: []helpFlag{
				{Name: "--env", Value: "VAR=NAME", Description: "set VAR to credential NAME; repeatable"},
			},
			Examples: []string{"tmact cred run --env NPM_TOKEN=npm-token -- npm publish"},
			Safety:   []string{"Do not run commands that print their environment (env, printenv, set -x) under cred run."},
		},
		{
			Command: "cred path",
			Summary: "Print the file path of a stored credential.",
			Usage:   []string{"tmact cred path NAME"},
			Notes:   []string{"Prefer cred run; use the path only for tools that read a token from a file argument."},
		},
		{
			Command: "cred ls",
			Summary: "List stored credential names and modification times.",
			Usage:   []string{"tmact cred ls [--json]"},
		},
		{
			Command: "cred rm",
			Summary: "Delete a stored credential.",
			Usage:   []string{"tmact cred rm NAME"},
		},
	}
}
