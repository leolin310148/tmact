# Proposal: OpenSpec Artifact Review Workflow v1

## Intent

Introduce a phase 1 agent development workflow that coordinates multiple tmux
agent panes through a serialized OpenSpec artifact review. The v1 sequence is:

```text
PM -> SWE -> QA -> reviewer -> stale accept refresh -> gate
```

The workflow is meant for unattended, long-running feature definition. It stops
only when the OpenSpec change is valid and every required role has accepted the
same current artifact revision, or when the run reaches a bounded terminal
outcome.

## User

The user is a tmact operator running an OpenSpec change through overnight agent
review. The operator needs a local, inspectable workflow that can send prompts
to existing agent panes, persist state, survive long waits, expose status, and
stop without auto-approving permission prompts.

## Problem

The earlier agent loop could keep agents active, but it did not create a durable
agreement point over the complete OpenSpec artifact set. A proposal-only gate is
too narrow once PM, SWE, QA, and reviewer roles are involved, because design,
tasks, and spec deltas can change the meaning of the change after a proposal has
already been accepted.

## Scope

In scope:

- Add `tmact workflow discuss` for serialized OpenSpec artifact review.
- Require these role keys by default: `pm`, `swe`, `qa`, and `reviewer`.
- Review the full OpenSpec change artifact set:
  - `proposal.md`
  - `design.md`
  - `tasks.md`
  - `specs/*/spec.md`
- Compute a canonical `change_hash` over the artifact set.
- Run `openspec validate <change> --strict` before acceptance can satisfy the
  gate.
- Parse strict role comments from pane output and persist them locally.
- Pass the gate only when every required role accepts the current
  `change_hash`, OpenSpec validation passed for that hash, and no unresolved
  blocking comments remain for that hash.
- Revisit stale acceptances after any role modifies artifacts.
- Persist state, comments, validation results, and trace events in the change
  folder and `.tmact`.
- Keep tmux side effects dry-run by default and require `--execute` to send
  prompts.
- Stop on permission prompts and operator stop requests.

Out of scope for v1:

- Automatic implementation of accepted specs.
- Parallel artifact editing.
- LLM API orchestration outside existing tmux panes.
- Auto-approving tool, filesystem, or shell permission prompts.
- Automatically starting the next phase after agreement.

## Proposed User Flow

1. The operator creates or selects `openspec/changes/<change>/`.
2. The operator starts:
   `tmact workflow discuss --config examples/openspec-workflow.yaml`.
3. tmact resolves configured role keys to exact agent names from
   `agents_config`.
4. tmact computes the current `change_hash`, runs OpenSpec validation, and
   prompts the first stale or missing role in the order `pm`, `swe`, `qa`,
   `reviewer`.
5. The prompted role may edit any in-scope OpenSpec artifact and must leave a
   strict comment marker such as:
   `TMAct-OpenSpec-Comment: role=pm kind=accept change_hash=sha256:... openspec_valid=true blocking=false`.
6. tmact captures pane output, records new comments, recomputes the
   `change_hash`, reruns validation, and advances to the next role that has not
   accepted the current hash.
7. If a later role changes any artifact, earlier acceptances become stale and
   the workflow refreshes those roles in serialized order.
8. The gate passes only when all required roles accept the same current
   `change_hash`, OpenSpec validation passed for that hash, and no unresolved
   blocking comments remain for that hash.
9. The run stops with one of:
   - `agreed`
   - `needs_user`
   - `blocked`

## Decisions

- The command group is `tmact workflow`.
- The initial runnable subcommand is `tmact workflow discuss`.
- Status and stop are `tmact workflow status` and `tmact workflow stop`.
- The durable comment marker prefix is `TMAct-OpenSpec-Comment:`.
- v1 uses serialized turns. It does not send prompts to multiple roles in
  parallel.
- The artifact revision key is `change_hash`, not `proposal_hash`.
- The workflow may create a deterministic missing `proposal.md` only when
  `create_missing_proposal: true`; the change folder itself must already exist.

## Success Criteria

- `tmact workflow discuss --dry-run --once` prints the next role prompt without
  touching tmux.
- `tmact workflow discuss --execute` can run long enough for overnight review
  while preserving local state and logs.
- `tmact workflow status` reports phase, turn count, pending role, latest
  validation, current `change_hash`, and outcome without attaching to panes.
- A role acceptance is valid only for the exact `change_hash` it references.
- Any artifact edit invalidates older acceptances.
- Blocking comments prevent the gate until resolved by decision or by a new
  accepted artifact revision.
- Tests cover config validation, path safety, canonical hashing, comment
  parsing, agreement gating, stale validation, dry-run behavior, and status
  rendering.
