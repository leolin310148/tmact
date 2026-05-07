# tmact

`tmact` is a local-first tmux automation CLI.

It manages tmux panes/windows, sends scheduled text or keys, watches pane output for text patterns, and triggers configured actions. The tool should stay focused on local tmux state and expose optional interfaces for external orchestrators such as n8n.

## Goals

- Provide a short, fast CLI for everyday tmux operations.
- Replace and extend the current interactive `tmux-send.sh` workflow.
- Support scheduled sends such as now, after a delay, at a specific time, and recurring intervals.
- Detect content in tmux panes and trigger actions when patterns appear.
- Support actions such as sending text, pressing keys, running sequences, notifying, and later calling LLM agents.
- Keep safety controls local: target allowlists, cooldowns, dedupe, logs, and explicit action definitions.
- Keep n8n optional. n8n can call `tmact`, but tmux-specific state and action execution should live in `tmact`.

## Non-Goals For The First Version

- Distributed scheduling across machines.
- Full web dashboard.
- Complex workflow engine.
- Replacing tmux itself.
- Letting arbitrary remote callers press keys without local auth.

## Recommended Stack

- Language: Go
- CLI: `github.com/spf13/cobra`
- Scheduler: `github.com/go-co-op/gocron/v2`
- Config: YAML via `gopkg.in/yaml.v3`
- Persistence: SQLite
- Logging: Go `log/slog` plus JSONL action logs
- Template rendering: Go `text/template`
- Regex matching: Go `regexp`

`gocron/v2` is a good fit because it supports duration, cron, daily/weekly/monthly, and one-time jobs. `robfig/cron/v3` is also solid, but it is more cron-focused and would need more wrapper code for human-friendly one-shot schedules.

## Command Shape

```sh
tmact list
tmact list --json

tmact send --to ops:0.0 "continue"
tmact send --to ops:0.0 --enter "continue"
tmact send --interactive

tmact key --to hc-api:0.0 Enter
tmact key --to hc-api:0.0 1 Enter

tmact schedule add --to ops:0.0 --after 10m --text "continue" --enter
tmact schedule add --to hc-api:0.0 --at 23:30 --keys "1,Enter"
tmact schedule add --every 1h --to ops:0.0 --text "status" --enter
tmact schedule list
tmact schedule cancel <id>

tmact watch
tmact rules test --rule build-failed --target hc-api:0.0

tmact run
tmact serve --addr 127.0.0.1:8765
```

## Current POC: Tmux Agent Loop

The first runnable slice is intentionally tmux-only:

```sh
tmact detect --target z_sample-project_sample:0.0 --json
tmact loop --config examples/night-loop.yaml --dry-run --once
tmact loop --config examples/night-loop.yaml
```

`detect` captures a pane and detects a known directory-access prompt.

`loop` is a configurable overnight harness for terminal agents. It repeatedly captures
one tmux pane, tracks whether output has changed, waits for idle windows, and runs
configured actions such as:

- `send_text`: paste a prompt and optionally press Enter.
- `send_keys`: send tmux key names.
- `clear`: send `/clear` or another configured clear command.

The loop is config-driven rather than hard-coded. Each action can set:

- `every`
- `initial_delay`
- `only_when_idle`
- `max_runs`
- `enter`

The loop can also stop when a permission prompt is visible, which is safer than
silently granting access while an agent is running unattended.

Real smoke test notes are tracked in `docs/smoke-test.md`.

## Main Concepts

### Targets

Targets are named groups or direct tmux pane/window selectors.

Examples:

```yaml
targets:
  ops:
    panes:
      - "ops:0.0"

  hc-api:
    match: "hc-api:*"

  all-agents:
    panes:
      - "hc-api:0.0"
      - "ops:0.0"
```

Target selectors should support:

- `session`
- `session:window`
- `session:window.pane`
- named target groups from config
- glob-like patterns such as `hc-api:*`

Internally, resolve targets through:

```sh
tmux list-panes -a -F '#{pane_id} #{session_name}:#{window_index}.#{pane_index} #{window_name} #{pane_current_path}'
```

Prefer pane IDs internally after resolution because `session:window.pane` can shift.

### Actions

Everything that changes state should be represented as an action.

Initial action types:

