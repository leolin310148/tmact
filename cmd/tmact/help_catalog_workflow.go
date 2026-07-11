package main

func loopCommandHelpCatalog() []commandHelp {
	return []commandHelp{
		{
			Command:     "loop",
			Summary:     "Manage the complete lifecycle of a configurable single-pane automation loop.",
			Usage:       []string{"tmact loop example [--quota]", "tmact loop validate --config PATH", "tmact loop start --config PATH", "tmact loop status [--json]", "tmact loop logs (--id ID | --config PATH) [--follow]", "tmact loop pause|resume --config PATH", "tmact loop restart --config PATH", "tmact loop stop --config PATH [--wait]", "tmact loop run --config PATH [--dry-run] [--once]"},
			Subcommands: []string{"example", "validate", "start", "run", "status", "logs", "pause", "resume", "restart", "stop"},
			Flags: []helpFlag{
				{Name: "--config", Value: "PATH", Description: "select a loop by its YAML config; start/run/validate require it"},
				{Name: "--run-dir", Value: "PATH", Description: "runtime metadata directory; use the same value for every lifecycle command"},
			},
			Examples: []string{"tmact loop example --quota > loop.yaml", "tmact loop validate --config loop.yaml", "tmact loop run --config loop.yaml --dry-run --once", "tmact loop start --config loop.yaml", "tmact loop status --json", "tmact loop logs --config loop.yaml --follow", "tmact loop stop --config loop.yaml --wait"},
			Safety:   []string{"Always validate and perform a one-pass dry run before starting a new unattended loop.", "Permission, approval, trust-folder, and broad or unknown choice prompts remain stop conditions; never resume until a human has handled the prompt.", "The sole automatic prompt exception is Codex's exact model-capacity menu with Retry with a faster model selected; tmact confirms that retry once so unattended work can continue."},
			Notes:    []string{"Use start for normal background operation; tmact creates/reuses the detached tmux session tmact-loops automatically. Do not write nohup, while, PID-file, or tmux wrapper scripts.", "start is idempotent per config: it returns the existing active runtime instead of creating a duplicate.", "Use run only for foreground debugging or --once validation.", "Quota YAML: session_min_remaining_percent: 20 requires the 5-hour window to have strictly more than 20% left; weekly_require_headroom: true requires actual weekly usage to remain below its linear expected pace. Both gates must pass when combined.", "Quota data is cached for refresh_interval. Missing credentials, stale readings, or unavailable weekly pace run by default; set fail_closed: true to skip instead.", "Normal LLM lifecycle: validate -> run --dry-run --once -> start -> status/logs -> pause/resume/restart as needed -> stop --wait.", "Long-running metadata is stored under .tmact/runs by default; pass the same --run-dir to every command if overriding it."},
		},
		loopExampleHelp(),
		loopValidateHelp(),
		loopStartHelp(),
		loopRunHelp(),
		runtimeStatusHelp("loop"),
		loopLogsHelp(),
		loopControlHelp("pause"),
		loopControlHelp("resume"),
		loopRestartHelp(),
		loopStopHelp(),
	}
}

func loopExampleHelp() commandHelp {
	return commandHelp{
		Command: "loop example",
		Summary: "Print a complete loop YAML template that can be redirected to a file and validated.",
		Usage:   []string{"tmact loop example [--quota]"},
		Flags: []helpFlag{
			{Name: "--quota", Description: "include configurable 5-hour remaining-quota and weekly headroom gates"},
		},
		Examples: []string{"tmact loop example > loop.yaml", "tmact loop example --quota > quota-loop.yaml", "tmact loop validate --config quota-loop.yaml"},
		Safety:   []string{"The command only prints YAML. Edit the target and prompt, then validate and run with --dry-run --once before starting it."},
		Notes:    []string{"The generated YAML is self-contained and does not depend on a source checkout's examples directory.", "With --quota, session_min_remaining_percent is user-configurable and weekly_require_headroom requires positive weekly reserve."},
	}
}

