# Tasks: Generalize Permission Prompt Stops

## 1. OpenSpec Review

- [x] Run the discussion workflow with PM, SWE, QA, and reviewer roles.
- [x] Resolve any blocking comments.
- [x] Finish phase 1 with an agreed artifact hash.

## 2. Implementation

- [x] Add generic prompt metadata to loop pane state and stop events.
- [x] Change loop stop checks to use `prompt.Detect`.
- [x] Change phase 1 workflow observation to use `prompt.Detect`.
- [x] Change phase 2 workflow observation to use `prompt.Detect`.
- [x] Preserve directory-access compatibility fields.
- [x] Update docs for generalized prompt-stop behavior.

## 3. Verification

- [x] Add or update prompt/loop tests for non-directory prompt stops.
- [x] Add or update workflow tests for non-directory prompt stops.
- [x] Run `openspec validate generalize-permission-prompt-stops --strict`.
- [x] Run `go test ./...`.
