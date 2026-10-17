package main

func mcpCommandHelpCatalog() []commandHelp {
	return []commandHelp{
		{
			Command:     "mcp",
			Summary:     "Serve a read-only MCP view of statusd for remote assistants such as ChatGPT.",
			Usage:       []string{"tmact mcp serve --access-team TEAM --access-aud AUD [--addr 127.0.0.1:7895]"},
			Subcommands: []string{"serve"},
			Examples: []string{
				"tmact mcp serve --access-team puni-tw --access-aud <aud-tag>",
				"tmact mcp serve --no-access",
			},
			Safety: []string{
				"Tools are read-only: list_sessions, list_panes, capture_pane. No dispatch, input, trust-folder, or kill is exposed.",
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
				{Name: "--config", Value: "PATH", Description: "statusd.json used to resolve peers for capture_pane"},
			},
			Notes: []string{"Reads the snapshot from the local statusd, so peers configured there are included."},
		},
	}
}
