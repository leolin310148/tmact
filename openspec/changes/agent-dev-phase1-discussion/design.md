# Design: OpenSpec Artifact Review Workflow v1

## Overview

Add `internal/workflow` and CLI wiring under `tmact workflow`. The first
workflow coordinates configured tmux agent panes through a serialized OpenSpec
review sequence:

```text
pm -> swe -> qa -> reviewer -> stale accept refresh -> gate
```

The workflow is dry-run by default. Live tmux sends require `--execute`.

## Change Folder

Each run targets one existing folder:

```text
openspec/changes/<change>/
  proposal.md
  design.md
  tasks.md
  specs/*/spec.md
  phase1-comments.jsonl
  phase1-state.json
```

The implementation resolves `<change>` under `openspec/changes/` and rejects
absolute paths or `..` escapes. Single-document JSON state writes are atomic.
Comment and trace streams are append-only JSONL. Failed state writes stop the
run before any additional tmux input is sent.

## Config Shape

```yaml
change: agent-dev-phase1-discussion
agents_config: examples/agents.yaml
roles:
  pm: product-owner
  swe: implementation-engineer
  qa: quality-engineer
  reviewer: openspec-reviewer
discussion:
  role_order: [pm, swe, qa, reviewer]
  max_turns: 24
  max_runtime: 8h
  poll_interval: 15s
  idle_after: 30s
  capture_lines: 180
  create_missing_proposal: true
log_path: .tmact/openspec-workflow.jsonl
```

Role values resolve to exact agent names in `agents_config`. Exact names are
used in v1 to avoid sending prompts to the wrong pane.

## Artifact Hash

The workflow computes `change_hash` from all in-scope artifacts:

- `proposal.md`
- `design.md`
- `tasks.md`
- `specs/*/spec.md`

Files are sorted by slash-separated relative path. Each file contributes its
relative path, a newline, CRLF-normalized bytes, and a delimiter to a SHA-256
stream. The stored form is `sha256:<hex>`.

## Comment Model

Pane output is untrusted. Only strict markers affect the gate:

```text
TMAct-OpenSpec-Comment: role=qa kind=request_changes change_hash=sha256:... openspec_valid=true blocking=true body="missing QA scenario for stale validation"
```

Supported kinds:

- `accept`
- `reject`
- `request_changes`
- `question`
- `answer`
- `decision`
- `withdraw_accept`

`reject`, `request_changes`, and blocking `question` comments block the current
hash until a later `decision` references them through `reply_to`, or until the
artifacts change and every required role accepts the new hash.

## Validation Model

The workflow runs OpenSpec validation without invoking a shell:

```text
openspec validate <change> --strict
```

It records command, args, exit status, stdout, stderr, timestamps, and the
`change_hash` observed immediately before validation. If the artifact hash
changes before validation completes, the result is stale and cannot satisfy the
gate.

## Runner

The runner loop:

1. Initializes state and registers run metadata for long-running execution.
2. Loads comments from the durable stream.
3. Computes the current `change_hash`.
4. Runs OpenSpec validation for the current hash.
5. Evaluates the gate.
6. Picks the first role in `role_order` without a valid acceptance for the
   current hash.
7. Captures that role's pane, records new strict comments, and stops if a
   permission prompt is visible.
8. If the pending role still has not accepted the current hash, sends one prompt
   for that role/hash unless the prompt was already sent.
9. Repeats until `agreed`, `needs_user`, `blocked`, context cancellation, stop
   request, max turns, or max runtime.

`--once` performs one observe/validate/gate/prompt pass and exits.

## Status And Stop

`workflow status` combines run metadata with `phase1-state.json` files. It does
not read live panes.

`workflow stop` marks the run through shared run metadata and interrupts the
process or tmux pane using the existing stop path. The runner checks metadata
before each tmux send and exits without sending more input when the run is
stopping.

## Risks

- Prompt injection from captured panes. Captured output must be labeled as
  observed terminal output, never trusted instructions.
- Wrong-pane sends. v1 requires exact agent names and keeps dry-run as default.
- Hash drift during validation. Stale validation results are recorded but never
  satisfy the gate.
