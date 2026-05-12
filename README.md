# tmact

`tmact` is a local-first tmux automation CLI for driving terminal AI agents
(Codex, Claude, Copilot, Gemini) and other long-running pane workloads.

It lists tmux panes, sends text/keys to them, watches pane output for known
prompts, classifies pane runtime/idle state, and runs config-driven loops and
multi-stage workflows on top of those panes.

## What's Built

The CLI is a single Go binary (`cmd/tmact`) with these subcommands:

| Command | Purpose |
| --- | --- |
| `ls` | List tmux panes; caches numbered targets in `.cache/tmact-targets.json` so `-t 0` works in follow-up calls. |
| `send` | Send text, a command, or tmux keys to a target pane. Dry-run by default; `--execute` actually presses keys. |
| `detect` | Capture a pane and detect a directory-access permission prompt. |
| `inspect` | Detect runtime (codex/claude/copilot/gemini/shell/…) and idle-vs-running for one pane, a session, or all panes. |
| `status` / `inbox` / `summarize` | Roll up configured agent panes into a status view, an attention-needed inbox, or a recent-activity summary. |
| `statusd` | Long-running daemon that publishes pane snapshots to `/tmp/tmact-status.json` and optional tmux session options so the tmux status line can read cheaply. See `daemon-status.md`. |
| `state` | Read/write/transition the YAML status files under `.agent-inbox/features/<name>/status.yaml` and append JSONL events. |
| `broadcast` | Send the same text/keys to a group of configured agent panes. |
| `panels` | Plan or reconcile configured agent panes (session/window/repo/launcher) into tmux. |
| `loop` | Run a configurable polling loop on one pane: pastes prompts, presses keys, clears context, stops on permission prompts. Supports `--dry-run --once`. |
| `watch` | Narrow prompt watcher; currently accepts Codex directory-access prompts when every requested path is allowlisted. |
| `workflow` | Staged harness for multi-step flows (planner → SWE → reviewer, or N improvement passes then review/fix/push). Stages can run across multiple panes via `.agent-inbox`. |

`loop` and `workflow` write run metadata under `.tmact/runs/` so long-running
processes can be inspected and stopped without remembering the tmux window:

```sh
tmact loop status
tmact loop stop --id loop-night-loop-123
tmact workflow stop --config examples/simple-improvement-workflow.yaml
```

## Quick Start

```sh
go build -o .cache/tmact ./cmd/tmact

tmact ls
tmact -t 0 send --text "summarize progress" --enter --execute
tmact detect --target z_sample-project_sample:0.0 --json
tmact inspect --all --json
tmact panels ensure --config examples/idll-agents.yaml --execute
tmact loop --config examples/night-loop.yaml --dry-run --once
tmact watch --config examples/accept-question-watch.yaml --dry-run --once
tmact workflow --config examples/simple-improvement-workflow.yaml --dry-run --once --assume-idle-on-start
```

Sends are dry-run by default — add `--execute` to actually press keys or paste
text. Targets accept the cache index (`-t 0`), a tmux pane id (`%42`), or
`session:window.pane`.

## Stack

- Go 1.26, stdlib `flag` for CLI parsing.
- Only external dependency: `gopkg.in/yaml.v3`.
- All state lives on the local filesystem; there is no daemon DB.

## Repo Layout

```text
cmd/tmact/main.go             # CLI entrypoint; subcommand dispatch and flags
internal/tmux/                # tmux command wrappers (list, capture, send, options)
internal/prompt/               # directory-access prompt detection
internal/panestate/            # classify pane runtime + idle/running/asking
internal/panestatus/           # pane snapshot + status rollup
internal/statusd/              # status daemon (writes JSON snapshot + tmux options)
internal/runmeta/              # `.tmact/runs/` metadata for loop/workflow processes
internal/state/                # agent-inbox status.yaml + JSONL event log
internal/agents/               # agents.yaml config: panels, broadcast, inbox, summary
internal/loop/                 # configurable single-pane loop runner
internal/watch/                # prompt watcher (directory-access acceptor)
internal/workflow/             # multi-stage workflow runner
examples/                      # YAML configs for loops, watches, workflows, agents
docs/                          # agent-inbox protocol, smoke test notes
launchd/                       # macOS launchd plist for `statusd`
```

## Key Concepts

### Targets

A target is one of:

- A numbered index from `tmact ls` (consumed via `-t 0`), cached at
  `.cache/tmact-targets.json`.
- A direct tmux pane id (`%42`), preferred internally.
- `session:window.pane` or `session:window`.

Pane ids are stable; `session:window.pane` is not — resolve early, store the id.

### agents.yaml

