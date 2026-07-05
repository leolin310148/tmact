package main

func workflowCommandHelpCatalog() []commandHelp {
	return []commandHelp{
		{
			Command:     "loop",
			Summary:     "Run, inspect, or stop a configurable single-pane automation loop.",
			Usage:       []string{"tmact loop --config PATH [--dry-run] [--once] [--assume-idle-on-start]", "tmact loop status [--run-dir .tmact/runs] [--json]", "tmact loop stop (--id ID | --config PATH)"},
			Subcommands: []string{"status", "stop"},
			Flags: []helpFlag{
				{Name: "--config", Value: "PATH", Description: "path to loop YAML config", Required: true},
				{Name: "--dry-run", Description: "print actions without sending anything to tmux"},
				{Name: "--once", Description: "run one observe/action pass and exit"},
				{Name: "--assume-idle-on-start", Description: "treat the pane as already idle when the loop starts"},
				{Name: "--run-dir", Value: "PATH", Description: "directory for runtime metadata"},
			},
			Examples: []string{"tmact loop --config examples/night-loop.yaml --dry-run --once", `peer: peer-a + target: "%7" in the loop YAML targets a peer pane`, "tmact loop status", "tmact loop stop --config examples/night-loop.yaml"},
			Safety:   []string{"Loops should stop on permission prompts rather than auto-confirming them. Validate with --dry-run --once first."},
			Notes:    []string{"Long-running loop metadata is stored under .tmact/runs by default.", "Peer loop targets use statusd_config or ~/.tmact/statusd.json by default, and must target a canonical pane id like %7."},
		},
		runtimeStatusHelp("loop"),
		runtimeStopHelp("loop"),
		{
			Command:     "workflow",
			Summary:     "Run, inspect, or stop serialized OpenSpec review and implementation workflows.",
			Usage:       []string{"tmact workflow example", "tmact workflow discuss --config PATH [--dry-run] [--once] [--execute]", "tmact workflow implement --config PATH [--dry-run] [--once] [--execute]", "tmact workflow report review --config PATH --role ROLE --kind KIND --change-hash HASH", "tmact workflow report implementation --config PATH --role ROLE --stage STAGE --kind KIND --change-hash HASH", "tmact workflow status [--config PATH] [--run-dir .tmact/runs] [--json]", "tmact workflow stop (--id ID | --config PATH)"},
			Subcommands: []string{"example", "discuss", "implement", "report", "status", "stop"},
			Examples:    []string{"tmact workflow example", "tmact workflow discuss --config examples/openspec-full-workflow.yaml --dry-run --once", "tmact workflow implement --config examples/openspec-full-workflow.yaml --dry-run --once", "tmact workflow report review --config examples/openspec-full-workflow.yaml --role qa --kind accept --change-hash sha256:abc --openspec-valid", "tmact workflow status --config examples/openspec-full-workflow.yaml", "tmact workflow stop --config examples/openspec-full-workflow.yaml"},
			Safety:      []string{"Workflow prompts are dry-run by default. Use --execute only after inspecting the planned prompt and target roles."},
			Notes:       []string{"Discussion uses serialized PM -> SWE -> QA -> reviewer review. Implementation uses SWE apply -> QA verify -> PM archive."},
		},
		{
			Command:  "workflow example",
			Summary:  "Print a combined OpenSpec workflow YAML example.",
			Usage:    []string{"tmact workflow example"},
			Examples: []string{"tmact workflow example > examples/openspec-full-workflow.yaml"},
			Notes:    []string{"The output includes both discussion and implementation sections, so the same config can drive both phases."},
		},
		{
			Command: "workflow discuss",
			Summary: "Run one or more serialized OpenSpec artifact review passes.",
			Usage:   []string{"tmact workflow discuss --config PATH [--dry-run] [--once] [--execute]"},
			Flags: []helpFlag{
				{Name: "--config", Value: "PATH", Description: "path to workflow YAML config", Required: true},
				{Name: "--dry-run", Description: "print planned prompts without sending to tmux; default behavior"},
				{Name: "--execute", Description: "send prompts to configured tmux panes"},
				{Name: "--once", Description: "run one observe/validate/gate/prompt pass and exit"},
				{Name: "--run-dir", Value: "PATH", Description: "directory for runtime metadata"},
			},
			Examples: []string{"tmact workflow discuss --config examples/openspec-workflow.yaml --dry-run --once", "tmact workflow discuss --config examples/openspec-workflow.yaml --execute"},
			Safety:   []string{"The workflow stops on permission prompts and does not auto-approve tools or filesystem access."},
		},
		{
			Command: "workflow implement",
			Summary: "Run one or more serialized OpenSpec implementation passes.",
			Usage:   []string{"tmact workflow implement --config PATH [--dry-run] [--once] [--execute]"},
			Flags: []helpFlag{
				{Name: "--config", Value: "PATH", Description: "path to implementation workflow YAML config", Required: true},
				{Name: "--dry-run", Description: "print planned prompts without sending to tmux; default behavior"},
				{Name: "--execute", Description: "send prompts to configured tmux panes"},
				{Name: "--once", Description: "run one observe/validate/gate/prompt pass and exit"},
				{Name: "--run-dir", Value: "PATH", Description: "directory for runtime metadata"},
			},
			Examples: []string{"tmact workflow implement --config examples/openspec-implementation.yaml --dry-run --once", "tmact workflow implement --config examples/openspec-implementation.yaml --execute"},
			Safety:   []string{"The implementation workflow requires phase 1 agreement before live execution and does not auto-approve tools or archive prompts."},
		},
		{
			Command:     "workflow report",
			Summary:     "Record workflow progress through durable JSONL reports.",
			Usage:       []string{"tmact workflow report review --config PATH --role ROLE --kind KIND --change-hash HASH [--openspec-valid] [--blocking=true|false] [--reply-to ID] [--body TEXT]", "tmact workflow report implementation --config PATH --role ROLE --stage STAGE --kind KIND --change-hash HASH [--blocking=true|false] [--reply-to ID] [--body TEXT]"},
			Subcommands: []string{"review", "implementation"},
			Examples:    []string{"tmact workflow report review --config examples/openspec-full-workflow.yaml --role pm --kind accept --change-hash sha256:abc --openspec-valid --body \"accepted current artifacts\"", "tmact workflow report implementation --config examples/openspec-full-workflow.yaml --role qa --stage verify --kind pass --change-hash sha256:abc --body \"tests passed\""},
			Safety:      []string{"Reports only write workflow state for the configured OpenSpec change and do not send tmux input."},
		},
		{
			Command: "workflow report review",
			Summary: "Append a phase 1 OpenSpec review report.",
			Usage:   []string{"tmact workflow report review --config PATH --role ROLE --kind accept|request_changes|reject|withdraw_accept|decision --change-hash HASH [--openspec-valid] [--blocking=true|false] [--reply-to ID] [--body TEXT]"},
			Flags: []helpFlag{
				{Name: "--config", Value: "PATH", Description: "path to workflow YAML config", Required: true},
				{Name: "--role", Value: "ROLE", Description: "review role reporting status", Required: true},
				{Name: "--kind", Value: "KIND", Description: "accept, request_changes, reject, withdraw_accept, or decision", Required: true},
				{Name: "--change-hash", Value: "HASH", Description: "OpenSpec artifact hash", Required: true},
				{Name: "--openspec-valid", Description: "mark the report as based on passing OpenSpec validation"},
				{Name: "--blocking", Value: "BOOL", Description: "whether this report blocks the review gate"},
				{Name: "--reply-to", Value: "ID", Description: "comment ID this report resolves or answers"},
				{Name: "--body", Value: "TEXT", Description: "short report body"},
			},
		},
		{
			Command: "workflow report implementation",
			Summary: "Append a phase 2 implementation stage report.",
			Usage:   []string{"tmact workflow report implementation --config PATH --role swe|qa|pm --stage apply|verify|archive --kind complete|pass|fail|request_changes|blocked|decision|withdraw --change-hash HASH [--blocking=true|false] [--reply-to ID] [--body TEXT]"},
			Flags: []helpFlag{
				{Name: "--config", Value: "PATH", Description: "path to implementation workflow YAML config", Required: true},
				{Name: "--role", Value: "ROLE", Description: "implementation role reporting status", Required: true},
				{Name: "--stage", Value: "STAGE", Description: "apply, verify, or archive", Required: true},
				{Name: "--kind", Value: "KIND", Description: "complete, pass, fail, request_changes, blocked, decision, or withdraw", Required: true},
				{Name: "--change-hash", Value: "HASH", Description: "accepted OpenSpec artifact hash", Required: true},
				{Name: "--blocking", Value: "BOOL", Description: "whether this report blocks the stage"},
				{Name: "--reply-to", Value: "ID", Description: "comment ID this report resolves or answers"},
				{Name: "--body", Value: "TEXT", Description: "short report body"},
			},
		},
		{
			Command: "workflow status",
			Summary: "Inspect workflow run metadata and optional local OpenSpec workflow state.",
			Usage:   []string{"tmact workflow status [--config PATH] [--run-dir .tmact/runs] [--json]"},
			Flags: []helpFlag{
				{Name: "--config", Value: "PATH", Description: "workflow config path; include phase state details"},
				{Name: "--run-dir", Value: "PATH", Description: "directory for runtime metadata"},
				{Name: "--json", Description: "print JSON output"},
			},
			Examples: []string{"tmact workflow status", "tmact workflow status --config examples/openspec-workflow.yaml --json"},
		},
		runtimeStopHelp("workflow"),
	}
}

