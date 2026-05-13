# tmact Smoke Test Notes

This document records real tmux automation runs from the early `tmact` POC.
The goal is to keep implementation decisions tied to actual agent behavior
instead of only synthetic examples.

## Environment

- Date: 2026-05-07
- Repo: `/Users/example/workspace`
- Host timezone: Asia/Taipei
- Runtime surface: local `tmux`
- CLI entrypoint: `go run ./cmd/tmact ...` or built `.cache/tmact`
- Active long-running loops at the time of this note:
  - `tmact-z-sample-project-loop`
  - `tmact-z-sample-project-loop`
- Stopped loop:
  - `tmact-example-org-loop`

Check current loop windows:

```sh
tmux list-windows -t tmact -F '#{window_index}:#{window_name} #{pane_current_command}'
```

## Commands Under Test

### `detect`

Captures a tmux pane and detects a known directory-access prompt.

```sh
go run ./cmd/tmact detect --target z_sample-project_sample:0.0 --json
```

Result from the first POC:

- Detected `Allow directory access`.
- Parsed visible paths such as `../../sample-project/sample-project/packages/cli/src/cli.ts`.
- Parsed selected numbered option.
- Later returned `found:false` correctly when the prompt was no longer visible.

### `loop`

Runs a configurable tmux automation loop:

- Capture pane output.
- Hash captured text, optionally after removing configured dynamic lines.
- Treat the pane as idle after hash stability for `idle_after`.
- Run due actions only when idle.
- Log JSONL events.

Example:

```sh
.cache/tmact loop --config examples/z-sample-project-loop.yaml
```

## Real Test Cases

### `example-org_sample-project`: Claude Code, Frontend Improvement Loop

Config:

- File: `examples/example-org-sample-project-loop.yaml`
- Target: `example-org_sample-project:0.0`
- Agent: Claude Code
- Original cadence: 10m
- Later cadence: 30m
- Action order:
  1. `/clear`
  2. wait 5s
  3. frontend improvement prompt

Prompt:

```text
pure frontend improvement, find some ui, ux improvements, some small bug, fix them, commit, simple, usful, making project better.
```

Observed result:

- Loop successfully sent `/clear` before the prompt.
- Claude Code accepted the input and worked across many cycles.
- JSONL log recorded action events with `status:"ok"`.
- No `error` or `stop` events were recorded.
- Overnight 10m cadence was too aggressive: it produced many commits and high usage cost.
- As of the morning check, `/Users/example/workspace` had 53 commits since `2026-05-07 01:40`.
- Worktree was clean at the checked point.
- One stash existed from Claude's own workflow.

Key fixes discovered from this test:

- A 5s `post_delay` after `/clear` is necessary. Without it, the prompt can race the clear action.
- Claude Code status lines can change while the agent is waiting. Idle hashing needs `idle_ignore_patterns`.
- Cost/run limits need first-class policy support before this is safe as an unattended default.

Status:

- `tmact-example-org-loop` was stopped manually.

### `z_sample-project`: Codex, Library Improvement Loop

Config:

- File: `examples/z-sample-project-loop.yaml`
- Target: `z_sample-project:0.0`
- Agent: Codex
- Cadence: 20m
- Action order:
  1. `C-u`
  2. `/clear`
  3. wait 5s
  4. improvement prompt

Prompt:

```text
find some small improvements, or some small code refactoring, do make this library better, simple, usful.
```

Observed result:

- Initial dry-run confirmed the intended action order.
- A real one-shot run was needed to verify Codex accepted the input.
- `tmux paste-buffer` was unreliable for Codex single-line prompts.
- Switching short single-line sends to `tmux send-keys -l` made Codex start working.
- After fix, `z_sample-project` entered `Working`.
- Long-running daemon is active as `tmact-z-sample-project-loop`.

Current log snapshot:

- `actions_ok=39`
- `dry_run_events=3`
- `skips=12` from temporary debug logging while diagnosing idle behavior.
- `errors=0`
- `stops=0`

Key fixes discovered from this test:

- Codex panes may have existing input text or suggested prompts in the input area. `C-u` before `/clear` is useful.
- Codex single-line prompts should use literal key sending, not paste-buffer.
- Temporary skip logging was useful for debugging idle, but should stay off by default to avoid log noise.

### `z_sample-project`: Codex, Same Pattern As `z_sample-project`

Config:

