# Delta for Agent Development Workflow

## ADDED Requirements

### Requirement: Workflow Status Report CLI

The system SHALL provide a workflow report CLI that writes durable review and
implementation status without relying on pane-output marker parsing.

#### Scenario: Review report records phase 1 comment

- GIVEN a workflow config targets an OpenSpec change
- WHEN an agent runs `tmact workflow report review` with role, kind,
  change hash, OpenSpec validity, blocking flag, and optional body
- THEN tmact appends a validated phase 1 comment to the change comment stream
- AND the discussion gate can evaluate that report without reading pane prose

#### Scenario: Implementation report records phase 2 comment

- GIVEN a workflow config targets an OpenSpec change
- WHEN an agent runs `tmact workflow report implementation` with role, stage,
  kind, change hash, blocking flag, and optional body
- THEN tmact appends a validated phase 2 comment to the change comment stream
- AND the implementation gate can evaluate that report without reading pane
  prose

#### Scenario: Invalid report is rejected

- GIVEN a report has an unknown role, stage, kind, or escaping change path
- WHEN tmact validates the report
- THEN it rejects the report without appending JSONL state

### Requirement: Report-First Workflow Prompts

The system SHALL instruct workflow agents to report completion through
`tmact workflow report` commands instead of primary pane-output markers.

#### Scenario: Discussion prompt includes report command

- GIVEN the discussion workflow prompts a role for hash `A`
- WHEN it builds the role prompt
- THEN the prompt includes a concrete `tmact workflow report review` command
  with config path, role, expected kind, and hash `A`

#### Scenario: Implementation prompt includes report command

- GIVEN the implementation workflow prompts a stage for accepted hash `A`
- WHEN it builds the stage prompt
- THEN the prompt includes a concrete `tmact workflow report implementation`
  command with config path, role, stage, expected kind, and hash `A`

#### Scenario: Marker fallback remains transitional

- GIVEN legacy marker fallback is enabled
- WHEN pane output contains an existing strict marker
- THEN tmact may parse it as before
- BUT new prompts still prefer the report command as the primary status path

### Requirement: Clear Before Workflow Prompt

The system SHALL clear configured workflow agent panes before sending a new
workflow assignment prompt when clear-before-prompt is enabled.

#### Scenario: Live prompt clears agent context

- GIVEN clear-before-prompt is enabled
- AND a live workflow is about to prompt a configured agent pane
- WHEN tmact dispatches the prompt
- THEN it sends the configured clear command before the assignment prompt
- AND waits the configured clear delay before sending the assignment

#### Scenario: Dry run shows clear plan

- GIVEN clear-before-prompt is enabled
- AND the operator runs a workflow with `--dry-run --once`
- WHEN tmact plans the next prompt
- THEN it reports the planned clear operation
- AND it sends no tmux input

#### Scenario: Stop observed before clear

- GIVEN a stop request is recorded before prompt dispatch
- WHEN the workflow reaches the dispatch point
- THEN it exits without sending clear or prompt input

#### Scenario: Stop observed after clear before prompt

- GIVEN a stop request is recorded after clear is sent but before prompt is sent
- WHEN the workflow reaches the second stop check
- THEN it exits without sending the assignment prompt

### Requirement: Archive Completion After Report

The system SHALL finish implementation cleanly when PM reports archive
completion and OpenSpec moves the change folder.

#### Scenario: Archive moved change folder after PM report

- GIVEN SWE apply completed
- AND QA verify passed
- AND PM reported archive complete for the accepted hash
- AND `openspec archive` moved the original change folder
- WHEN the implementation runner evaluates completion
- THEN it finishes with outcome `implemented`
- AND it does not fail only because the original change folder no longer exists
