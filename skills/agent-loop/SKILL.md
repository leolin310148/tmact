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
limit, and cadence. Name sessions `<project>-impl` and `<project>-review`.
Dry-run each new dispatch before adding `--execute`.

## Run a round

### 1. Implement or fix

Dispatch the task specification on the first round. On later rounds, pass the
reviewer's confirmed findings verbatim. Require the implementer to run the
project checks and end with a commit hash or a clear blocker.

### 2. Review with fresh context

Wait until the implementer becomes idle, then dispatch to the reviewer. Session
reuse sends `/clear`, giving each round a fresh-context review. Require the
reviewer to verify every finding, retain only true blocking issues, and end with
exactly one verdict:

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

## Cadence

- For interactive work, poll on a 30-60 second cadence and advance when the
  current agent is idle.
- For timed unattended work, use `tmact loop` or the user's scheduler instead
  of busy-waiting in the coordinator session.

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