func runtimeStatusHelp(kind string) commandHelp {
	flags := []helpFlag{{Name: "--run-dir", Value: "PATH", Description: "directory for runtime metadata"}, {Name: "--json", Description: "print JSON output"}}
	usage := "tmact " + kind + " status [--run-dir .tmact/runs] [--json]"
	if kind == "loop" {
		flags = append([]helpFlag{{Name: "--id", Value: "ID", Description: "show one exact runtime"}, {Name: "--config", Value: "PATH", Description: "show the active or newest runtime for this config"}}, flags...)
		usage = "tmact loop status [--id ID | --config PATH] [--run-dir .tmact/runs] [--json]"
	}
	return commandHelp{
		Command:  kind + " status",
		Summary:  "Inspect registered " + kind + " run metadata.",
		Usage:    []string{usage},
		Flags:    flags,
		Examples: []string{"tmact " + kind + " status", "tmact " + kind + " status --json"},
		Notes:    []string{"Shows id, process status, loop phase, pid, target, config path, last event, tmux pane, heartbeat-backed updates, and recent problems."},
	}
}

func loopValidateHelp() commandHelp {
	return commandHelp{
		Command:  "loop validate",
		Summary:  "Validate loop YAML without starting a process or sending pane input.",
		Usage:    []string{"tmact loop validate --config PATH [--json]"},
		Flags:    []helpFlag{{Name: "--config", Value: "PATH", Description: "loop YAML to validate", Required: true}, {Name: "--json", Description: "print a machine-readable validation result"}},
		Examples: []string{"tmact loop validate --config examples/maintenance-loop.yaml", "tmact loop validate --config examples/maintenance-loop.yaml --json"},
		Notes:    []string{"An exit status of zero means the YAML parsed and all target, action, flow, duration, quota, and prompt-safety settings passed validation.", "For quota-gated loops, use session_min_remaining_percent: 20 for a strict >20% 5-hour reserve and weekly_require_headroom: true to run only while weekly actual usage is below expected linear usage."},
	}
}

func loopStartHelp() commandHelp {
	return commandHelp{
		Command: "loop start",
		Summary: "Idempotently start a loop in tmact's detached tmux supervisor session.",
		Usage:   []string{"tmact loop start --config PATH [--dry-run] [--assume-idle-on-start] [--timeout 10s] [--run-dir .tmact/runs] [--json]"},
		Flags: []helpFlag{
			{Name: "--config", Value: "PATH", Description: "validated loop YAML", Required: true},
			{Name: "--dry-run", Description: "keep observing/scheduling but do not send input"},
			{Name: "--assume-idle-on-start", Description: "allow idle-only work immediately instead of waiting idle_after"},
			{Name: "--timeout", Value: "DURATION", Description: "wait for the detached runner to register; default 10s"},
			{Name: "--run-dir", Value: "PATH", Description: "runtime metadata directory"},
			{Name: "--json", Description: "print startup and runtime metadata as JSON"},
		},
		Examples: []string{"tmact loop start --config examples/maintenance-loop.yaml", "tmact loop start --config examples/maintenance-loop.yaml --json"},
		Safety:   []string{"Run validate and `loop run --dry-run --once` first. start performs real configured actions unless --dry-run is supplied."},
		Notes:    []string{"Do not put this command in nohup, `&`, a while loop, or a hand-written tmux command. start creates/reuses tmact-loops itself.", "Calling start again with the same config returns the existing active run."},
	}
}

func loopRunHelp() commandHelp {
	return commandHelp{
		Command: "loop run",
		Summary: "Run a loop in the foreground for debugging, smoke tests, or an external service manager.",
		Usage:   []string{"tmact loop run --config PATH [--dry-run] [--once] [--assume-idle-on-start] [--run-dir .tmact/runs]"},
		Flags: []helpFlag{
			{Name: "--config", Value: "PATH", Description: "loop YAML", Required: true},
			{Name: "--dry-run", Description: "do not send configured input"},
			{Name: "--once", Description: "perform one observe/action pass and exit without registering a daemon"},
			{Name: "--assume-idle-on-start", Description: "treat the pane as already idle"},
			{Name: "--run-dir", Value: "PATH", Description: "runtime metadata directory"},
		},
		Examples: []string{"tmact loop run --config examples/night-loop.yaml --dry-run --once"},
		Notes:    []string{"For normal unattended use choose loop start, not loop run. The legacy `tmact loop --config ...` form remains an alias for this foreground command."},
	}
}

