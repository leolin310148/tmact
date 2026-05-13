# Tasks: Workflow CLI Status Reporting

## 1. OpenSpec Review

- [ ] Run the discussion workflow for this change.
- [ ] Resolve PM/SWE/QA/reviewer comments.
- [ ] Finish phase 1 with an agreed artifact hash.

## 2. Report CLI

- [x] Add `tmact workflow report review`.
- [x] Add `tmact workflow report implementation`.
- [x] Validate role, stage, kind, hash, blocking, reply-to, and body inputs.
- [x] Append reports through existing phase 1 and phase 2 JSONL helpers.
- [x] Add command help and manifest entries.

## 3. Prompt Dispatch

- [x] Add prompt dispatch config defaults.
- [x] Send `/clear` before live workflow prompts when configured.
- [x] Emit dry-run clear plan events without touching tmux.
- [x] Check stop requests before clear and before prompt.
- [x] Update discussion prompts to prefer report commands.
- [x] Update implementation prompts to prefer report commands.
- [x] Keep marker parsing as configurable legacy fallback.

## 4. Archive Completion

- [x] Finish phase 2 cleanly when PM archive report exists and OpenSpec archive
  moved the change folder.
- [x] Preserve blocked behavior for missing artifacts before archive completion.

## 5. Verification

- [x] Add unit tests for review report validation and JSONL append.
- [x] Add unit tests for implementation report validation and JSONL append.
- [x] Add runner tests for clear-before-prompt dry-run and live send order.
- [x] Add tests for stop request before clear and before prompt.
- [x] Add tests for archive-after-report missing change folder handling.
- [x] Run `openspec validate workflow-cli-status-reporting --strict`.
- [x] Run `go test ./...`.
