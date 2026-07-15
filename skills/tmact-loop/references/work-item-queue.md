# Work-item queue loops

Use this pattern when one unattended agent should repeatedly take the first
unchecked item, finish it, and commit before the next cycle.

## Prepare the queue and worker

Keep durable work in a repository queue such as `WORK_ITEMS.md` or
`WORKITEM.md`. Use `/tmp` only for intentionally disposable queues. Make every
item independently completable and testable; order dependencies explicitly.

Create a dedicated agent pane for the repository. Initialize it once with a
short prompt that reads repository instructions, confirms the exact branch and
clean state, makes no edits, and waits for loop input. Do not reuse a pane that
also receives interactive user work.

## Build each cycle

Use a two-step flow: clear context, then send one complete worker prompt. Set
`only_when_idle: true`. Choose `max_runs` from the number of intended items and
remember that `max_actions` counts steps, so a two-step flow needs at least
twice as many actions as runs.

The worker prompt must define:

1. Exact repository, branch, queue file, and instruction files to read.
2. Selection of only the first unchecked item; never a second item that cycle.
3. Dirty-tree policy. Default to pausing and reporting. Clean or discard work
   only when the user explicitly authorized it, check status first, and never
   run an unconditional reset on an already clean tree.
4. The item's complete acceptance criteria and required validation, including
   real UI checks when applicable.
5. Atomic completion: update only that checkbox and commit implementation,
   tests, reports, and checkbox together. Confirm a clean tree and report the
   commit hash. Do not push unless separately authorized.
6. Blocker behavior: do not check off or commit partial work; leave or restore
   the tree only according to the authorized dirty-tree policy, then report the
   blocker. Never auto-confirm permission or approval prompts.
7. Exhaustion behavior: when all items are checked, make no changes and report
   completion.

Put stable rules in repository instructions or the queue file when possible;
keep the loop prompt focused on the execution contract. Long duplicated prompts
consume context on every cycle and are harder to revise safely.

## Operate and revise

Validate, dry-run once, then start through the managed lifecycle. Keep the
config and log under `.tmact/` and use the default run directory when possible.
Managed runs with a custom `--run-dir` are still discoverable machine-wide, so
omit that flag for normal list, status, logs, pause, resume, restart, and stop
commands. Pass it only when intentionally restricting a command to one runtime
directory.

When requirements change mid-run:

1. Pause the active loop.
2. Update the queue and prompt without racing the worker.
3. Validate and dry-run the changed config.
4. Restart it to load the new YAML. Do not merely resume an old process.

Treat loop logs as scheduler evidence. `flow status: ok` means the clear and
prompt were delivered, not that the item succeeded. Confirm progress from the
checkbox count, new commits, clean working tree, and final pane response. Stop
the loop once the queue is complete instead of spending later cycles repeatedly
asking an exhausted queue.