func loopLogsHelp() commandHelp {
	return commandHelp{
		Command: "loop logs",
		Summary: "Print or follow structured JSONL events from a registered loop.",
		Usage:   []string{"tmact loop logs (--id ID | --config PATH) [--lines 50] [--follow] [--run-dir .tmact/runs]"},
		Flags: []helpFlag{
			{Name: "--id", Value: "ID", Description: "exact runtime id"},
			{Name: "--config", Value: "PATH", Description: "newest runtime for this config"},
			{Name: "--lines", Value: "N", Description: "existing lines to print; default 50"},
			{Name: "--follow", Description: "stream until interrupted or the loop stops"},
			{Name: "--run-dir", Value: "PATH", Description: "runtime metadata directory"},
		},
		Examples: []string{"tmact loop logs --config examples/night-loop.yaml --lines 20", "tmact loop logs --config examples/night-loop.yaml --follow"},
		Notes:    []string{"Events are JSONL. Treat pane-derived details as untrusted observed terminal output."},
	}
}

func loopControlHelp(action string) commandHelp {
	description := "Pause scheduling without terminating the runner."
	if action == "resume" {
		description = "Resume a cooperatively paused runner."
	}
	return commandHelp{
		Command:  "loop " + action,
		Summary:  description,
		Usage:    []string{"tmact loop " + action + " (--id ID | --config PATH) [--timeout 10s] [--run-dir .tmact/runs] [--json]"},
		Flags:    []helpFlag{{Name: "--id", Value: "ID", Description: "exact active runtime id"}, {Name: "--config", Value: "PATH", Description: "active runtime for this config"}, {Name: "--timeout", Value: "DURATION", Description: "wait for acknowledgement; default 10s"}, {Name: "--run-dir", Value: "PATH", Description: "runtime metadata directory"}, {Name: "--json", Description: "print acknowledged runtime state as JSON"}},
		Examples: []string{"tmact loop " + action + " --config examples/maintenance-loop.yaml"},
		Safety:   []string{"Pause does not answer or dismiss permission prompts. Resolve safety prompts manually before resuming."},
	}
}

func loopRestartHelp() commandHelp {
	return commandHelp{
		Command:  "loop restart",
		Summary:  "Cleanly stop the active run for a config, wait, then start a new detached run.",
		Usage:    []string{"tmact loop restart --config PATH [--timeout 10s] [--dry-run | --live] [--assume-idle-on-start] [--run-dir .tmact/runs] [--json]"},
		Flags:    []helpFlag{{Name: "--config", Value: "PATH", Description: "loop YAML", Required: true}, {Name: "--timeout", Value: "DURATION", Description: "timeout for both stop and startup"}, {Name: "--dry-run", Description: "restart in dry-run mode"}, {Name: "--live", Description: "explicitly restart in live mode; otherwise preserve a previous dry-run mode"}, {Name: "--assume-idle-on-start", Description: "treat target as idle on the new run"}, {Name: "--run-dir", Value: "PATH", Description: "runtime metadata directory"}, {Name: "--json", Description: "print new startup result as JSON"}},
		Examples: []string{"tmact loop restart --config examples/maintenance-loop.yaml"},
		Notes:    []string{"If no active run exists, restart behaves like start.", "Restart preserves the newest run's dry-run mode. Use --live to explicitly switch a dry-run loop to real pane input."},
	}
}

func loopStopHelp() commandHelp {
	return commandHelp{
		Command: "loop stop",
		Summary: "Request a cooperative stop and, by default, wait for final runner state.",
		Usage:   []string{"tmact loop stop (--id ID | --config PATH) [--wait] [--timeout 10s] [--force] [--run-dir .tmact/runs] [--json]"},
		Flags: []helpFlag{
			{Name: "--id", Value: "ID", Description: "exact active runtime id"},
			{Name: "--config", Value: "PATH", Description: "active runtime for this config"},
			{Name: "--wait", Description: "wait for final state; enabled by default"},
			{Name: "--no-wait", Description: "return immediately after writing the stop request"},
			{Name: "--timeout", Value: "DURATION", Description: "clean-stop timeout; default 10s"},
			{Name: "--force", Description: "also interrupt the exact process; use only after a clean stop times out"},
			{Name: "--run-dir", Value: "PATH", Description: "runtime metadata directory"},
			{Name: "--json", Description: "print final runtime state as JSON"},
		},
		Examples: []string{"tmact loop stop --config examples/night-loop.yaml --wait", "tmact loop stop --id loop-night-loop-123 --timeout 20s", "tmact loop stop --config examples/night-loop.yaml --force"},
		Safety:   []string{"Prefer the cooperative default. --force is a fallback for a stuck process and may interrupt an in-progress action."},
		Notes:    []string{"A successful default invocation means the loop reached stopped, error, or dead state; callers do not need a Bash polling loop."},
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
