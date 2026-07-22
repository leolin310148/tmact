package main

func logCommandHelpCatalog() []commandHelp {
	return []commandHelp{
		{
			Command:     "log",
			Summary:     "Search normalized local Claude and Codex session logs with privacy-safe output defaults.",
			Usage:       []string{"tmact log search QUERY [--provider claude|codex] [--since DURATION|RFC3339] [--cwd DIR] [--kind KIND] [--limit N] [--json] [--show-content]"},
			Subcommands: []string{"search"},
			Examples:    []string{`tmact log search "failing test"`, `tmact log search "git status" --provider codex --since 24h --json`},
			Safety:      []string{"Read-only. Raw prompts, tool output, environment values, and full command arguments are omitted unless content is explicitly requested."},
			Notes:       []string{"Searches both Claude and Codex by default through their normalized streaming readers.", "Every result includes provider parse coverage and errors so partial scans are visible."},
		},
		{
			Command: "log search",
			Summary: "Search normalized session-log metadata and content with privacy-safe output defaults.",
			Usage:   []string{"tmact log search QUERY [--provider claude|codex] [--since DURATION|RFC3339] [--cwd DIR] [--kind KIND] [--limit N] [--json] [--show-content]"},
			Flags: []helpFlag{
				{Name: "--provider", Value: "claude|codex", Description: "provider to search; may be repeated, and omission searches both"},
				{Name: "--since", Value: "DURATION|RFC3339", Description: "include records at or after a relative duration such as 24h or an RFC3339 timestamp"},
				{Name: "--cwd", Value: "DIR", Description: "require exact normalized record cwd equality"},
				{Name: "--kind", Value: "KIND", Description: "normalized kind: unknown, session, context, message, reasoning, tool_call, tool_result, usage, progress, system, or queue"},
				{Name: "--limit", Value: "N", Description: "maximum newest matches retained; default 100"},
				{Name: "--json", Description: "print matches and per-provider parse coverage/errors as JSON"},
				{Name: "--show-content", Description: "opt in to normalized private prompt/tool content, bounded to 16 KiB per match"},
			},
			Examples: []string{
				`tmact log search "permission denied" --since 48h`,
				`tmact log search "go test" --provider claude --provider codex --kind tool_call --limit 20 --json`,
				`tmact log search "specific phrase" --cwd . --show-content`,
			},
			Safety: []string{
				"Default text and JSON output never include raw prompts, tool output, environment values, or full arguments; commands are reduced to a safe verb and recognized subcommand.",
				"--show-content is an explicit privacy opt-in and may reveal private prompts, command text, patches, or tool output from local provider logs.",
			},
			Notes: []string{
				"QUERY matching is case-insensitive across normalized metadata, full in-memory command text, and normalized content even when content is hidden from output.",
				"Results are newest-first and memory-bounded by --limit; content is not copied to an index or persisted.",
				"Coverage reports source, line, record, malformed, unknown, oversized, and error counts independently for each selected provider.",
			},
		},
	}
}
