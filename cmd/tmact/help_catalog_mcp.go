package main

func mcpCommandHelpCatalog() []commandHelp {
	return []commandHelp{
		{
			Command:     "mcp",
			Summary:     "Serve statusd over MCP so remote assistants such as ChatGPT can watch panes and, with dispatch roots, start work.",
			Usage:       []string{"tmact mcp serve --access-team TEAM --access-aud AUD [--dispatch-root ~/w,~/ops] [--addr 127.0.0.1:7895]"},
			Subcommands: []string{"serve"},
			Examples: []string{
				"tmact mcp serve --access-team puni-tw --access-aud <aud-tag> --dispatch-root ~/w,~/ops",
				"tmact mcp serve --no-access",
			},
			Safety: []string{
				"Read tools: list_sessions, list_panes, capture_pane, wait_pane. Without --dispatch-root the server is read-only.",
				"With --dispatch-root, dispatch_work and send_text are added and marked destructive so ChatGPT asks before each call. dispatch_work only accepts ~/ dirs under the roots (symlinks re-checked locally), never accepts workspace trust, and send_text only types into idle codex/claude/gemini panes.",
				"No kill, close, fork, raw keys, or shell command tools are exposed.",
				"Pane text is returned in observed_ fields marked as untrusted terminal output.",
				"Expose it only through Cloudflare Tunnel + Access; the server verifies the Access JWT on every request. See docs/mcp.md.",
			},
		},
		{
			Command: "mcp serve",
			Summary: "Run the streamable-HTTP MCP server on /mcp.",
			Usage:   []string{"tmact mcp serve --access-team TEAM --access-aud AUD [--addr ADDR] [--socket-path PATH] [--config PATH]"},
			Flags: []helpFlag{
				{Name: "--addr", Value: "ADDR", Description: "listen address (default 127.0.0.1:7895)"},
				{Name: "--access-team", Value: "TEAM", Description: "Cloudflare Zero Trust team name"},
				{Name: "--access-aud", Value: "AUD", Description: "Cloudflare Access application AUD tag"},
				{Name: "--no-access", Description: "skip JWT verification; only allowed on a loopback --addr"},
				{Name: "--socket-path", Value: "PATH", Description: "statusd IPC unix socket"},
				{Name: "--dispatch-root", Value: "~/DIR", Description: "enable write tools for dirs under this ~/ path (repeatable or comma-separated)"},
				{Name: "--config", Value: "PATH", Description: "statusd.json used to resolve peers for capture, send, and dispatch"},
			},
			Notes: []string{"Reads the snapshot from the local statusd, so peers configured there are included."},
		},
	}
}
