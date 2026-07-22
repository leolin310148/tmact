# tmact Agent-Ergonomics Work Items

This queue turns the 2026-05-01 through 2026-07-22 Claude/Codex session-log
analysis into small, dependency-ordered implementation tasks. The dominant
observed failure mode was that agents use `tmact dispatch-work`, then fall back
to hand-written `sleep`, `tmux capture-pane`, polling loops, `send-keys`, and
`kill-session` because tmact does not yet expose bounded wait, incremental
capture, structured completion, or a complete CLI session lifecycle.

## Worker contract

- Work on branch `main` in `/Users/puni/w/puni/tmact`.
- Read `AGENTS.md`, `CLAUDE.md`, and this file before starting an item.
- Select only the first unchecked item. Never start a second item in the same
  cycle.
- Start only from a clean worktree. If it is dirty, make no changes and report
  the exact paths as a blocker. Never reset, clean, stash, or discard work.
- Preserve dry-run defaults, exact targets, peer behavior, prompt refusal, and
  folder-trust safety boundaries.
- Keep Go dependencies minimal. Prefer existing `internal/tmux`, `panestate`,
  `statusd`, `workflow`, and `agentspend` primitives over parallel subsystems.
- Add focused tests and update CLI help/catalog/LLM instructions when a public
  command or flag changes.
- Run targeted tests plus `go test ./...` for Go-only changes. For frontend or
  embedded-web changes, run `make test`.
- Complete atomically: implementation, tests, docs/help, and exactly this
  item's checkbox belong in one commit. Use a concise imperative commit subject.
- Do not push.
- If blocked, do not check the item or commit partial work. Leave the tree clean
  when safely possible without discarding pre-existing work, and report the
  blocker.
- If every item is checked, make no changes and report queue completion.

## Queue

- [x] **WI-001 — Add a safe `tmact capture` command.**
  Expose the existing plain-text pane capture through a top-level CLI command
  accepting one exact target, `--lines`, `--non-empty`, and `--json`. Reuse
  `internal/tmux.CapturePane`; do not shell out through a second implementation.
  Text mode prints only captured text. JSON includes canonical target/pane,
  requested line count, text, and truncation metadata when knowable. Support
  local targets first and return an explicit unsupported error for peer targets
  rather than silently treating them as local. Add command/help tests and unit
  tests that do not require live tmux.

- [x] **WI-002 — Add opaque incremental cursors to `tmact capture`.**
  Add `--after CURSOR` and return a new opaque cursor in JSON. A repeated capture
  should emit only newly observed terminal rows when overlap can be established;
  if history rolled or the cursor cannot be reconciled, return a documented
  reset/full snapshot indication. Keep cursor contents versioned and bounded;
  do not persist pane contents or introduce SQLite. Add deterministic tests for
  append, unchanged, rewritten-screen, rollover, invalid, and version-mismatch
  cases.

- [x] **WI-003 — Add bounded `tmact wait` for pane state transitions.**
  Implement a read-only command that accepts exactly one target/session,
  `--until input-ready|working|needs-human|gone`, `--require-transition`,
  `--settle`, `--poll-interval`, `--timeout`, and `--json`. Reuse pane
  classification and capture helpers. Distinguish terminal reasons
  `condition_met`, `needs_human`, `timeout`, and `pane_gone`; never claim that
  idle alone proves task success. Permission/approval prompts must return
  `needs_human`, not be confirmed. Add cancellation and fake-clock/dependency
  tests without live tmux.

- [x] **WI-004 — Keep pane DOM stable during selection and clicks.**
  Stop live pane repaints from invalidating browser Selection/Range objects or
  click targets. Keep receiving WebSocket patches and updating the pane
  buffer/cache, but defer `pre#content` DOM commits while any pointer
  interaction is in progress, selection mode is enabled, or a non-collapsed
  browser selection belongs to the pane. Hold the pointer lock through click
  dispatch; on unlock, render only the newest pending frame once. A pane switch
  must clear the old interaction/selection and immediately render the newly
  selected pane, never flush stale content from the prior pane. Preserve the
  imperative terminal renderer, path marking, Mermaid rendering, scroll
  behavior, and rAF coalescing; do not throttle or reconnect the WebSocket.
  Show an unobtrusive, accessible "Live updates paused while selecting"
  indicator while a frame is deferred. Add focused `ContentPane`/App tests for
  DOM identity during pointer-to-click, selection retention across incoming
  text, latest-frame flush on selection collapse, selection-mode locking, and
  pane switching. Run frontend Vitest and `make test`, then use `borz` against a
  rapidly changing local pane to verify that text can be selected/copied and a
  previewable path can be clicked without the target disappearing; record the
  manual case in `docs/smoke-test.md` when appropriate.

- [x] **WI-005 — Integrate bounded waiting into `dispatch-work`.**
  Add opt-in `--wait`, `--wait-timeout`, `--wait-settle`, and `--result-lines`
  flags. Record the post-submit baseline, require evidence that the submission
  was accepted, then wait for stable input-ready or a terminal blocker. Preserve
  existing JSON and behavior without `--wait`; add a structured wait/result
  section when enabled. Support local panes and fail explicitly if peer waiting
  is unavailable. Test immediate-idle, working-to-idle, permission, timeout, and
  disappeared-pane cases.

