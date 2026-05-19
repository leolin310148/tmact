# tmact

`tmact` is a local-first tmux automation CLI for driving terminal AI agents
(Codex, Claude, Copilot, Gemini) and other long-running pane workloads.

It lists tmux panes, sends text/keys to them, watches pane output for known
prompts, classifies pane runtime/idle state, and runs config-driven loops on
top of those panes.

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
| `broadcast` | Send the same text/keys to a group of configured agent panes. |
| `panels` | Plan or reconcile configured agent panes (session/window/repo/launcher) into tmux. |
| `loop` | Run a configurable polling loop on one pane: pastes prompts, presses keys, clears context, stops on known interactive approval prompts. Supports `--dry-run --once`. |
| `workflow` | Run serialized OpenSpec review and implementation workflows across configured agent panes. |
| `watch` | Narrow prompt watcher; currently accepts Codex directory-access prompts when every requested path is allowlisted. |
| `dispatch-work` | Create or reuse a tmux session, launch an agent (claude/codex/gemini/copilot), and send it a prompt. Dry-run by default; `--execute` actually creates/launches/sends. |

`loop` writes run metadata under `.tmact/runs/` so long-running processes can
be inspected and stopped without remembering the tmux window:

```sh
tmact loop status
tmact loop stop --id loop-night-loop-123
```

## Quick Start

```sh
go build -o .cache/tmact ./cmd/tmact

tmact ls
tmact -t 0 send --text "summarize progress" --enter --execute
tmact detect --target sample:0.0 --json
tmact inspect --all --json
tmact panels ensure --config examples/multi-agent-panels.yaml --execute
tmact loop --config examples/night-loop.yaml --dry-run --once
tmact workflow discuss --config examples/openspec-workflow.yaml --dry-run --once
tmact workflow implement --config examples/openspec-implementation.yaml --dry-run --once
tmact watch --config examples/accept-question-watch.yaml --dry-run --once
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
internal/runmeta/              # `.tmact/runs/` metadata for loop processes
internal/agents/               # agents.yaml config: panels, broadcast, inbox, summary
internal/loop/                 # configurable single-pane loop runner
internal/workflow/             # OpenSpec review and implementation workflows
internal/watch/                # prompt watcher (directory-access acceptor)
examples/                      # YAML configs for loops, watches, and agents
docs/                          # smoke test notes
launchd/                       # macOS launchd template for `statusd`
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

Most non-loop commands read an agents config to define agent panes, their
launcher (`codex` / `claude` / `copilot` / `gemini`), repo, role, and target.
By default `tmact` looks for `tmact.agents.yaml` or `agents.yaml` in the current
working directory. `examples/agents.yaml` is a sample only; pass it explicitly
or copy it before using it for a real setup. `panels`, `broadcast`, `status`,
`inbox`, `summarize` all share this shape.

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

Loops stop when a known interactive permission, approval, trust-folder, or
confirmation prompt is visible — safer for unattended runs.

### OpenSpec Workflows

`tmact workflow discuss` runs the proposal review phase for an OpenSpec change.
It serializes PM, SWE, QA, and reviewer prompts over the full artifact set
(`proposal.md`, `design.md`, `tasks.md`, and `specs/*/spec.md`) and gates on
the current `change_hash`.

`tmact workflow implement` runs the post-agreement phase:

```text
SWE apply -> QA verify -> PM archive
```

Live implementation requires `phase1-state.json` to have outcome `agreed` for
the same current artifact hash. Dry-runs can be configured to preview the next
prompt before phase 1 agreement, but `--execute` still enforces the agreement
precondition. Status and stop use the same surfaces:

```sh
tmact workflow status --config examples/openspec-implementation.yaml
tmact workflow stop --config examples/openspec-implementation.yaml
```

Both phases can share one config file when it contains both `discussion` and
`implementation` sections:

```sh
tmact workflow example
tmact workflow discuss --config examples/openspec-full-workflow.yaml --dry-run --once
tmact workflow implement --config examples/openspec-full-workflow.yaml --dry-run --once
```

### Watcher

`tmact watch` polls a target pane with `tmux capture-pane` and applies allow-
listed rules. The first supported rule type is `directory_access_prompt`:

```yaml
target: sample-agent:0.0
rules:
  - name: accept-sample-directory-access
    type: directory_access_prompt
    allow_paths:
      - .
    allow_path_patterns:
      - /tmp/tmact-sample-*
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
status-line refresh path. On macOS, `scripts/install.sh` generates a per-user
LaunchAgent from `launchd/com.tmact.statusd.plist.in`. Design notes in
`daemon-status.md`.

## State And Logs

```text
.cache/tmact                       # built binary
.cache/tmact-targets.json          # numbered target cache from `ls`
.tmact/runs/                       # loop run metadata + status
.tmact/<name>.jsonl                # ad-hoc event logs for long runs
openspec/changes/*/phase*.json     # workflow phase state
openspec/changes/*/phase*-comments.jsonl # workflow comment streams
/tmp/tmact-status.json             # statusd snapshot (see launchd plist)
/tmp/tmact-statusd.{jsonl,*.log}   # statusd logs
```

## Safety

- Sends are dry-run by default; `--execute` is required to press keys.
- Watcher decisions enforce allowlists; never widen them silently.
- Loops and workflows stop on known interactive permission or approval prompts
  rather than auto-confirming.
- Terminal output is treated as untrusted input; do not feed pane text to an
  LLM without wrapping it as observed terminal output.
- Cooldowns, max-runs, and dedupe hashes are first-class in loop/watch configs.

## Running Background Loops

Long-running daemons go in the detached tmux session `tmact-loops`, not the
working `tmact` session. See `RUNNING_LOOPS.md` for the live inventory and
inspection commands.

## Further Reading

- `AGENTS.md` — contributor guide (build, test, style, PR conventions).
- `docs/smoke-test.md` — manual smoke-test notes.
- `daemon-status.md` — `statusd` design and tmux integration plan.
- `RUNNING_LOOPS.md` — currently-active background loops.
