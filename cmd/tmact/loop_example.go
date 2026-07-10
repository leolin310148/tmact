package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
)

func runLoopExample(args []string) error {
	if wantsHelp(args) {
		return printCommandHelp("loop example")
	}
	fs := flag.NewFlagSet("loop example", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	quota := fs.Bool("quota", false, "include 5-hour reserve and weekly headroom gates")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return errors.New("loop example does not accept positional arguments")
	}
	fmt.Print(renderLoopExampleYAML(*quota))
	return nil
}

func renderLoopExampleYAML(quota bool) string {
	quotaBlock := ""
	if quota {
		quotaBlock = loopExampleQuotaBlock
	}
	return loopExampleHeader + quotaBlock + loopExampleFlow
}

const loopExampleHeader = `# Generate this template with: tmact loop example
# Then edit target/prompt, validate it, and run one dry pass before starting:
#   tmact loop validate --config loop.yaml
#   tmact loop run --config loop.yaml --dry-run --once --assume-idle-on-start
#   tmact loop start --config loop.yaml

target: sample-agent:0.0
capture_lines: 160
poll_interval: 30s
idle_after: 2m
max_runtime: 8h
max_actions: 48
log_path: .tmact/maintenance-loop.jsonl
log_skipped_actions: true
stop_on_permission_prompt: true

`

const loopExampleQuotaBlock = `# Quota gates are evaluated together: every enabled gate must pass.
quota:
  enabled: true
  provider: codex                    # codex | claude
  session_min_remaining_percent: 20 # configurable; remaining 5h quota must be > this
  weekly_require_headroom: true      # actual weekly usage must be below expected linear usage
  weekly_skip_at_percent: 100        # optional absolute weekly ceiling
  refresh_interval: 5m
  fail_closed: false                 # false runs when quota/pace is unavailable

`

const loopExampleFlow = `flows:
  - name: maintenance-cycle
    every: 20m
    initial_delay: 20m
    only_when_idle: true
    max_runs: 24
    steps:
      - name: clear-context
        type: clear
        command: /clear
        post_delay: 5s

      - name: improvement-prompt
        type: send_text
        enter: true
        text: |
          Find one small, useful improvement for this repository.
          Keep the change scoped, run relevant tests, and summarize the result.
`
