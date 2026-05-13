# Delta for Agent Development Workflow

## ADDED Requirements

### Requirement: Serialized Multi-Role OpenSpec Review

The system SHALL coordinate OpenSpec artifact review through a serialized role
order of PM, SWE, QA, and reviewer.

#### Scenario: Valid role mapping

- GIVEN a workflow config maps `pm`, `swe`, `qa`, and `reviewer` to configured
  agent names
- WHEN the config is loaded
- THEN the workflow is valid for v1 role orchestration

#### Scenario: Missing role

- GIVEN a workflow config missing any required role key
- WHEN the config is loaded
- THEN validation fails before tmux input is sent

#### Scenario: Serialized turn order

- GIVEN no role has accepted the current change hash
- WHEN the workflow selects the next pending role
- THEN it selects the first stale role in configured `role_order`

### Requirement: OpenSpec Artifact Set

The system SHALL treat the full OpenSpec change artifact set as the reviewed
artifact.

#### Scenario: Existing change folder

- GIVEN `openspec/changes/<change>/` exists
- WHEN a workflow run starts
- THEN the workflow reads artifact files from that folder

#### Scenario: Missing proposal

- GIVEN a change folder exists without `proposal.md`
- WHEN `create_missing_proposal` is true
- THEN the workflow creates a deterministic proposal template

#### Scenario: Change name escapes openspec/changes

- GIVEN a config whose `change` value resolves outside `openspec/changes/`
- WHEN the config is loaded
- THEN validation fails before state files are created or tmux input is sent

#### Scenario: Artifact revision changes

- GIVEN any reviewed artifact changes during discussion
- WHEN the workflow records or evaluates agreement
- THEN agreement is tied to the current `change_hash`

### Requirement: Canonical Change Hash

The system SHALL compute a canonical SHA-256 `change_hash` over `proposal.md`,
`design.md`, `tasks.md`, and `specs/*/spec.md`.

#### Scenario: Hash includes spec deltas

- GIVEN `specs/example/spec.md` changes
- WHEN the workflow recomputes `change_hash`
- THEN the hash changes even if `proposal.md` is unchanged

#### Scenario: CRLF normalization

- GIVEN two artifact sets differ only by CRLF versus LF line endings
- WHEN the workflow computes `change_hash`
- THEN both sets produce the same hash

### Requirement: OpenSpec Validation Gate

The system SHALL run OpenSpec validation before any acceptance can satisfy the
phase gate.

#### Scenario: Validation passes

- GIVEN OpenSpec validation succeeds for change hash `A`
- WHEN a role accepts hash `A`
- THEN that acceptance may satisfy the gate

#### Scenario: Validation fails

- GIVEN OpenSpec validation fails for change hash `A`
- WHEN a role accepts hash `A`
- THEN that acceptance does not satisfy the gate

#### Scenario: Artifacts change during validation

- GIVEN validation starts for hash `A`
- AND artifacts change to hash `B` before validation finishes
- WHEN the result is recorded
- THEN the result is stale and cannot satisfy acceptance for hash `B`

### Requirement: Structured Comment Stream

The system SHALL persist role comments from strict
`TMAct-OpenSpec-Comment:` pane-output markers.

#### Scenario: Role leaves acceptance comment

- GIVEN a role outputs a valid marker with `kind=accept`
- WHEN the workflow parses pane output
- THEN it records an acceptance comment for the referenced `change_hash`

#### Scenario: Ambiguous prose

- GIVEN pane output says "looks good" without the strict marker
- WHEN the workflow parses pane output
- THEN the output does not affect the gate

#### Scenario: Blocking comment

- GIVEN a role outputs `kind=request_changes` with `blocking=true`
- WHEN the gate is evaluated for that hash
- THEN the gate remains open with reason `blocking_comments`

#### Scenario: Blocking comment resolved by decision

- GIVEN a blocking comment exists for the current hash
- WHEN a later `decision` comment references it with `reply_to`
- THEN the blocking comment no longer prevents the gate

### Requirement: All-Role Agreement Gate

The system MUST NOT finish as agreed until all required roles accept the same
current `change_hash`, OpenSpec validation passed for that hash, and no
unresolved blocking comments remain for that hash.

#### Scenario: All roles agree to same current hash

- GIVEN PM, SWE, QA, and reviewer each accepted hash `A`
- AND `A` is the current `change_hash`
- AND OpenSpec validation passed for `A`
- AND no unresolved blocking comments remain for `A`
- WHEN the gate is evaluated
- THEN the outcome is `agreed`

#### Scenario: Earlier acceptances are stale

- GIVEN PM accepted hash `A`
- AND SWE changed an artifact, producing hash `B`
- WHEN the gate is evaluated
- THEN PM acceptance is stale and the workflow schedules PM for refresh before
  the gate can pass

#### Scenario: Acceptance withdrawn

- GIVEN a role accepted the current hash
- WHEN the same role later outputs `withdraw_accept`, `reject`, or
  `request_changes` for that hash
- THEN that role is no longer accepted for the current hash

### Requirement: Bounded Long-Running Workflow

The system SHALL run the review workflow with explicit turn, runtime, polling,
and stop bounds.

#### Scenario: Dry run

- GIVEN the user runs without `--execute`
- WHEN a prompt is planned
- THEN no text or keys are sent to tmux

#### Scenario: Max turns reached

- GIVEN the workflow reaches `max_turns` before agreement
- WHEN another role prompt would be scheduled
- THEN the run stops with outcome `needs_user`

#### Scenario: Max runtime reached

- GIVEN the workflow reaches `max_runtime` before agreement
- WHEN the runner observes the limit
- THEN the run stops with outcome `blocked` and reason `max_runtime`

#### Scenario: Permission prompt visible

- GIVEN a target pane shows a permission prompt
- WHEN the workflow observes that pane
- THEN the run stops with outcome `blocked` and reason `permission_prompt`

#### Scenario: Stop observed before tmux send

- GIVEN `tmact workflow stop` marks a run as stopping
- WHEN the runner reaches the next send point
- THEN it exits without sending more tmux input

### Requirement: Workflow Status

The system SHALL allow the operator to inspect workflow state from local files.

#### Scenario: Status while running

- GIVEN a workflow is in progress
- WHEN the operator runs `tmact workflow status`
- THEN output reports phase, turn count, pending role, latest validation,
  current `change_hash`, and run metadata

#### Scenario: Status after terminal outcome

- GIVEN a workflow stopped with `agreed`, `needs_user`, or `blocked`
- WHEN the operator runs `tmact workflow status`
- THEN output reports the final outcome and final `change_hash`
