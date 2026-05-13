# Design: Workflow CLI Status Reporting

## Overview

Add a structured report subcommand under `tmact workflow` and make workflow
prompts instruct agents to call that command. The runner continues to evaluate
the existing phase 1 and phase 2 gate functions, but the preferred way to create
comments is now local CLI writes rather than pane-output marker parsing.

## CLI Shape

Phase 1 review report:

```sh
tmact workflow report review \
  --config PATH \
  --role ROLE \
  --kind accept|request_changes|reject|withdraw_accept|decision \
  --change-hash sha256:... \
  [--openspec-valid] \
  [--blocking=true|false] \
  [--reply-to COMMENT_ID] \
  [--body TEXT]
```

Phase 2 implementation report:

```sh
tmact workflow report implementation \
  --config PATH \
  --role swe|qa|pm \
  --stage apply|verify|archive \
  --kind complete|pass|fail|request_changes|blocked|decision|withdraw \
  --change-hash sha256:... \
  [--blocking=true|false] \
  [--reply-to COMMENT_ID] \
  [--body TEXT]
```

The command loads the workflow config to resolve the change directory, validates
the change name with the same `ChangeDir` rules, validates role/stage/kind, and
then appends through the existing JSONL append helpers.

## Prompt Dispatch

Add workflow config fields:

```yaml
prompt_dispatch:
  clear_before_prompt: true
  clear_command: /clear
  clear_delay: 5s
  legacy_marker_fallback: false
```

Before sending a prompt in live mode, the runner:

1. Checks stop request.
2. Sends the configured clear command plus Enter to the target agent pane.
3. Waits for `clear_delay`.
4. Checks stop request again.
5. Sends the assignment prompt.

Dry-run emits planned `clear` and `prompt` trace events but sends nothing.

## Report-First Prompts

Prompts should include the exact report command the agent must run when done.
The old marker text should either be omitted or moved to a clearly labeled
legacy fallback section when `legacy_marker_fallback` is true.

The report command should use the config path from the active runner options so
agents do not guess which workflow is active.

## Gate Evaluation

The existing phase 1 and phase 2 comment loaders can remain unchanged because
reports append the same durable comment structures. Pane observation still
captures prompts for safety and may parse legacy markers when enabled, but new
workflow progress should not depend on reading newly emitted marker lines.

## Archive Completion

When PM reports `stage=archive kind=complete`, the implementation runner should
be able to finish even if `openspec archive` has moved the original change
folder. A missing original change directory after an archive completion report
should be interpreted as expected archive movement, not as a generic artifact
hash failure.

Implementation reports are also mirrored under `.tmact/workflow/<change>/` so
the runner can still observe the completed apply/verify/archive sequence after
the OpenSpec change directory is gone.

## Safety

- The report command only writes workflow state for the configured change.
- Reports must not accept absolute or escaping change paths.
- `/clear` is only sent to configured workflow role panes.
- Stop requests must be observed before clear and before prompt.
- Permission or approval prompts still stop live workflows.
- The report command does not bypass phase 2 preconditions; it only records
  role/stage output. Gate evaluation still decides what can advance.