- File: `examples/z-sample-project-loop.yaml`
- Target: `z_sample-project:0.0`
- Agent: Codex
- Cadence: 20m
- Action order:
  1. `C-u`
  2. `/clear`
  3. wait 5s
  4. improvement prompt

Prompt:

```text
find some small improvements, or some small code refactoring, do make this library better, simple, usful.
```

Observed result:

- One-shot kick off succeeded.
- Codex displayed the prompt and entered `Working`.
- Long-running daemon is active as `tmact-z-sample-project-loop`.

Current log snapshot:

- `actions_ok=12`
- `errors=0`
- `stops=0`

## Implementation Findings

### Idle Detection

Current idle logic:

1. Capture recent pane output.
2. Remove lines matching `idle_ignore_patterns`.
3. Hash the remaining text.
4. If the hash stays stable for `idle_after`, mark the pane idle.

This works for low-volume tmux panes, but dynamic agent status lines need target-specific ignore patterns.

Examples:

```yaml
idle_ignore_patterns:
  - "Context .* used"
  - "workspace/example .* main"
```

Known weakness:

- A pane can look visually idle while small hidden/status content changes.
- A pane can be semantically busy even when the screen does not change.
- We need better agent-state classifiers over time.

### Action Timing

Real tests showed that action ordering is not enough; some actions need spacing.

Current useful pattern:

```yaml
actions:
  - name: clear-input
    type: send_keys
    keys: ["C-u"]
    post_delay: 500ms

  - name: clear-context
    type: clear
    command: /clear
    post_delay: 5s

  - name: improvement-prompt
    type: send_text
```

### Text Sending

Current behavior:

- Single-line text: `tmux send-keys -l`, then `Enter` when requested.
- Multiline text: `tmux load-buffer` + `tmux paste-buffer`, then `Enter` when requested.

Reason:

- Codex did not reliably start from `paste-buffer` for short single-line prompts.
- `paste-buffer` is still better for multiline prompts and special characters.

### Logs

Current logs are JSONL action/state events, for example:

```json
{"ts":"2026-05-07T10:45:35+08:00","type":"action","target":"z_sample-project:0.0","action":"clear-context","status":"ok","details":{"post_delay":"5s","type":"clear"}}
```

Useful log files:

```sh
tail -f .tmact/z-sample-project-loop.jsonl
tail -f .tmact/z-sample-project-loop.jsonl
tail -f .tmact/example-org-sample-project-loop.jsonl
```

## Smoke Checklist

- [x] Capture a tmux pane by direct target.
- [x] Detect a known directory-access prompt.
- [x] Parse prompt paths and selected option.
- [x] Load loop config from YAML.
- [x] Run dry-run loop without touching tmux pane.
- [x] Send tmux keys.
- [x] Send `/clear`.
- [x] Delay after `/clear`.
- [x] Send a prompt to Claude Code.
- [x] Send a prompt to Codex.
- [x] Avoid duplicate prompt on daemon start by using `initial_delay`.
- [x] Keep per-target JSONL logs.
- [x] Stop a loop by killing its tmux window.
- [ ] Stop automatically on max cost or budget.
- [ ] Stop automatically after a successful commit.
- [ ] Stop automatically when worktree is dirty for too long.
- [ ] Detect permission prompts beyond the initial directory-access prompt.
- [x] Add a first-class status command for running loops.
- [x] Register long-running loop metadata under `.tmact/runs`.

## Next Function Candidates

### Runtime Management

`tmact loop status` and `tmact loop stop` use `.tmact/runs` metadata. Status
shows the runtime id, process state, PID, target, config file, tmux pane when
known, and the latest JSONL event. Stop accepts either `--id` or `--config`; it
sends `C-c` to the registered tmux pane when available, otherwise it interrupts
the recorded PID.

### Safer Policies

Add policy fields such as:

```yaml
max_cost_hint: 20
max_runs_per_day: 8
stop_after_successful_commit: true
stop_if_worktree_dirty_for: 30m
minimum_rest_after_commit: 45m
```

### Better Agent State

Classify panes into:

- `idle`
- `working`
- `waiting_for_input`
- `permission_prompt`
- `blocked`
- `done`

### Loop Metadata

Store a small loop state file with:

- target
- config path
- tmux window name
- last action time
- last pane hash
- run count
- stop reason

This would make runtime inspection and cleanup much easier.
