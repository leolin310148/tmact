# Daemon Status Proposal

## Summary

Add a long-running status daemon to `tmact` that continuously observes local
tmux panes, classifies what CLI/runtime is active, estimates whether each pane
is idle or working, detects common permission/approval prompts, and publishes a
small status snapshot for tmux status lines and other local tools.

The main goal is to move expensive and stateful detection work out of tmux
`#()` status commands. The tmux status line should become a cheap reader of
already-computed state.

Recommended shape:

```sh
tmact statusd start
tmact statusd once
tmact statusd stop
tmact statusd status
tmact statusd read --json
```

The daemon should write an atomic JSON snapshot, and should optionally update
tmux session options such as `@ai-tag`, `@ai-running`, and `@ai-asking`
directly.

## Problem

The current tmux status line implementation does useful work but does it in the
wrong execution context:

- tmux periodically invokes a shell script from `status-format` using `#()`.
- That script lists sessions, inspects process trees, captures pane text,
  hashes output, and updates tmux options.
- The script runs on the status refresh path, so any slow command can delay or
  jitter status rendering.
- Runtime detection and idle detection are spread across shell scripts instead
  of reusing `tmact`'s Go classifiers.
- The status line has to be conservative about sampling because sleeps or
  multiple captures per pane are too expensive inside a status command.

This is especially visible when many panes are open. Commands such as `ps`,
`pgrep`, `tmux capture-pane`, and JSON/text parsing are individually small, but
they become expensive when repeated every status interval.

## Goals

- Keep the tmux status line fast and predictable.
- Reuse `tmact`'s richer runtime, idle, and prompt classification logic.
- Preserve the current tmux status UI behavior and compact tags.
- Track state across ticks so running/idle classification can use debounce and
  previous snapshots.
- Provide a machine-readable status file for debugging and integration.
- Support a direct tmux-option publishing mode for maximum status-line speed.
- Fail soft when tmux restarts, panes disappear, process inspection fails, or
  the daemon is stale.
- Keep all behavior local-first and safe. This daemon observes state; it should
  not press keys or approve prompts.

## Non-Goals

- Replacing `tmact watch`, `loop`, or `workflow`.
- Running prompt-acceptance actions from the status daemon.
- Building a web dashboard.
- Distributed state across machines.
- Persisting long-term history. JSONL logs may be useful for debugging, but the
  status daemon's primary output is the latest snapshot.

## Current Behavior To Preserve

The current tmux status line shows sessions in three rows. Each session label
includes:

- a compact runtime tag:
  - `cc` for Claude
  - `cx` for Codex
  - `cp` for Copilot
  - `g` for Gemini
  - `$` for shell/unknown
- a running indicator when pane output changed recently
- an asking indicator when a known prompt appears

The current tmux options are:

```text
@ai-tag
@ai-running
@ai-asking
@row-bucket
```

The daemon should preserve these options so `.tmux.conf` can stay mostly
unchanged.

## Proposed Architecture

```text
                 +-------------------------+
                 | tmact statusd daemon    |
                 |                         |
                 | - list tmux panes       |
                 | - inspect process tree  |
                 | - capture pane text     |
                 | - classify runtime      |
                 | - classify state        |
                 | - debounce running      |
                 | - detect prompts        |
                 +-----------+-------------+
                             |
             +---------------+----------------+
             |                                |
             v                                v
  /tmp/tmact-status.json          tmux set-option per session
  atomic snapshot                 @ai-tag / @ai-running / @ai-asking
             |
             v
  tiny statusline helper, other local tools, debugging
```

There are two useful publishing modes:

1. File-only mode:
   - `statusd` writes `/tmp/tmact-status.json`.
   - tmux status invokes a small helper that reads the JSON and updates tmux
     options or formats text.

2. Tmux-option mode:
   - `statusd` writes `/tmp/tmact-status.json`.
   - `statusd` also directly calls `tmux set-option -t <session> ...`.
   - tmux status line only renders existing tmux options.

Tmux-option mode is the preferred runtime mode because it removes JSON parsing
from the status line path. File output should still be kept for observability.

## Proposed Commands

### `tmact statusd start`

Run the status daemon until interrupted.

