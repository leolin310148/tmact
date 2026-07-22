package main

func logCommandHelpCatalog() []commandHelp {
	return []commandHelp{
		{
			Command:     "log",
			Summary:     "Search and summarize normalized local Claude and Codex session logs with privacy-safe defaults.",
			Usage:       []string{"tmact log search QUERY [--provider claude|codex] [--since DURATION|RFC3339] [--cwd DIR] [--kind KIND] [--limit N] [--json] [--show-content]", "tmact log stats [--since DURATION|RFC3339] [--json]", "tmact log doctor [--json]"},
			Subcommands: []string{"search", "stats", "doctor"},
			Examples:    []string{`tmact log search "failing test"`, `tmact log stats --since 24h --json`, `tmact log doctor`},
			Safety:      []string{"Read-only toward provider logs. Raw prompts, tool output, environment values, and full command arguments are omitted unless search content is explicitly requested."},
			Notes:       []string{"Searches both Claude and Codex by default through their normalized streaming readers.", "Search and doctor expose parser coverage and errors so partial scans are visible.", "Stats and doctor maintain a privacy-safe plain-file index under ~/.tmact; the index never stores prompt or tool content."},
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
		{
			Command: "log stats",
			Summary: "Aggregate privacy-safe session-log metadata through an incremental plain-file index.",
			Usage:   []string{"tmact log stats [--since DURATION|RFC3339] [--json]"},
			Flags: []helpFlag{
				{Name: "--since", Value: "DURATION|RFC3339", Description: "include records at or after a relative duration such as 24h or an RFC3339 timestamp"},
				{Name: "--json", Description: "print aggregates and index activity as JSON"},
			},
			Examples: []string{`tmact log stats`, `tmact log stats --since 7d --json`},
			Safety: []string{
				"Provider logs are read-only; the index stores only timestamp, provider, kind, tool, and reduced command verb/subcommand fields.",
				"Beyond the required source-path key, raw prompts, tool output, environment values, normalized cwd/session-id fields, and full command arguments are never cached or printed.",
			},
			Notes: []string{
				"Aggregates are grouped independently by provider, tool, command verb, and recognized subcommand.",
				"The index at ~/.tmact/log-index.json is keyed by source path, size, mtime, and parser version; unchanged files are reused and verified append-only growth is parsed incrementally.",
				"Missing, corrupt, or parser-stale indexes are rebuilt with an atomic plain-file write; use log doctor for coverage and cache health.",
			},
		},
		{
			Command:  "log doctor",
			Summary:  "Report session-log file counts, skipped records, schema coverage, and cache health.",
			Usage:    []string{"tmact log doctor [--json]"},
			Flags:    []helpFlag{{Name: "--json", Description: "print file, record, schema-coverage, cache-health, and error details as JSON"}},
			Examples: []string{`tmact log doctor`, `tmact log doctor --json`},
			Safety: []string{
				"Read-only toward provider logs; cache repair writes only privacy-safe normalized fields under ~/.tmact.",
				"Doctor never prints or persists raw prompt, tool-output, environment, or full-argument content.",
			},
			Notes: []string{
				"Skipped is malformed plus oversized JSONL records; unknown records are reported separately as schema coverage.",
				"A missing, corrupt, or parser-version-stale cache is rebuilt before healthy=true is reported.",
				"Discovery and stream errors remain visible instead of silently claiming complete coverage.",
			},
		},
	}
}