- `send`: send text to a pane, optionally press Enter.
- `key`: send tmux key names such as `Enter`, `C-c`, `1`, `2`.
- `wait`: sleep between sequence steps.
- `sequence`: run multiple actions in order.
- `notify`: local notification or log-only message.

Later action types:

- `llm`: send matched context to an agent pane or CLI.
- `webhook`: call a configured URL.
- `shell`: run allowlisted local commands only.

Example:

```yaml
actions:
  choose-one:
    type: sequence
    steps:
      - type: key
        to: "{{source}}"
        keys: ["1"]
      - type: wait
        duration: "500ms"
      - type: key
        to: "{{source}}"
        keys: ["Enter"]

  send-agent-context:
    type: send
    to: "ops:0.0"
    text: |
      Detected in {{source}}:

      {{match}}

      Context:
      {{context}}
    enter: true
```

For multiline text, prefer `tmux load-buffer` plus `tmux paste-buffer` instead of raw `send-keys`, because it is safer for long prompts and special characters.

### Scheduler

Scheduling should live in `tmact`, not in n8n.

Reasoning:

- Scheduling needs local tmux target resolution.
- Jobs should survive restarts.
- Jobs need action logs and failure handling.
- `tmact` needs cooldowns, dedupe, and allowlists close to the action runner.

Use SQLite as the durable source of scheduled jobs. The daemon registers enabled jobs with `gocron` on startup.

Suggested table:

```sql
create table scheduled_jobs (
  id text primary key,
  enabled integer not null,
  schedule_type text not null,
  run_at text,
  interval_seconds integer,
  cron_expr text,
  action_json text not null,
  last_run_at text,
  next_run_at text,
  created_at text not null,
  updated_at text not null
);
```

Supported schedule types:

- `once`
- `interval`
- `cron`
- `daily`

First implementation can keep reload simple:

- `tmact run` reloads SQLite schedules periodically, for example every 5 seconds.
- Later, add a local socket or HTTP reload endpoint so `tmact schedule add` can notify the daemon immediately.

### Watcher

MVP watcher should use polling:

```sh
tmux capture-pane -t "$pane" -p -J -S -120
```

Default settings:

- Poll interval: 3s to 5s.
- Capture lines: 100 to 200.
- Scope: configured targets only.
- Hash pane content and skip matching if unchanged.
- Cooldown per rule and pane.
- Dedupe by hash of matched context.

Polling is acceptable for tens of panes if capture size and interval are bounded. The expensive part is usually calling an LLM or triggering repeated actions, not `capture-pane`.

Later, high-volume panes can use `tmux pipe-pane` for event-driven output:

```sh
tmux pipe-pane -t "$pane" -o "tmact ingest --pane '$pane'"
```

`pipe-pane` is lower-latency but more complex because it only sees output after the pipe starts and needs lifecycle management when panes restart.

### Rules

Rules connect watcher matches to actions.

Example:

```yaml
rules:
  - name: build-failed
    watch: "hc-api"
    match: "BUILD FAILED|Traceback|Exception"
    cooldown: "2m"
    context_lines: 80
    action:
      type: send
      to: "ops:0.0"
      text: |
        Detected issue in {{source}}:

        {{context}}
      enter: true

  - name: choose-yes
    watch: "some-agent"
    match: "Do you want to continue\\?"
    cooldown: "30s"
    action:
      type: sequence
      steps:
        - type: key
          to: "{{source}}"
          keys: ["1"]
        - type: wait
          duration: "500ms"
        - type: key
          to: "{{source}}"
          keys: ["Enter"]
```

Template variables:

- `{{source}}`: source tmux target or pane ID.
- `{{session}}`
- `{{window}}`
- `{{pane}}`
- `{{match}}`
- `{{context}}`
- `{{rule}}`
- `{{triggered_at}}`

### State And Logs

Suggested local files:

```text
~/.local/share/tmact/state.sqlite
~/.local/state/tmact/actions.jsonl
~/.config/tmact/config.yaml
```

For repo-local development, allow overrides:

```sh
tmact --config ./config.yaml --state ./state.sqlite run
```

Important state:

- scheduled jobs
- watcher dedupe hashes
- cooldown timestamps
- action history
- trigger history

JSONL action log example:

```json
{"ts":"2026-05-06T12:00:00+08:00","type":"key","to":"hc-api:0.0","keys":["Enter"],"status":"ok"}
```

