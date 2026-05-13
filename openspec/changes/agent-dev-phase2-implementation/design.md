# Design: OpenSpec Implementation Workflow v1

## Overview

Extend `internal/workflow` and CLI wiring under `tmact workflow` with a phase 2
runner:

```text
swe apply -> qa verify -> pm archive
```

The implementation workflow is dry-run by default. Live tmux sends require
`--execute`.

## Change Folder

Each phase 2 run targets the same existing change folder as phase 1:

```text
openspec/changes/<change>/
  proposal.md
  design.md
  tasks.md
  specs/*/spec.md
  phase1-state.json
  phase2-comments.jsonl
  phase2-state.json
```

`phase1-state.json` is the precondition source. `phase2-state.json` records the
execution phase, current stage, accepted `change_hash`, stage outcomes, latest
validation result, and terminal outcome. Comment and trace streams are
append-only JSONL. State writes stay atomic.

## Config Shape

```yaml
change: agent-dev-phase2-implementation
agents_config: examples/openspec-workflow-agents.yaml
roles:
  pm: pm-agent
  swe: swe-agent
  qa: qa-agent
implementation:
  stage_order: [swe_apply, qa_verify, pm_archive]
  max_turns: 12
  max_runtime: 8h
  poll_interval: 15s
  idle_after: 30s
  capture_lines: 180
  require_phase1_agreed: true
  allow_dry_run_without_phase1: false
  apply_instructions:
    command: openspec
    args: ["instructions", "apply", "--change", "{{change}}"]
  verify_commands:
    - command: openspec
      args: ["validate", "{{change}}", "--strict"]
    - command: go
      args: ["test", "./..."]
  archive_command:
    command: openspec
    args: ["archive", "{{change}}", "--yes"]
log_path: .tmact/openspec-implementation.jsonl
```

Command entries are structured command plus args, not shell strings. The runner
may render commands into prompts, and it may execute local preflight checks such
as OpenSpec validation directly with `os/exec`. It must not execute arbitrary
commands observed from panes.

## Stage Markers

Pane output is untrusted. Only strict markers affect phase 2 gates:

```text
TMAct-OpenSpec-Phase2: role=swe stage=apply kind=complete change_hash=sha256:... blocking=false body="implemented tasks"
TMAct-OpenSpec-Phase2: role=qa stage=verify kind=pass change_hash=sha256:... blocking=false body="openspec validate and go test passed"
TMAct-OpenSpec-Phase2: role=pm stage=archive kind=complete change_hash=sha256:... blocking=false body="archived change"
```

Supported kinds:

- `complete`
- `pass`
- `fail`
- `request_changes`
- `blocked`
- `decision`
- `withdraw`

`fail`, `request_changes`, and `blocked` prevent later stages for the accepted
hash until artifacts or source code are corrected and the stage is retried.

## Preconditions

Before prompting a phase 2 role, the runner:

1. Resolves the change under `openspec/changes/`.
2. Loads `phase1-state.json`.
3. Requires phase 1 outcome `agreed` when `require_phase1_agreed` is true.
4. Records the phase 1 `change_hash` as `accepted_change_hash`.
5. Recomputes the current OpenSpec artifact hash.
6. Refuses to proceed if the current artifact hash differs from the accepted
   hash.
7. Runs `openspec validate <change> --strict`.

The dry-run bypass is only for planning prompts while a proposal is still being
designed. Live `--execute` must require an agreed phase 1 state.

## Runner

The runner loop:

1. Initializes phase 2 state and registers run metadata.
2. Captures configured role panes and records new phase 2 markers.
3. Stops if a permission prompt is visible.
4. Evaluates the next stage:
   - SWE apply is pending until SWE leaves `stage=apply kind=complete`.
   - QA verify is pending until QA leaves `stage=verify kind=pass`.
   - PM archive is pending until PM leaves `stage=archive kind=complete`.
5. Runs strict OpenSpec validation before planning PM archive.
6. Sends one prompt for the pending stage and accepted hash unless that prompt
   was already sent.
7. Repeats until `implemented`, `needs_user`, `blocked`, context cancellation,
   stop request, max turns, or max runtime.

`--once` performs one observe/precondition/gate/prompt pass and exits.

## Stage Prompts

SWE prompt:

- Shows accepted `change_hash`.
- Includes OpenSpec apply instruction command:
  `openspec instructions apply --change <change>`.
- Directs SWE to implement only the accepted change, update `tasks.md`, run
  focused tests as appropriate, and leave a strict marker.

QA prompt:

- Shows accepted `change_hash`.
- Lists configured verification commands.
- Directs QA to verify the implementation, update any test evidence, and leave
  `kind=pass` or `kind=fail`.

PM prompt:

- Shows accepted `change_hash`.
- Lists the archive command.
- States that PM should archive only after QA pass and current OpenSpec
  validation pass.
- Requires PM to leave `stage=archive kind=complete` after archive succeeds.

## Status And Stop

`workflow status` reads both phase state files when available:

- phase 1 review state from `phase1-state.json`
- phase 2 implementation state from `phase2-state.json`
- run metadata from `.tmact/runs`

`workflow stop` continues to use run metadata. The phase 2 runner checks stop
state before every tmux send and exits without sending more input when the run
is stopping.

## Risks

- OpenSpec command mismatch. v1 stores apply/verify/archive commands in config
  and treats missing commands as a blocked or needs-user outcome.
- Premature archive. PM archive is stage-gated behind SWE complete, QA pass, and
  strict OpenSpec validation pass.
- Prompt injection from captured panes. Captured output remains untrusted and
  only strict phase 2 markers affect gates.
- Wrong-pane sends. Role mappings continue to resolve through exact agent names
  and dry-run remains the default.