```sh
tmact statusd start
tmact statusd start --interval 1s
tmact statusd start --state-path /tmp/tmact-status.json
tmact statusd start --tmux-options
tmact statusd start --no-tmux-options
tmact statusd start --log-path /tmp/tmact-statusd.jsonl
```

Suggested flags:

```text
--interval duration             scan interval, default 1s or 2s
--state-path path               latest JSON snapshot path
--log-path path                 optional JSONL daemon log
--tmux-options                  write @ai-* tmux options, default true
--no-tmux-options               only write the state file
--capture-lines int             lines per pane, default 120
--running-debounce duration     keep running indicator after changes, default 5s
--stale-after duration          mark daemon/file stale after this age, default 10s
--idle-ignore regexp            extra line ignore pattern, repeatable
--session glob                  include sessions matching glob, repeatable
--exclude-session glob          exclude sessions matching glob, repeatable
--once                          run one scan then exit
--json                          print the written snapshot
```

### `tmact statusd once`

Convenience alias for a single scan.

```sh
tmact statusd once --json
tmact statusd once --tmux-options
```

This should be useful for smoke tests and for comparing behavior with
`tmact inspect --all`.

### `tmact statusd read`

Read and validate the latest status file.

```sh
tmact statusd read
tmact statusd read --json
tmact statusd read --state-path /tmp/tmact-status.json
```

Human output can be compact:

```text
ts: 2026-05-12T10:50:39+08:00 age: 1.2s
sample:0.0    codex   working  tag:cx running:true asking:false
review:0.0    claude  idle     tag:cc running:false asking:false
shell:0.0     shell   idle     tag:$  running:false asking:false
```

### `tmact statusd status`

Report daemon health from the state file and optional pid/lock file.

```text
state_path: /tmp/tmact-status.json
last_update: 2026-05-12T10:50:39+08:00
age: 1.2s
stale: false
panes: 16
errors: 0
tmux_options: enabled
```

### `tmact statusd stop`

Optional. This is useful only if `statusd start --background` or a launchd
helper is added. A first implementation can omit background management and rely
on tmux/launchd/shell supervision.

## Status Snapshot Schema

The status file should be a single JSON document written atomically.

Example:

```json
{
  "version": 1,
  "ts": "2026-05-12T10:50:39+08:00",
  "generated_by": "tmact statusd",
  "interval_ms": 1000,
  "stale_after_ms": 10000,
  "tmux": {
    "server_pid": 12345,
    "socket": "/tmp/tmux-UID/default"
  },
  "summary": {
    "sessions": 14,
    "panes": 16,
    "working": 3,
    "asking": 0,
    "errors": 0
  },
  "sessions": {
    "sample": {
      "session": "sample",
      "active_target": "sample:0.0",
      "tag": "cx",
      "runtime": "codex",
      "state": "working",
      "running": true,
      "asking": false,
      "stale": false,
      "row_bucket": 1,
      "updated_at": "2026-05-12T10:50:39+08:00"
    }
  },
  "panes": {
    "sample:0.0": {
      "target": "sample:0.0",
      "pane_id": "%10",
      "session": "sample",
      "window_index": 0,
      "window": "codex-aarch64-a",
      "pane_index": 0,
      "cwd": "/path/to/sample-project",
      "current_command": "codex-aarch64-a",
      "pane_pid": 69606,
      "runtime": "codex",
      "tag": "cx",
      "state": "working",
      "idle": false,
      "running": true,
      "asking": false,
      "confidence": "high",
      "signals": [
        "child_process",
        "pane_current_command",
        "changed_capture"
      ],
      "last_line": "~/work/sample-project · 5h 92% · Context 34% used · 258K window",
      "last_changed_at": "2026-05-12T10:50:38+08:00",
      "updated_at": "2026-05-12T10:50:39+08:00"
    }
  },
  "errors": []
}
```

Notes:

- `version` allows future schema changes.
- `sessions` is derived from panes and optimized for tmux status display.
- `panes` preserves detailed information for debugging and other tools.
- `tag` is the compact display value used by tmux.
- `running` means "changed recently within debounce", not necessarily
  "process is consuming CPU".
- `state` is the classifier result (`idle`, `working`,
  `waiting_permission`, `blocked`, `unknown`).
- `asking` should be true for permission/approval prompts that need human input.
- `stale` should be true only when reading an old snapshot; the writer normally
  writes fresh snapshots with `stale: false`.