### Safety

Required from the beginning:

- Default HTTP API binds only to `127.0.0.1`.
- HTTP API should require a token if enabled.
- Shell actions must be disabled by default.
- Shell actions, if added, must use an explicit allowlist.
- Rules need cooldowns.
- Watcher should dedupe repeated matches.
- `tmact rules test` should show what would trigger without executing actions.
- `--dry-run` should exist for send, key, schedule, and rule testing.

Terminal output should be treated as untrusted input. If sending context to an LLM agent, wrap it clearly as observed terminal output and instruct the agent not to treat it as direct instructions.

## n8n Boundary

Recommended split:

```text
tmact = local tmux brain and hands
n8n = optional external workflow orchestrator
```

`tmact` should own:

- tmux target state
- send/key execution
- watcher
- local schedules
- cooldown and dedupe
- action logs

n8n can own:

- cross-service workflows
- Slack, GitHub, Notion, email, calendar integrations
- remote webhooks that call `tmact serve`
- higher-level orchestration

Optional HTTP endpoints:

```http
GET  /panes
GET  /jobs
POST /send
POST /key
POST /actions/run
POST /schedule
GET  /triggers
```

The HTTP layer should be a thin wrapper around the same action runner used by the CLI and watcher.

## Implementation Plan

### Phase 1: CLI Foundation

- Create Go module.
- Add Cobra root command.
- Implement tmux client wrapper:
  - list sessions/windows/panes
  - capture pane
  - send keys
  - paste text
- Implement `tmact list`.
- Implement direct `tmact send`.
- Implement direct `tmact key`.

### Phase 2: Config And Targets

- Add YAML config loader.
- Add target resolver for direct targets and named groups.
- Support `--to <target>`.
- Add `--json` output for `list`.

### Phase 3: Action Runner

- Define action structs.
- Implement `send`, `key`, `wait`, `sequence`, `notify`.
- Add template rendering.
- Add dry-run mode.
- Add JSONL action log.

### Phase 4: Scheduler

- Add SQLite store.
- Add `schedule add/list/cancel`.
- Add `tmact run` daemon mode.
- Register persisted jobs into `gocron`.
- Execute actions from scheduled jobs.
- Track last/next run.

### Phase 5: Watcher

- Add `tmact watch`.
- Poll configured targets with `capture-pane`.
- Implement regex rules.
- Add content hash skip.
- Add cooldown and dedupe.
- Trigger action runner.
- Add `rules test`.

### Phase 6: Optional API

- Add `tmact serve --addr 127.0.0.1:8765`.
- Add token auth.
- Add endpoints for panes, send, key, schedule, and action run.
- Keep API as a thin wrapper over core services.

### Phase 7: Launchd

- Add a launchd plist template for macOS.
- Run `tmact run` as a user agent.
- Add commands:
  - `tmact service install`
  - `tmact service start`
  - `tmact service stop`
  - `tmact service logs`

## Suggested Repo Layout

```text
cmd/tmact/
  main.go

internal/tmux/
  client.go
  target.go

internal/config/
  config.go

internal/actions/
  action.go
  runner.go
  template.go

internal/scheduler/
  scheduler.go

internal/watcher/
  watcher.go
  rules.go

internal/store/
  sqlite.go
  migrations.go

internal/logs/
  jsonl.go

internal/server/
  server.go

examples/
  config.yaml
  launchd.plist
```

## MVP Acceptance Criteria

- `tmact list` shows all panes with stable pane IDs and human-readable targets.
- `tmact send --to ops:0.0 "hello" --enter` works.
- `tmact key --to ops:0.0 Enter` works.
- `tmact schedule add --after 10m ...` persists and runs via `tmact run`.
- `tmact watch` can detect a configured regex in a target pane.
- Watcher does not repeatedly trigger the same visible match.
- Every executed action is logged.
- Dry-run is available for risky operations.

## Open Questions

- Should config default to repo-local `./tmact.yaml` or user-level `~/.config/tmact/config.yaml`?
- Should first release include HTTP API, or leave it for after CLI and daemon are stable?
- Should scheduled jobs be editable by ID, or should cancel and recreate be enough initially?
- Should `tmact run` include both scheduler and watcher by default, or should they be separately enabled?
- Should target aliases be required for watcher rules, or allow direct tmux patterns everywhere?
