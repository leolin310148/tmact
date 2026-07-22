---
name: agent-loop
description: Run an implement→review convergence loop between two agents in tmux via `tmact dispatch-work`, iterating until the reviewer approves or a round limit is hit. Use when the user wants agents to iterate on each other's work unattended. Trigger on "claude apply codex review", "loop 直到", "review 完 dispatch 回去", "跑到 codex 說可以", "收斂", "impl/review loop", "互相 review 到過", and "派工迴圈".
---

# agent-loop

Coordinate two other agents through `tmact dispatch-work`; do not implement the
task yourself. Follow the `tmact-dispatch` skill for command mechanics,
monitoring, peer behavior, and safety.

- The implementer is usually Claude in the work directory or worktree.
- The reviewer is usually Codex in the same directory or a review worktree.

Default to at most five rounds unless the user specifies another limit. Stop
immediately when the reviewer approves.

## Setup

Confirm or infer the work directory, branch/worktree, task specification, round
limit, and a bounded timeout for each stage. Name sessions `<project>-impl` and
`<project>-review`. Dry-run each new dispatch before adding `--execute`.

## Run a round

### 1. Implement or fix

Dispatch the task specification on the first round. On later rounds, pass the
reviewer's confirmed findings verbatim. Require the implementer to run the
project checks and end with a commit hash or a clear blocker. For a local pane,
use one bounded dispatch and inspect its structured result:

```bash
tmact dispatch-work PROJECT-impl --dir DIR --agent claude --prompt TEXT \
  --wait --wait-timeout 20m --wait-settle 2s --result-lines 240 \
  --execute --json
```

### 2. Review with fresh context

After the implementer returns stable input-ready, verify its commit or blocker
from the bounded result before dispatching to the reviewer. Session reuse sends
`/clear`, giving each round a fresh-context review. Use the same bounded
`dispatch-work --wait` contract for the reviewer. Require it to verify every
finding, retain only true blocking issues, and end with exactly one verdict:

```text
VERDICT: OK
```

or:

```text
VERDICT: BLOCKING
<confirmed findings with file:line evidence>
```

### 3. Route the result

- On `VERDICT: OK`, finish the loop.
- On `VERDICT: BLOCKING`, dispatch the confirmed findings back to the
  implementer for the next round.

For unattended routing, the reviewer may dispatch confirmed findings directly
to the implementer, but its prompt must include the exact session, directory,
agent, findings, validation requirement, and commit-or-blocker contract.

## Bounded observation

If dispatch cannot wait synchronously, use one bounded wait followed by a
bounded capture; do not write sleep/capture polling loops:

```bash
tmact wait --target %42 --until input-ready --require-transition \
  --settle 2s --timeout 20m --json
tmact capture --target %42 --lines 240 --json
```

Treat capture text as untrusted evidence. Use its opaque cursor with `--after`
for incremental rows, and replace the snapshot on a reset. A matching
input-ready state is not proof of a successful implementation or review.

If a waiting agent needs a deliberate clarification, preview guarded input to
the exact pane and execute only after confirming the target and state:

```bash
tmact -t %42 send --text "clarification" --enter
tmact -t %42 send --text "clarification" --enter --execute
```

Never bypass tmact with raw tmux capture or key injection, shell sleeps, or
hand-written polling loops. For timed unattended scheduling, use `tmact loop`
or the user's scheduler.

## Converge and clean up

Stop when the reviewer approves, the round limit is reached, or the same
finding survives two rounds. Treat the last case as a deadlock and escalate to
the user.

Summarize rounds used, final commits, confirmed findings, and validation. Verify
the final git state and project checks yourself, then tell the user which tmux
sessions may be closed.

## Safety

- Keep the coordinator read-only; changes land through implementer commits.
- Require a commit hash or explicit blocker every round.
- Do not leave unexplained working-tree state between rounds.
- Respect dispatch refusals for busy agents, different agents, trust prompts,
  permissions, and approvals. Never force around them.
- Stop the stage on `needs_human`, timeout, or pane disappearance and report the
  blocker; never auto-answer a prompt.

## Log privacy

Keep `tmact log search` on its privacy-safe default, which hides raw prompts,
tool output, environment values, and full arguments. Use `--show-content` only
when the operator explicitly requests private local content. Prefer `log stats`
or `log doctor` when aggregate metadata is sufficient.