- [x] **WI-006 — Add recoverable CLI session close/history/reopen.**
  Introduce `tmact session close`, `tmact session closed`, and
  `tmact session reopen`, reusing statusd/web closed-session persistence.
  Closing is dry-run by default and requires `--execute`; targets must be exact
  and broad deletion is out of scope. Reopen restores recorded name/cwd/runtime
  intent where safely supported and refuses conflicts. Add service and CLI tests.

- [x] **WI-007 — Add guarded session create and agent resume.**
  Add `tmact session create NAME --dir DIR` for an idle shell and
  `tmact session resume NAME --dir DIR --agent claude|codex --session-id ID`.
  Both are dry-run by default, validate canonical cwd, refuse busy/different
  runtimes and prompts, and require `--execute`. Never infer a resume id from
  pane text. Keep provider command construction unit-testable and update help.

- [x] **WI-008 — Extract normalized Claude/Codex session-log readers.**
  Create a shared internal package for provider discovery and streaming JSONL,
  factoring path resolution out of `internal/agentspend` without changing spend
  results. Normalize timestamp, provider, session id, cwd, role, event kind,
  tool, command, exit code, and duration where present. Handle oversized lines,
  malformed records, unknown event types, and current Claude/Codex tool-call
  shapes. Use only redacted synthetic fixtures.

- [x] **WI-009 — Add privacy-safe `tmact log search`.**
  Implement `tmact log search QUERY` with `--provider`, `--since`, `--cwd`,
  `--kind`, `--limit`, `--json`, and opt-in `--show-content`. Default output
  includes normalized metadata and command verb/subcommand only, never raw
  prompts, tool output, environment values, or full arguments. Search both
  providers through WI-008 and report provider parse coverage/errors. Add help
  and fixture-based tests.

- [x] **WI-010 — Add `tmact log stats` and an incremental plain-file index.**
  Aggregate by provider, tool, command, and subcommand with `--since` and JSON.
  Cache safe normalized fields under the tmact config directory, keyed by source
  path, size, mtime, and parser version. Use atomic plain-file writes and rebuild
  after missing/corrupt cache. Add `tmact log doctor` for file counts, skipped
  records, schema coverage, and cache health. Do not add SQLite.

- [x] **WI-011 — Update canonical skills for the new CLI workflow.**
  Edit only canonical `skills/`. Change `tmact-dispatch` and `agent-loop` to use
  bounded `tmact wait`/`capture` and guarded `tmact send`, not raw capture-pane,
  polling loops, or send-keys. Make routine preflight concise and document log
  privacy defaults. Extend `scripts/install-skills.sh --check` to report active
  duplicate/orphan backup skill directories without deleting them. Run the
  skill-creator validator, the install check, and relevant tests.

- [x] **WI-012 — Protect web session mutations and validate reopen history.**
  Treat `/api/session/kill` and `/api/session/reopen` as destructive browser
  mutations. Reject cross-site browser requests and CORS-safelisted bodies that
  bypass preflight; require the expected JSON request shape without breaking
  same-origin PWA use or authenticated/configured peer proxying. A rejected
  request must never call `KillSession` or `NewSession`. Reopen must resolve one
  exact local entry from `ClosedSessions` and use its recorded cwd; reject an
  unknown name or a caller-supplied cwd that differs from history. Keep exact
  local-session and peer boundaries. Add handler tests for cross-origin and
  `text/plain` POSTs, valid same-origin JSON, unknown history, cwd tampering,
  and the peer path. Run focused web tests and `go test ./...`.

- [x] **WI-013 — Make command summaries fail closed around environment values.**
  Replace the `strings.Fields` command-summary parsing that can turn part of a
  quoted environment value into the reported executable. Parse only enough
  shell-word structure to safely skip complete leading assignments and common
  `env` forms; never evaluate expansions, and return an empty/conservative
  summary for malformed or ambiguous syntax. Cover single/double quotes,
  escaped spaces, multiple assignments, option-bearing `env`, unbalanced
  quotes, and `SECRET='alpha beta' git status`. Verify both default `log search`
  output and the plain-file stats index contain neither complete nor partial
  environment values. Bump the parser/cache identity so an existing index made
  by the vulnerable parser is rebuilt instead of reused. Run focused log tests
  and `go test ./...`.

- [ ] **WI-014 — Make recoverable session close durable before killing.**
  A successful `tmact session close --execute` must guarantee its reopen intent
  survived an atomic disk write. Expose persistence errors from
  `ClosedSessionLog` instead of silently ignoring them, durably stage the exact
  history entry before `KillSession`, and do not kill when staging fails. If
  killing fails, roll back the staged entry and report any rollback failure
  without hiding the original error. Preserve explicit best-effort behavior
  only for non-destructive daemon tracking where it is intentional. Add tests
  for unwritable/write/rename failure, kill-not-called on persistence failure,
  kill rollback, and visibility from a newly constructed history instance.
  Check other mutation callers for the updated error contract, then run focused
  session/statusd/web tests and `go test ./...`.

- [ ] **WI-015 — Enforce a real wall-clock deadline for `tmact wait`.**
  Make `--timeout` bound target resolution, pane capture, settling, and polling,
  not just the gaps between completed tmux calls. Thread a deadline-bearing
  context through target resolution as well as capture using cancellable tmux
  subprocesses; do not leak goroutines. Expiration must still return the
  structured `timeout` terminal reason/report, while operator cancellation
  remains distinguishable. Add deterministic blocking dependency tests for
  resolve, capture, and poll waits, plus CLI and `dispatch-work --wait`
  regression coverage. Run focused wait/dispatch/CLI tests and `go test ./...`.
