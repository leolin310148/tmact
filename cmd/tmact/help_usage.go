package main

import "fmt"

func usage() error {
	fmt.Print(usageText())
	return nil
}

func usageText() string {
	return `tmact - local tmux automation for agent panes

Usage:
  tmact ls [--json]
  tmact -t 0 send --command "go test ./..." [--execute]
  tmact -t 0 send --text "summarize progress" [--enter] [--execute]
  tmact -t 0 send --key Enter [--execute]
  tmact -t 0 send --keys C-u,Enter [--execute]
  tmact detect [--target sample:0.0] [--lines 120] [--json]
  tmact inspect [--target sample:0.0 | --session sample | --all] [--sample 2 --interval 1s] [--json]
  tmact status [--config examples/agents.yaml] [--agent sample-codex] [--role maintenance] [--json]
  tmact statusd start|once|read|status [--state-path /tmp/tmact-status.json]
  tmact stt-set --provider openai --api-key KEY [--model gpt-4o-transcribe]
  tmact inbox [--config examples/agents.yaml] [--agent sample-codex] [--role maintenance] [--json]
  tmact summarize [--config examples/agents.yaml] [--agent sample-codex] [--json]
  tmact broadcast [--config examples/agents.yaml] --agent sample-codex --text "summarize progress" [--enter] [--execute]
  tmact panels plan [--config examples/agents.yaml] [--session sample-team] [--json]
  tmact panels ensure [--config examples/agents.yaml] [--session sample-team] [--execute]
  tmact loop --config examples/night-loop.yaml [--dry-run] [--once] [--assume-idle-on-start]
  tmact loop status [--run-dir .tmact/runs] [--json]
  tmact loop stop (--id ID | --config path)
  tmact workflow discuss --config examples/openspec-workflow.yaml [--dry-run] [--once] [--execute]
  tmact workflow implement --config examples/openspec-implementation.yaml [--dry-run] [--once] [--execute]
  tmact workflow report review --config examples/openspec-workflow.yaml --role qa --kind accept --change-hash sha256:...
  tmact workflow example
  tmact workflow status [--config examples/openspec-workflow.yaml] [--json]
  tmact workflow stop (--id ID | --config path)
  tmact watch --config examples/accept-question-watch.yaml [--dry-run] [--once]
  tmact dispatch-work SESSION --dir DIR --agent claude --prompt "..." [--ready-timeout 30s] [--ready-settle 1.5s] [--execute]
  tmact help [command] [--json]
  tmact commands [--json]
  tmact version [--json]

Commands:
  ls        list tmux panes and cache numbered targets for -t
  send      send text, a command, or keys to a selected tmux target
  detect    capture a tmux pane and detect a directory-access prompt
  inspect   detect runtime and idle/running state for tmux panes
  status    summarize configured agent panes
  statusd   maintain a cached tmux pane status snapshot
  stt-set   configure statusd web UI voice transcription
  inbox     list agent panes that need human intervention
  summarize summarize recent pane and git activity
  broadcast safely send text to selected agent panes
  panels    plan or ensure configured agent tmux panels
  loop      run, inspect, or stop a configurable tmux automation loop
  workflow  run, inspect, or stop serialized OpenSpec review and implementation workflows
  watch     watch a pane and answer allowlisted prompts
  dispatch-work create or reuse a session, launch an agent, and send it a prompt
  commands  print a machine-readable command catalog for tools and LLMs
  version   print the tmact build version

Safety:
  send, broadcast, and panels ensure default to dry-run. For loop and watch,
  validate with --dry-run --once before running a live automation.

More help:
  tmact help loop
  tmact help loop status
  tmact commands --json
`
}
