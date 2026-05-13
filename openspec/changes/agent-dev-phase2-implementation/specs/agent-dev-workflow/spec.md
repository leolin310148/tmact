# Delta for Agent Development Workflow

## ADDED Requirements

### Requirement: Serialized OpenSpec Implementation Phase

The system SHALL coordinate an accepted OpenSpec change through a serialized
implementation phase of SWE apply, QA verify, and PM archive.

#### Scenario: Valid phase 2 role mapping

- GIVEN a workflow config maps `swe`, `qa`, and `pm` to configured agent names
- WHEN the implementation workflow config is loaded
- THEN the workflow is valid for phase 2 role orchestration

#### Scenario: Serialized stage order

- GIVEN phase 1 agreed for the targeted change
- AND no phase 2 stage has completed
- WHEN the workflow selects the next pending stage
- THEN it selects SWE apply

#### Scenario: QA waits for SWE

- GIVEN SWE has not completed the apply stage
- WHEN the workflow evaluates the phase 2 gate
- THEN QA verify is not prompted

#### Scenario: PM waits for QA

- GIVEN SWE apply completed
- AND QA verify has not passed
- WHEN the workflow evaluates the phase 2 gate
- THEN PM archive is not prompted

### Requirement: Phase 1 Agreement Precondition

The system SHALL require an agreed phase 1 state before live phase 2 execution.

#### Scenario: Phase 1 agreed

- GIVEN `phase1-state.json` has outcome `agreed`
- AND the current OpenSpec artifact hash matches the phase 1 `change_hash`
- WHEN the operator starts `workflow implement --execute`
- THEN the implementation workflow may prompt the first pending phase 2 stage

#### Scenario: Phase 1 missing

- GIVEN `phase1-state.json` is missing
- WHEN the operator starts `workflow implement --execute`
- THEN the workflow stops before tmux input is sent

#### Scenario: Phase 1 not agreed

- GIVEN `phase1-state.json` has an outcome other than `agreed`
- WHEN the operator starts `workflow implement --execute`
- THEN the workflow stops before tmux input is sent

#### Scenario: Accepted artifact hash changed

- GIVEN phase 1 agreed for hash `A`
- AND the current OpenSpec artifact hash is `B`
- WHEN the implementation workflow evaluates preconditions
- THEN the workflow stops before tmux input is sent

### Requirement: SWE Apply Stage

The system SHALL prompt the SWE role to apply the accepted OpenSpec change
before verification or archive can proceed.

#### Scenario: Apply prompt

- GIVEN phase 1 agreed for hash `A`
- AND SWE apply has not completed
- WHEN the workflow plans a prompt
- THEN the prompt includes change name, hash `A`, apply-stage instructions, and
  the required phase 2 marker format

#### Scenario: SWE completes apply

- GIVEN SWE outputs a valid phase 2 marker with `stage=apply`
- AND the marker has `kind=complete`
- AND the marker references the accepted `change_hash`
- WHEN the workflow parses pane output
- THEN SWE apply is recorded as complete

#### Scenario: SWE marker for stale hash

- GIVEN the accepted phase 2 hash is `A`
- AND SWE outputs an apply marker for hash `B`
- WHEN the gate is evaluated
- THEN SWE apply remains pending for hash `A`

### Requirement: QA Verify Stage

The system SHALL prompt the QA role to verify implementation after SWE apply
has completed.

#### Scenario: Verification prompt

- GIVEN SWE apply completed for hash `A`
- AND QA verify has not passed
- WHEN the workflow plans a prompt
- THEN the prompt includes change name, hash `A`, configured verification
  commands, and the required phase 2 marker format

#### Scenario: QA passes verification

- GIVEN QA outputs a valid phase 2 marker with `stage=verify`
- AND the marker has `kind=pass`
- AND the marker references the accepted `change_hash`
- WHEN the workflow parses pane output
- THEN QA verify is recorded as passed

#### Scenario: QA fails verification

- GIVEN QA outputs a valid phase 2 marker with `stage=verify`
- AND the marker has `kind=fail`
- WHEN the gate is evaluated
- THEN PM archive is not prompted

#### Scenario: Verification commands are structured

- GIVEN a workflow config contains verification commands
- WHEN the config is loaded
- THEN each command is represented as command plus args rather than a shell
  string

### Requirement: PM Archive Stage

The system SHALL prompt PM to archive only after implementation and verification
are complete and OpenSpec validation is passing.

#### Scenario: Archive prompt

- GIVEN SWE apply completed for hash `A`
- AND QA verify passed for hash `A`
- AND `openspec validate <change> --strict` passes
- WHEN the workflow plans a prompt
- THEN it prompts PM to run the configured archive command

#### Scenario: Validation fails before archive

- GIVEN SWE apply completed
- AND QA verify passed
- AND strict OpenSpec validation fails
- WHEN the workflow evaluates PM archive readiness
- THEN PM archive is not prompted

#### Scenario: PM completes archive

- GIVEN PM outputs a valid phase 2 marker with `stage=archive`
- AND the marker has `kind=complete`
- AND the marker references the accepted `change_hash`
- WHEN the workflow parses pane output
- THEN the workflow may finish with outcome `implemented`

### Requirement: Phase 2 Status And Stop

The system SHALL allow the operator to inspect and stop phase 2 through the
existing workflow status and stop surfaces.

#### Scenario: Status during phase 2

- GIVEN a phase 2 implementation workflow is in progress
- WHEN the operator runs `tmact workflow status --config <path>`
- THEN output reports the current implementation stage, accepted
  `change_hash`, stage outcomes, latest validation, and run metadata

#### Scenario: Status after implementation

- GIVEN phase 2 finished with outcome `implemented`
- WHEN the operator runs `tmact workflow status --config <path>`
- THEN output reports the final phase 2 outcome

#### Scenario: Stop observed before phase 2 send

- GIVEN `tmact workflow stop` marks the phase 2 run as stopping
- WHEN the runner reaches the next send point
- THEN it exits without sending more tmux input

### Requirement: Phase 2 Safety Boundaries

The system SHALL preserve the workflow safety boundaries from phase 1 during
implementation.

#### Scenario: Dry run

- GIVEN the user runs `workflow implement` without `--execute`
- WHEN a prompt is planned
- THEN no text or keys are sent to tmux

#### Scenario: Permission prompt visible

- GIVEN a target pane shows a permission prompt
- WHEN the phase 2 workflow observes that pane
- THEN the run stops with outcome `blocked` and reason `permission_prompt`

#### Scenario: Captured prose is ignored

- GIVEN pane output says "verification passed" without the strict phase 2
  marker
- WHEN the workflow parses pane output
- THEN the output does not advance the phase 2 gate