Most non-loop commands read an agents config (defaults to `examples/agents.yaml`)
to define agent panes, their launcher (`codex` / `claude` / `copilot` /
`gemini`), repo, role, and target. `panels`, `broadcast`, `status`, `inbox`,
`summarize` all share this shape.

### Loops

`tmact loop` runs a single-pane polling loop. Each action sets:

- `every`, `initial_delay`, `max_runs`
- `only_when_idle` — skip when the pane is still working
- `enter`, `post_delay`

Action types: `send_text`, `send_keys`, `clear` (sends `/clear`).

Related actions can be grouped into a `flow` whose timing/idle rules apply once
to the whole sequence:

```yaml
flows:
  - name: improvement-cycle
    every: 20m
    initial_delay: 20m
    only_when_idle: true
    max_runs: 24
    steps:
      - { name: clear-input,   type: send_keys, keys: ["C-u"], post_delay: 500ms }
      - { name: clear-context, type: clear,                   post_delay: 5s    }
      - name: improvement-prompt
        type: send_text
        enter: true
        text: |
          Continue with one small scoped improvement.
```

Loops stop when a permission prompt is visible — safer for unattended runs.

### Workflows

`tmact workflow` is a staged runner. Each stage waits for its pane to become
idle, sends a role-specific prompt, and advances only when its `complete_when`
conditions match. Stages can:

- Override the default target (planner/SWE/reviewer on different panes).
- `repeat` for maintenance loops (e.g. 5 improvement passes then 1 review pass).
- `stage_every` to enforce a minimum delay between stage starts.
- Use `complete_when.state_path` + `state_in` as an additive completion source,
  resolved relative to the workflow `repo`.

For unattended feature work, keep stages going past technical review:
UAT/player feedback → stakeholder acceptance → feedback fixes → commit-check.
The commit-check stage should commit only cohesive accepted diffs and defer
oversized or mixed changes.

See `docs/agent-inbox.md`, `examples/implement-review-workflow.yaml`, and
`examples/simple-improvement-workflow.yaml`.

### Watcher

`tmact watch` polls a target pane with `tmux capture-pane` and applies allow-
listed rules. The first supported rule type is `directory_access_prompt`:

```yaml
target: z_sample-project_sample:0.0
rules:
  - name: accept-sample-project-directory-access
    type: directory_access_prompt
    allow_paths:
      - /Users/example/workspace
    allow_path_patterns:
      - /tmp/sample-project-rn-*
    accept_option: selected
    cooldown: 30s
    max_runs: 10
```

If any requested path is outside `allow_paths` and does not match
`allow_path_patterns` (Go filepath glob), the watcher logs a blocked decision
and does not press a key.

### Status Daemon

`statusd` continuously classifies panes and writes a JSON snapshot plus
optional tmux session options (`@ai-tag`, `@ai-running`, `@ai-asking`). The
goal is to move expensive detection out of `#()` shell commands on the tmux
status-line refresh path. Install via `launchd/com.tmact.statusd.plist`
(macOS). Design notes in `daemon-status.md`.

### Agent Inbox

`.agent-inbox/features/<name>/` is a file handoff protocol for multi-agent
async work. Each feature has a `status.yaml` (state machine, owner, stage) and
a JSONL event log written by `tmact state`. Workflow stages consult these to
decide when to advance. Full protocol in `docs/agent-inbox.md`.

## State And Logs

```text
.cache/tmact                       # built binary
.cache/tmact-targets.json          # numbered target cache from `ls`
.tmact/runs/                       # loop/workflow run metadata + status
.tmact/<name>.jsonl                # ad-hoc event logs for long runs
/tmp/tmact-status.json             # statusd snapshot (see launchd plist)
/tmp/tmact-statusd.{jsonl,*.log}   # statusd logs
.agent-inbox/features/<name>/      # workflow status + JSONL events
```

## Safety

- Sends are dry-run by default; `--execute` is required to press keys.
- Watcher decisions enforce allowlists; never widen them silently.
- Loops stop on permission prompts rather than auto-confirming.
- Terminal output is treated as untrusted input; do not feed pane text to an
  LLM without wrapping it as observed terminal output.
- Cooldowns, max-runs, and dedupe hashes are first-class in loop/watch configs.

## Running Background Loops

Long-running daemons go in the detached tmux session `tmact-loops`, not the
working `tmact` session. See `RUNNING_LOOPS.md` for the live inventory and
inspection commands.

## Further Reading

- `AGENTS.md` — contributor guide (build, test, style, PR conventions).
- `docs/agent-inbox.md` — multi-agent handoff protocol.
- `docs/smoke-test.md` — manual smoke-test notes.
- `daemon-status.md` — `statusd` design and tmux integration plan.
- `RUNNING_LOOPS.md` — currently-active background workflows.
