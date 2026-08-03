---
name: agent-loop
description: Run a durable unattended Coordinator→Implementer→Reviewer development loop with tmact workflow agent_dev, including phase planning, one-at-a-time committed work items, structured review findings, automatic fix planning, and re-review until approval. Use for "agent 自我運作", "大 loop", "loop 直到 approve", "review 完 dispatch 回去", "impl/review loop", "收斂", "派工迴圈", and requests for a coordinator to drive implementation phases without blocking the caller. Do not use for generic periodic single-pane prompts or one-shot delegation; use tmact-loop or tmact-dispatch respectively.
---

# Agent development loop

Use workflow v2's `agent_dev` stage as the durable scheduler. The Coordinator
plans; tmact dispatches. Do not let an agent bypass workflow state by calling
`dispatch-work` for another role.

## Pre-flight

Honor repository instructions, including command prefixes. Confirm:

```sh
tmact version
tmact help workflow --json
git status --porcelain
git branch --show-current
```

Require a clean attached branch. Do not stash, discard, switch branches, or
start while another process owns unfinished changes. Resolve exact runtime and
session choices for Coordinator, Implementer, and Reviewer.

## Create and validate

Generate the current profile:

```sh
tmact workflow example --profile agent-dev > agent-dev-workflow.yaml
```

Set `workspace.root`, unique session names, `queue_path`, timeouts, and bounded
phase/item/no-progress limits. Keep `workspace.git.lease: true` and
`defaults.max_parallel: 1` for a shared worktree.

Commit the workflow config before live start and confirm the repository is
clean. The Coordinator's first dispatch is rejected when generated config or
other setup files are still untracked.

Validate and inspect the complete side-effect-free plan:

```sh
tmact workflow validate --config agent-dev-workflow.yaml --var request="REQUEST"
tmact workflow plan --config agent-dev-workflow.yaml --var request="REQUEST"
```

Do not pre-create fake checked items. The Coordinator must add one unchecked
line per implementation item plus one review item, then commit only the queue
file. Stable item IDs must be unique across every phase and fix round.

## Start and observe

Starting live work is a separate side effect. Once authorized:

```sh
tmact workflow start --config agent-dev-workflow.yaml \
  --var request="REQUEST" --execute
tmact workflow status --config agent-dev-workflow.yaml
tmact workflow logs --config agent-dev-workflow.yaml
```

The caller does not wait for individual agents. The background runner keeps a
durable dispatch ID for every turn and resumes from reports after restarts.
If Claude reaches its session limit, the runner may enter durable
`waiting_quota`; this is normal unattended operation, not a failed work item.
It selects only the exact preselected `Stop and wait for limit to reset` menu,
never `Upgrade your plan`, sleeps until the parsed reset time, and resumes the
same dispatch/session/attempt. The wait and reset time survive runner restarts.

Completion contracts are enforced by tmact:

- Every dispatch starts clean on the same branch.
- An Implementer completes exactly one item, checks it, commits code/tests and
  checkbox together, and leaves a clean tree.
- A Reviewer request for changes leaves Git unchanged and submits structured
  findings with stable fingerprints.
- A Reviewer approves only by committing evidence and the phase review
  checkbox.
- Coordinator fix plans append committed fix items; tmact dispatches them and
  returns to the same review item.
- Only Reviewer approval closes a phase. The Coordinator then plans the next
  phase or reports the overall request done.

## Blockers and control

Permission or approval prompts, dirty Git state, branch drift, missing commits,
wrong checkboxes, timeout, and repeated identical findings stop as
`needs_user`. Never auto-answer or route around these states. The only prompt
exception is tmact's built-in exact Claude session-limit wait menu; any menu
shape or wording drift remains `needs_user`.

```sh
tmact workflow pause --config agent-dev-workflow.yaml
tmact workflow retry --id RUN_ID --stage delivery
tmact workflow resume --config agent-dev-workflow.yaml
tmact workflow stop --config agent-dev-workflow.yaml --wait
```

Inspect status and the exact worktree before retrying. A retry preserves the
durable phase queue and reschedules the interrupted item; it does not authorize
cleaning or discarding changes.

## Finish

Report phase count, completed work-item and review commits, review rounds,
remaining findings, tests, final workflow status, branch, and clean worktree.
Do not push unless separately authorized.
