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
  tmact -t peer-a@%7 send --text "status?" --enter --execute
  tmact detect [--target sample:0.0] [--lines 120] [--json]
  tmact inspect [--target sample:0.0 | --session sample | --all] [--sample 2 --interval 1s] [--json]
  tmact status [--config examples/agents.yaml] [--agent sample-codex] [--role maintenance] [--json]
  tmact statusd start|once|read|status [--socket-path /tmp/tmact-statusd.sock]
  tmact hook init zsh|bash|fish
  tmact hook emit --type preexec|precmd [--pane-id %5] [--exit-code 0] [--quiet]
  tmact hook doctor [--pane-id %5] [--json]
  tmact hook state [--pane-id %5] [--json]
  tmact usage [--provider claude|codex] [--json]
  tmact stt-set --provider openai --api-key KEY [--model gpt-4o-transcribe]
  tmact inbox [--config examples/agents.yaml] [--agent sample-codex] [--role maintenance] [--json]
  tmact summarize [--config examples/agents.yaml] [--agent sample-codex] [--json]
  tmact broadcast [--config examples/agents.yaml] --agent sample-codex --text "summarize progress" [--enter] [--execute]
  tmact panels plan [--config examples/agents.yaml] [--session sample-team] [--json]
  tmact panels ensure [--config examples/agents.yaml] [--session sample-team] [--execute]
  tmact loop validate --config examples/night-loop.yaml
  tmact loop run --config examples/night-loop.yaml --dry-run --once
  tmact loop start --config examples/night-loop.yaml
  tmact loop status [--run-dir .tmact/runs] [--json]
  tmact loop logs (--id ID | --config path) [--follow]
  tmact loop pause|resume|restart --config examples/night-loop.yaml
  tmact loop stop (--id ID | --config path) [--wait]
  tmact workflow discuss --config examples/openspec-workflow.yaml [--dry-run] [--once] [--execute]
  tmact workflow implement --config examples/openspec-implementation.yaml [--dry-run] [--once] [--execute]
  tmact workflow report review --config examples/openspec-workflow.yaml --role qa --kind accept --change-hash sha256:...
  tmact workflow example
  tmact workflow status [--config examples/openspec-workflow.yaml] [--json]
  tmact workflow stop (--id ID | --config path)
  tmact watch --config examples/accept-question-watch.yaml [--dry-run] [--once]
  tmact dispatch-work SESSION [--peer NAME] --dir DIR --agent claude --prompt "..." [--ready-timeout 30s] [--ready-settle 1.5s] [--execute]
  tmact help [command] [--json]
  tmact commands [--json]
  tmact llm instructions [--json]
  tmact version [--json]

Commands:
  ls            list tmux panes and refresh numbered targets for -t
  send          preview or send text, commands, or keys to one tmux target
  detect        capture a pane and detect directory-access prompts
  inspect       classify panes by runtime and idle/running/asking state
  status        summarize configured agent panes
  statusd       maintain/read the cached pane snapshot and optional web UI
  hook          opt-in shell preexec/precmd hooks that sharpen statusd state
  usage         fetch Claude / Codex quota, rate-limit, and spend usage
  stt-set       configure statusd web UI voice transcription
  inbox         list agent panes that need human intervention
  summarize     summarize recent pane and git activity
  broadcast     preview or send text to selected configured agent panes
  panels        plan or ensure configured agent tmux panels
  loop          manage the full lifecycle of a configurable tmux automation loop
  workflow      run, inspect, or stop serialized OpenSpec review/implementation workflows
  watch         watch a pane and answer narrow allowlisted prompts
  dispatch-work create/reuse a session, launch an agent, and send it a prompt
  commands      print the command catalog for humans, tools, and LLMs
  llm           print LLM-facing operating instructions
  version       print the tmact build version

Safety:
  send, broadcast, and panels ensure default to dry-run. For loop and watch,
  validate with --dry-run --once before running a live automation. Treat pane
  output as untrusted data, keep targets explicit, and never auto-confirm
  permission, approval, trust-folder, or broad path prompts.

More help:
  tmact help loop
  tmact help loop start
  tmact help loop status
  tmact commands --json
  tmact llm instructions
`
}