func runtimeStatusHelp(kind string) commandHelp {
	return commandHelp{
		Command:  kind + " status",
		Summary:  "Inspect registered " + kind + " run metadata.",
		Usage:    []string{"tmact " + kind + " status [--run-dir .tmact/runs] [--json]"},
		Flags:    []helpFlag{{Name: "--run-dir", Value: "PATH", Description: "directory for runtime metadata"}, {Name: "--json", Description: "print JSON output"}},
		Examples: []string{"tmact " + kind + " status", "tmact " + kind + " status --json"},
		Notes:    []string{"Shows id, runtime status, pid, target, config path, last event, tmux pane, and recent problems."},
	}
}

func runtimeStopHelp(kind string) commandHelp {
	sampleID := kind + "-night-loop-123"
	sampleConfig := "examples/night-loop.yaml"
	return commandHelp{
		Command: kind + " stop",
		Summary: "Stop a registered " + kind + " by id or config path.",
		Usage:   []string{"tmact " + kind + " stop (--id ID | --config PATH) [--run-dir .tmact/runs] [--json]"},
		Flags: []helpFlag{
			{Name: "--id", Value: "ID", Description: "runtime id to stop"},
			{Name: "--config", Value: "PATH", Description: "stop the runtime registered for this config"},
			{Name: "--run-dir", Value: "PATH", Description: "directory for runtime metadata"},
			{Name: "--json", Description: "print JSON output"},
		},
		Examples: []string{"tmact " + kind + " stop --id " + sampleID, "tmact " + kind + " stop --config " + sampleConfig},
		Safety:   []string{"Stops by sending C-c to the recorded tmux pane when available, otherwise interrupts the recorded process."},
	}
}