## Tag Mapping

Recommended mapping:

```text
claude   -> cc
codex    -> cx
copilot  -> cp
gemini   -> g
tmact    -> tm
shell    -> $
unknown  -> ?
```

Compatibility mode can map `unknown` to `$` to preserve the current UI.

`tmact` deserves its own tag because live loops currently look like shell in the
existing detector. Showing `tm` makes background automation panes easier to
distinguish from ordinary shells.

## State Model

Runtime and state should remain separate:

- `runtime`: what is running in or under the pane (`codex`, `claude`, `shell`).
- `state`: what the captured UI appears to be doing (`idle`, `working`,
  `waiting_permission`, `blocked`, `unknown`).
- `running`: whether pane content changed recently after ignoring volatile
  lines.
- `asking`: whether a known prompt needs attention.

This matters because a pane can be:

- `runtime=claude`, `state=unknown`, `running=false`
- `runtime=codex`, `state=idle`, `running=true` briefly after a footer refresh
- `runtime=tmact`, `state=idle`, `running=false`
- `runtime=shell`, `state=working`, `running=true`

The tmux status line usually only needs `tag`, `running`, and `asking`. The
detailed fields should stay available for debugging.

## Detection Strategy

### Runtime Detection

Use the existing `internal/panestatus` logic:

- inspect child process tree from the pane PID
- fall back to `pane_current_command`
- fall back to window name
- fall back to pane text
- fall back to shell prompt detection

The daemon should treat process inspection failures as a soft signal failure,
not as a pane failure. If `ps`/`pgrep` is unavailable or blocked, detection
should still use tmux metadata and pane text.

### Working/Idle Detection

Use persistent per-pane state:

```text
pane_id -> previous_hash
pane_id -> last_changed_at
pane_id -> previous_runtime
pane_id -> previous_state
```

On each tick:

1. Capture pane text.
2. Normalize it for idle detection.
3. Hash normalized text.
4. Compare with previous hash.
5. If changed, set `last_changed_at = now`.
6. Set `running = now - last_changed_at <= running_debounce`.
7. Combine this with text classification to decide `state`.

The daemon should prefer stable pane IDs such as `%10` for internal state. The
human target (`session:window.pane`) should be recomputed on each tick because
window indexes can move.

### Shell Hook Events (opt-in)

