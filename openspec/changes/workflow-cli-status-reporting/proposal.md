# Proposal: Workflow CLI Status Reporting

## Intent

Replace pane-output markers as the primary workflow status channel with an
explicit `tmact workflow report` CLI, and make workflow prompts start from a
clean agent context by sending `/clear` before work is assigned.

## User

The user is a tmact operator coordinating Codex and Claude panes through an
OpenSpec review and implementation workflow. They need durable workflow state
to come from local tmact state files, not from fragile terminal rendering, and
they need each agent turn to start without stale pane context from previous
workflow runs.

## Problem

The current workflow asks agents to output strict marker lines such as
`TMAct-OpenSpec-Comment:` and `TMAct-OpenSpec-Phase2:`. In live tmux panes this
proved fragile:

- Long `change_hash` values visually wrap and may be captured as multiple
  lines.
- Quoted marker bodies can wrap and break marker parsing.
- Old pane history can be recaptured and mixed into prompt summaries.
- Agents may include prose around markers or reason from stale pane context.
- Archive can move the change folder after PM acts, leaving the runner to infer
  completion from pane output at exactly the wrong time.

The status channel should be a command the agent executes deliberately:

```sh
tmact workflow report review ...
tmact workflow report implementation ...
```

The runner should then gate on JSONL reports written by tmact itself.

## Scope

In scope:

- Add `tmact workflow report review` for phase 1 role review reports.
- Add `tmact workflow report implementation` for phase 2 stage reports.
- Validate report inputs before appending to workflow comment JSONL files.
- Update workflow prompts to instruct agents to run the appropriate report
  command instead of emitting marker text.
- Make workflow prompt dispatch optionally send `/clear` before the assignment
  prompt.
- Default the new workflow behavior to clear-before-prompt for agent panes.
- Preserve dry-run behavior by showing planned clear and prompt operations
  without sending tmux input.
- Keep legacy marker parsing as an explicit opt-in fallback for existing
  workflows.
- Treat PM archive moving the change folder as a normal implemented terminal
  condition when the corresponding report has been recorded.

Out of scope:

- Allowing agents to report arbitrary shell command strings into workflow
  state.
- Automatically approving permission, command, patch, or trust prompts.
- Replacing `tmact watch` allowlist decisions.
- Parallelizing workflow roles or stages.
- Clearing shell panes that are not configured workflow agent panes.

## Proposed User Flow

1. The operator starts a discussion workflow:
   `tmact workflow discuss --config examples/openspec-full-workflow.yaml --execute`.
2. Before each role receives work, tmact sends `/clear`, waits for a configured
   delay, and then sends the role prompt.
3. The role reviews artifacts and reports status by running a command such as:

   ```sh
   tmact workflow report review \
     --config examples/openspec-full-workflow.yaml \
     --role pm \
     --kind accept \
     --change-hash sha256:... \
     --openspec-valid \
     --blocking=false \
     --body "accepted current artifacts"
   ```

4. The runner observes the JSONL report file and advances the gate without
   parsing pane prose.
5. During implementation, SWE/QA/PM use:

   ```sh
   tmact workflow report implementation \
     --config examples/openspec-full-workflow.yaml \
     --role swe \
     --stage apply \
     --kind complete \
     --change-hash sha256:... \
     --blocking=false \
     --body "implemented accepted change"
   ```

6. PM archives only after QA passes. After archive moves the change folder, the
   runner finishes as `implemented` when it has already recorded the PM archive
   completion report.

## Decisions

- CLI reports are the primary durable state channel.
- Marker parsing remains as an opt-in legacy fallback for a transition period,
  not as the preferred prompt instruction.
- `/clear` is part of workflow prompt dispatch and is controlled by config.
- The default clear command is `/clear` plus Enter.
- The clear step must be visible in dry-run output and trace events.
- The runner must check stop requests before clear and again before prompt.
- Report commands write local JSONL through structured code paths; they do not
  execute shell fragments from agent output.

## Success Criteria

- `tmact workflow report review` appends a valid phase 1 comment that can
  satisfy the existing discussion gate.
- `tmact workflow report implementation` appends a valid phase 2 comment that
  can satisfy the existing implementation gate.
- Discussion and implementation prompts include concrete report commands with
  the current config path, role/stage, change hash, and expected kind.
- Live workflow prompt dispatch sends `/clear` before prompts when configured;
  dry-run prints the clear plan without touching tmux.
- Workflows can complete without parsing any new marker text from pane output.
- Legacy marker parsing can still read existing pane-output markers when the
  operator explicitly enables fallback.
- PM archive completion can finish cleanly after `openspec archive` moves the
  change folder.
- `openspec validate workflow-cli-status-reporting --strict` and
  `go test ./...` pass.
