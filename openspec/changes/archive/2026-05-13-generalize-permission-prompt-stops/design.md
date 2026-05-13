# Design: Generalized Permission Prompt Stops

## Overview

The existing prompt package has two layers:

- `prompt.DetectDirectoryAccess`, which returns directory path and selected
  option details.
- `prompt.Detect`, which wraps directory access and also recognizes other
  interactive approval prompts.

This change should move safety stop checks in runners to the generic detector
while keeping the directory-access-specific fields available for compatibility.

## Loop Runner

`internal/loop` currently stores `PermissionPrompt *prompt.DirectoryAccess` in
its pane state. Add a generic prompt field alongside it:

```go
InteractivePrompt *prompt.Prompt
```

The loop should call `prompt.Detect(raw)` once per captured pane. When a prompt
is detected and `stop_on_permission_prompt` is enabled, the stop event should
include the generic prompt metadata. If the detected prompt type is
`directory_access`, the existing `permission_prompt` field should continue to
be populated.

## Workflow Runners

Both phase 1 and phase 2 workflow runners should check `prompt.Detect(raw)`.
The returned error should include the role and prompt type, for example:

```text
permission_prompt in qa: command_approval Allow command?
```

The durable state can keep the existing `reason=permission_prompt` gate reason
for compatibility, but trace events or error text should expose the prompt type
and title.

## Watcher

`internal/watch` remains directory-access-only in this change. It may continue
to use `prompt.DetectDirectoryAccess` and its allowlist logic. Automatically
answering non-directory prompts requires a separate design because command and
patch approvals need different safety controls.

## Compatibility

Existing JSON fields should not be removed. New JSON fields are additive.
Existing tests for directory-access detection should continue to pass.

## Risks

- False positives could stop automation earlier than before. That is acceptable
  for unattended safety and should surface with prompt type/title details.
- Prompt text differs across agent versions. This change relies on the prompt
  detector patterns already present in the repository and adds focused samples
  rather than attempting broad fuzzy matching.
