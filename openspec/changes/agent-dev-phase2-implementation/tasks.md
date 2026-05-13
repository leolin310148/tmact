# Tasks: OpenSpec Implementation Workflow v1

## 1. OpenSpec Alignment

- [x] Validate this phase 2 proposal with the phase 1 discussion workflow.
- [x] Confirm the final command vocabulary for apply, verify, and archive in
  the local OpenSpec CLI.
- [x] Decide whether QA verification commands are prompt-only in v1 or also
  locally executed by tmact as preflight checks.

## 2. Config And State

- [x] Add phase 2 implementation config structs and defaults.
- [x] Validate required role mappings: `swe`, `qa`, and `pm`.
- [x] Add structured command config for apply instructions, verify commands,
  and archive command.
- [x] Add `phase2-state.json` read/write helpers.
- [x] Add `phase2-comments.jsonl` append/read helpers.
- [x] Bind phase 2 state to the accepted phase 1 `change_hash`.
- [x] Reject live phase 2 execution when phase 1 did not finish as `agreed`.

## 3. Markers And Gate

- [x] Parse strict `TMAct-OpenSpec-Phase2:` markers.
- [x] Require SWE `stage=apply kind=complete` before QA can run.
- [x] Require QA `stage=verify kind=pass` before PM archive can run.
- [x] Treat QA `fail`, `request_changes`, or `blocked` as a terminal
  non-archive outcome until the operator restarts or artifacts change.
- [x] Require strict OpenSpec validation pass before PM archive prompt.
- [x] Require PM `stage=archive kind=complete` before final `implemented`
  outcome.
- [x] Return explicit gate reasons for missing stage, failed QA, stale hash,
  failed validation, missing command, permission prompt, and stop request.

## 4. Runner

- [x] Implement dry-run first phase 2 runner.
- [x] Implement serialized stage selection.
- [x] Generate stage-specific prompts for SWE apply, QA verify, and PM archive.
- [x] Capture role pane output and record new phase 2 comments.
- [x] Avoid repeated prompts for the same stage/hash unless state changes.
- [x] Add max turn, max runtime, and polling bounds.
- [x] Stop on permission prompts.
- [x] Persist JSONL trace events.
- [x] Register long-running phase 2 runs through `internal/runmeta`.
- [x] Observe stop state before sending tmux input.

## 5. CLI And Examples

- [x] Add `tmact workflow implement`.
- [x] Update `tmact workflow status` to include phase 2 state when present.
- [x] Update `tmact workflow stop` help if needed.
- [x] Add `examples/openspec-implementation.yaml`.
- [x] Add command help and command manifest entries.
- [x] Document the phase 1 to phase 2 operator flow.

## 6. Verification

- [x] Add unit tests for phase 2 config defaults and validation.
- [x] Add unit tests for phase 1 precondition enforcement.
- [x] Add unit tests for accepted hash mismatch rejection.
- [x] Add unit tests for phase 2 marker parsing.
- [x] Add unit tests for stage gate pass/fail cases.
- [x] Add unit tests for PM archive gating.
- [x] Add CLI tests for `workflow implement` help.
- [x] Run `openspec validate agent-dev-phase2-implementation --strict`.
- [x] Run `go test ./...`.
- [x] Run `go run ./cmd/tmact workflow implement --config examples/openspec-implementation.yaml --dry-run --once`.
