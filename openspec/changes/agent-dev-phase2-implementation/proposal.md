# Proposal: OpenSpec Implementation Workflow v1

## Intent

Add a phase 2 workflow that starts after the OpenSpec artifact review has
agreed and coordinates implementation, verification, and archive handoff through
the same tmux agent model.

The v1 sequence is:

```text
phase1 agreed -> SWE apply -> QA verify -> PM archive -> done
```

The workflow should make the implementation phase inspectable and resumable
without weakening the safety boundaries from phase 1.

## User

The user is a tmact operator who has an accepted OpenSpec change and wants the
existing SWE, QA, and PM agent panes to carry it through implementation. The
operator needs tmact to preserve local state, show progress, stop safely, and
avoid archiving a change before implementation and verification have completed.

## Problem

The phase 1 workflow produces agreement over the OpenSpec artifact set, but it
stops before any work is applied. That leaves the operator to manually prompt
SWE, QA, and PM panes, track whether they all acted on the same accepted change,
and ensure the PM does not archive before QA has passed.

There is also a command-shape mismatch to handle carefully: the local OpenSpec
CLI currently exposes `openspec instructions apply --change <change>`,
`openspec validate <change> --strict`, `openspec status --change <change>`, and
`openspec archive <change> --yes`, but it does not expose top-level
`openspec apply` or `openspec verify` commands. Phase 2 should model the user
intent as apply and verify stages while keeping the actual commands configurable
and detectable.

## Scope

In scope:

- Add `tmact workflow implement` for a serialized post-agreement execution
  phase.
- Reuse the existing workflow config shape where practical and add an
  `implementation` section.
- Require role keys `swe`, `qa`, and `pm` for phase 2.
- Start only when the targeted phase 1 state has outcome `agreed`, unless the
  operator explicitly bypasses the precondition in dry-run mode.
- Bind phase 2 to the accepted phase 1 `change_hash`.
- Prompt SWE to apply the accepted OpenSpec change and leave a strict completion
  marker.
- Prompt QA to verify the implemented change and leave a strict pass/fail
  marker.
- Prompt PM to archive the change only after QA passes.
- Run OpenSpec validation before PM archive can satisfy the final gate.
- Persist phase 2 state, comments, command observations, and trace events in
  the change folder and `.tmact`.
- Keep tmux side effects dry-run by default and require `--execute` to send
  prompts.
- Stop on permission prompts, operator stop requests, failed verification, or
  archive conflicts.

Out of scope for v1:

- Automatically editing source code without routing through the SWE pane.
- Automatically approving tool, filesystem, shell, or archive prompts inside an
  agent pane.
- Running arbitrary shell command strings from captured pane output.
- Parallel SWE/QA/PM execution.
- Automatic rollback of implementation changes.

## Proposed User Flow

1. The operator completes phase 1:
   `tmact workflow discuss --config examples/openspec-workflow.yaml --execute`.
2. The operator confirms agreement:
   `tmact workflow status --config examples/openspec-workflow.yaml`.
3. The operator starts:
   `tmact workflow implement --config examples/openspec-implementation.yaml`.
4. tmact loads `phase1-state.json`, confirms outcome `agreed`, records the
   accepted `change_hash`, and runs OpenSpec validation for the same change.
5. tmact prompts SWE with apply-stage instructions. The prompt includes the
   accepted `change_hash`, the OpenSpec apply instruction command, and the
   required marker format.
6. SWE implements the change, updates `tasks.md`, and leaves a marker such as:
   `TMAct-OpenSpec-Phase2: role=swe stage=apply kind=complete change_hash=sha256:... blocking=false`.
7. tmact prompts QA after SWE completion. QA runs the configured verification
   checks and leaves a marker such as:
   `TMAct-OpenSpec-Phase2: role=qa stage=verify kind=pass change_hash=sha256:... blocking=false`.
8. tmact prompts PM after QA pass. PM archives the change using the configured
   archive command and leaves a marker such as:
   `TMAct-OpenSpec-Phase2: role=pm stage=archive kind=complete change_hash=sha256:... blocking=false`.
9. The run stops with one of:
   - `implemented`
   - `needs_user`
   - `blocked`

## Decisions

- Phase 2 is a separate subcommand: `tmact workflow implement`.
- The default phase 2 role order is `swe`, `qa`, `pm`.
- The durable marker prefix is `TMAct-OpenSpec-Phase2:`.
- The phase 2 gate is stage-based, not all-role agreement-based.
- The accepted phase 1 `change_hash` is the revision token for phase 2. Source
  code edits are not included in that hash.
- OpenSpec commands are represented as stage labels plus explicit command args.
  v1 must not assume top-level `openspec apply` or `openspec verify` exists.
- PM archive is never prompted until SWE apply completed, QA verification
  passed, and OpenSpec validation is passing.

## Success Criteria

- `tmact workflow implement --dry-run --once` prints the next phase 2 prompt
  without touching tmux.
- A phase 2 run refuses to start when phase 1 has not agreed for the same
  change, except in dry-run bypass mode.
- SWE cannot be skipped before QA.
- QA failure stops the run with `blocked` or `needs_user` and does not prompt
  PM archive.
- PM archive is prompted only after QA pass and strict OpenSpec validation pass.
- `tmact workflow status` reports both phase 1 and phase 2 state when a config
  targets a change with phase 2 files.
- Tests cover phase 2 config validation, precondition checks, marker parsing,
  stage gate ordering, dry-run behavior, failed QA gating, and archive gating.