Shells that source `tmact hook init zsh|bash|fish` emit structured
preexec/precmd events (`tmact hook emit` → `POST /api/hook-event` on the
daemon's unix socket, `internal/shellhook`). Per pane, the daemon keeps the
latest active (unfinished preexec) and completed (matching precmd) command
and overlays that onto the snapshot each tick, before tmux-option publishing
and peer merging:

- Active command → `running`/`working`, signals `shell_hook`,
  `shell_hook_active`.
- Completed command → `idle`/`input_ready`, signals `shell_hook`,
  `shell_hook_completed` — even while the capture-hash debounce would still
  say running.
- Capture evidence outranks hooks where stronger: an asking pane keeps every
  capture-derived field (only signals are added), a `working_text`
  classification is not downgraded by a completed hook command, and an
  active command past a short grace window loses to a capture that shows an
  input-ready prompt (covers lost precmd emits — sends are fire-and-forget —
  and foreground TUIs sitting at their own input prompt).
- Precmd events are order-checked: a delayed precmd whose command_id
  mismatches the active command and whose timestamp predates its start is
  dropped instead of clearing the newer command.
- Panes without hook events keep the capture-based behavior above unchanged.
- `/api/hook-event` is IPC-only: requests over a TCP listener are rejected
  (403); only the unix socket accepts hook events.

### Lines To Ignore Before Hashing

The existing `panestatus.DefaultIdleIgnorePatterns` is a good start:

```text
(?i)\bcontext [0-9]+% used\b
(?i)\bcost:\s*\$
(?i)\btoken usage:\b
(?i)\b[0-9]+k/[0-9]+[kmg]\b
```

The daemon should extend this list as real false positives appear. Likely
additions:

```text
(?i)\b5h [0-9]+%\b
(?i)\b[0-9]+% used\b
(?i)\b[0-9]+k window\b
(?i)\belapsed\b
(?i)\bthinking for\b
```

Be careful not to ignore real work output. These patterns should target status
footers and transient counters, not arbitrary agent text.

### Prompt Detection

Initial `asking` detection should include:

- directory access prompt from `internal/prompt`
- Claude folder trust prompt
- common yes/no prompts
- "Allow command?"
- "Apply this patch?"
- "waiting for approval"
- "waiting for confirmation"

`asking` should be true when a human decision is useful. It should not imply
that the daemon will answer the prompt.

## Tmux Option Publishing

When `--tmux-options` is enabled, the daemon should set per-session options:

```sh
tmux set-option -t "$session" @ai-tag "$tag"
tmux set-option -t "$session" @ai-running "$running_glyph_or_empty"
tmux set-option -t "$session" @ai-asking "$asking_glyph_or_empty"
tmux set-option -t "$session" @row-bucket "$bucket"
```

Session-level status is derived from the active pane of that session by default,
because the current UI labels sessions rather than every pane.

If a session has multiple panes, future options could support:

- active pane only
- any pane working
- any pane asking
- priority order: asking > working > active

Recommended default:

```text
tag: active pane runtime
running: active pane running
asking: any pane in session asking
```

This keeps the label intuitive while still surfacing urgent prompts.

## Row Bucket Assignment

The current script sorts sessions by name and assigns a row bucket by
alphabetical slice:

```text
bucket = index * 3 / total
```

The daemon can preserve this exactly. Put the computed bucket in both the JSON
snapshot and `@row-bucket`.

Later, a config file could allow pinned row groups, but that is not needed for
the first daemon implementation.

## Atomic File Writes

The daemon must never leave a partially-written JSON file.

Recommended write algorithm:

1. Marshal JSON in memory.
2. Write to `state_path + ".tmp.<pid>"`.
3. `fsync` the file if practical.
4. Close the file.
5. Rename temp file to `state_path`.

On POSIX filesystems, rename within the same directory is atomic.

If writing fails, keep the previous good state file in place and log an error.

## Staleness

Readers should treat a status file as stale when:

```text
now - snapshot.ts > stale_after
```

Status-line behavior when stale:

- keep displaying the last known `@ai-*` options if the daemon writes tmux
  options directly
- optionally show a stale marker in a debug helper
- do not block tmux while trying to restart the daemon

The daemon can also set a global option:

```sh
tmux set-option -g @tmact-statusd-ts "$timestamp"
tmux set-option -g @tmact-statusd-stale ""
```

However, per-session `@ai-*` options are enough for the first implementation.

## Performance Expectations

Current status-line script:

- runs in tmux status refresh path
- repeats process inspection and pane capture on each refresh
- must avoid slow sampling

Daemon mode:

- runs work on its own interval
- keeps previous hashes in memory
- amortizes process inspection and capture cost
- lets tmux status rendering read precomputed options

Expected behavior on a machine with 10-30 panes:

- status rendering should remain near-instant
- daemon scan should usually complete well under the interval
- one slow pane capture or process lookup should not block tmux rendering

The first implementation can be single-threaded. If scan duration becomes a
problem, process/pane inspection can later use a bounded worker pool.

## Error Handling

The daemon should keep running through common failures:

- tmux server temporarily unavailable
- pane disappears between list and capture
- process inspection fails
- capture fails for one pane
- state file write fails
- tmux option update fails for one session

The snapshot should include pane-level errors where useful:

```json
{
  "target": "ops:0.0",
  "state": "blocked",
  "runtime": "unknown",
  "error": "capture pane: no such pane"
}
```

A daemon-level `errors` array should be bounded so the status file cannot grow
without limit.

## Safety

The daemon should be observe-only:

- no key presses
- no prompt approvals
- no arbitrary shell actions
- no remote HTTP server in the first version

It may call:

- `tmux list-panes`
- `tmux capture-pane`
- `tmux set-option`
- `ps`/`pgrep` or platform equivalents for process inspection

Any future integration that acts on prompts should stay in `watch`, `loop`, or
`workflow`, where allowlists and dry-run behavior already exist.

## Suggested Internal Packages

Possible package layout:

```text
internal/statusd/
  daemon.go        main loop and lifecycle
  snapshot.go      schema and aggregation
  publisher.go     atomic file writer and tmux option publisher
  state.go         in-memory previous hashes and timestamps
  config.go        options/defaults
  statusline.go    compact tag/glyph mapping
```

Reuse existing packages:

```text
internal/tmux       list/capture/set-option wrappers
internal/panestatus runtime/state detection
internal/prompt     permission prompt parsing
internal/agents     state constants and text classifiers
```

`internal/tmux` may need a helper for session options:

```go
func SetSessionOption(session string, key string, value string) error
func SetGlobalOption(key string, value string) error
```

## Implementation Plan

### Phase 1: Snapshot Once

Add:

```sh
tmact statusd once --json
```

Behavior:

- list all panes
- inspect each pane once
- build the proposed snapshot schema
- compute tags
- write atomic status file
- optionally print JSON

Validation:

```sh
go test ./...
go run ./cmd/tmact statusd once --json
```

### Phase 2: In-Memory Debounce

Add:

```sh
tmact statusd start --interval 1s
```

Behavior:

- keep previous normalized hashes by pane ID
- compute `last_changed_at`
- compute `running`
- refresh snapshot every interval
- handle pane creation/removal

Validation:

```sh
go run ./cmd/tmact statusd start --interval 1s --state-path /tmp/tmact-status.json
tmact statusd read
```

### Phase 3: Tmux Option Publisher

Add `--tmux-options` support.

Behavior:

- derive session-level status
- write `@ai-tag`
- write `@ai-running`
- write `@ai-asking`
- write `@row-bucket`

Validation:

```sh
tmux show-options -t ops -qv @ai-tag
tmux show-options -t ops -qv @ai-running
tmux show-options -t ops -qv @ai-asking
```

### Phase 4: Status-Line Integration

Change `.tmux.conf` from running a detector every refresh to either:

- no helper at all, relying on daemon-updated options
- or a tiny "ensure daemon" helper that returns immediately

The existing display format can stay mostly unchanged:

```tmux
#S·#{@ai-tag}#{@ai-running}#{@ai-asking}
```

### Phase 5: Launch/Supervision

Document one or more ways to run the daemon:

- manual tmux window
- launchd user service on macOS
- shell startup command
- tmux `run-shell -b` helper

The first reliable path can be manual:

```sh
tmux new-window -n tmact-statusd 'tmact statusd start --tmux-options'
```

## Testing Plan

Unit tests:

- tag mapping
- snapshot aggregation
- row bucket assignment
- atomic writer behavior using a temp directory
- stale detection
- debounce behavior with fake clock
- pane removal cleanup
- session-level aggregation with multiple panes
- prompt-to-asking mapping

Integration/smoke tests:

```sh
go test ./...
go run ./cmd/tmact statusd once --json
go run ./cmd/tmact statusd once --tmux-options
tmux show-options -t ops -qv @ai-tag
```

Manual scenarios:

- idle shell pane
- active Codex pane
- active Claude pane with version-like `pane_current_command`
- tmact workflow pane
- permission prompt pane
- tmux server restart while daemon is running
- session created after daemon start
- session killed after daemon start

## Migration Plan

1. Implement `statusd once` and compare its JSON output with
   `tmact inspect --all --sample 2 --interval 1s --json`.
2. Implement long-running mode and verify `running` debounce.
3. Enable tmux-option publishing manually.
4. Remove expensive detection from `tmux-ai-update.sh`, or replace it with a
   no-op/staleness helper.
5. Keep the old shell scripts as a fallback until the daemon has been used for
   several days.

## Open Questions

- Should `unknown` display as `?` or preserve the current `$` fallback?
- Should session-level `running` represent only the active pane or any pane in
  the session?
- Should `asking` use any pane in the session by default?
- Should statusd include non-agent shells in detail, or only publish compact
  status for sessions that look like agents?
- Should the daemon write tmux options globally, per session, or both?
- Should there be a config file, or are CLI flags enough for the first version?
- Should launchd service files live in this repo?

## Recommendation

Implement `tmact statusd` as the long-term replacement for the current shell
status detector. Keep the tmux status UI and option names stable, but move
runtime detection, prompt detection, output hashing, and debounce into the Go
daemon.

The best first milestone is:

```sh
tmact statusd start --interval 1s --state-path /tmp/tmact-status.json --tmux-options
```

At that point, tmux status rendering becomes a cheap read of existing `@ai-*`
options, while `tmact` becomes the single source of truth for pane runtime and
activity state.
