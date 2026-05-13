# Tasks: OpenSpec Artifact Review Workflow v1

## 1. OpenSpec Alignment

- [x] Update proposal from two-role proposal review to multi-role artifact
  review.
- [x] Replace `proposal_hash` gate with `change_hash` gate.
- [x] Define serialized `pm -> swe -> qa -> reviewer -> stale refresh -> gate`
  flow.

## 2. Config And State

- [x] Add `internal/workflow` config structs and loader.
- [x] Reject change names that resolve outside `openspec/changes/`.
- [x] Validate required role mappings: `pm`, `swe`, `qa`, `reviewer`.
- [x] Resolve role mappings through `internal/agents`.
- [x] Add canonical `sha256:<hex>` change hash over proposal, design, tasks,
  and spec delta files.
- [x] Add append-only comment stream helpers.
- [x] Add atomic state read/write helpers.

## 3. Validation And Gate

- [x] Run `openspec validate <change> --strict` without shell invocation.
- [x] Capture validation exit status, stdout, stderr, timestamps, and
  `change_hash`.
- [x] Treat validation as stale if artifacts change during validation.
- [x] Parse strict `TMAct-OpenSpec-Comment:` markers.
- [x] Require all roles to accept the same current `change_hash`.
- [x] Block advancement when OpenSpec validation fails or is stale.
- [x] Block advancement when unresolved blocking comments remain.
- [x] Return explicit gate reasons for missing, stale, withdrawn, mismatched, or
  blocked agreement.

## 4. Runner

- [x] Implement dry-run first workflow runner.
- [x] Implement serialized pending-role selection.
- [x] Generate role-specific prompts for PM, SWE, QA, and reviewer.
- [x] Capture pane output and record new comments.
- [x] Avoid repeated prompts for the same role/hash unless state changes.
- [x] Add max turn, max runtime, and polling bounds.
- [x] Stop on permission prompts.
- [x] Persist JSONL trace events.
- [x] Register long-running runs through `internal/runmeta`.
- [x] Observe stop state before sending tmux input.

## 5. CLI And Examples

- [x] Add `tmact workflow discuss`.
- [x] Add `tmact workflow status`.
- [x] Add `tmact workflow stop`.
- [x] Add `examples/openspec-workflow.yaml`.
- [x] Add command help and command manifest entries.

## 6. Verification

- [x] Add unit tests for config defaults and validation.
- [x] Add unit tests for change-name path traversal rejection.
- [x] Add unit tests for canonical artifact hashing.
- [x] Add unit tests for comment marker parsing.
- [x] Add unit tests for agreement gate pass/fail cases.
- [ ] Add unit tests for stale validation results.
- [x] Add CLI tests for workflow help and status output.
- [x] Run `openspec validate agent-dev-phase1-discussion --strict`.
- [x] Run `go test ./...`.
- [x] Run `go run ./cmd/tmact workflow discuss --config examples/openspec-workflow.yaml --dry-run --once`.
