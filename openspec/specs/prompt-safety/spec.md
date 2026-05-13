# prompt-safety Specification

## Purpose
Define safety requirements for stopping unattended tmux automation when agent
panes show known interactive permission, approval, trust, or confirmation
prompts.

## Requirements
### Requirement: Generic Prompt Stop Detection

The system SHALL stop unattended runners when any known interactive permission
or approval prompt is visible in a target pane.

#### Scenario: Loop stops on command approval prompt

- GIVEN a loop captures a pane showing a known command approval prompt
- AND `stop_on_permission_prompt` is enabled
- WHEN the loop evaluates the pane state
- THEN it stops before sending more tmux input
- AND the stop details include the detected prompt type

#### Scenario: Loop preserves directory-access details

- GIVEN a loop captures a pane showing a directory-access prompt
- AND `stop_on_permission_prompt` is enabled
- WHEN the loop evaluates the pane state
- THEN it stops before sending more tmux input
- AND the stop details preserve the existing directory-access metadata

#### Scenario: Workflow discussion stops on generic approval prompt

- GIVEN a discussion workflow role pane shows a known generic approval prompt
- WHEN the workflow observes role panes
- THEN the workflow stops before sending the next role prompt
- AND the blocked reason remains `permission_prompt`
- AND the operator can see the prompt type or title in the error details

#### Scenario: Workflow implementation stops on generic approval prompt

- GIVEN an implementation workflow role pane shows a known patch approval or
  command approval prompt
- WHEN the workflow observes role panes
- THEN the workflow stops before sending the next stage prompt
- AND the blocked reason remains `permission_prompt`
- AND the operator can see the prompt type or title in the error details

#### Scenario: Watcher remains directory access only

- GIVEN a watcher config contains a directory-access allowlist rule
- WHEN the watcher captures a directory-access prompt
- THEN it may evaluate the configured allowlist as before
- AND it MUST NOT automatically answer command, patch, trust-folder, or generic
  confirmation prompts as part of this change
