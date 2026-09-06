# Local feedback triage — 2026-09-07

Reviewed 14 local entries dated 2026-08-13 through 2026-09-06, grouped into
10 issues. The original local feedback log is preserved. This page records
resolution evidence without copying private pane output or task prompts.

| Feedback | Status and evidence |
| --- | --- |
| Version-number Claude runtime rejected (five entries, Sep 1–6) | Fixed the fallback matcher for the newer `auto mode on · … · ← for agents` footer. Version numbers alone still do not identify an agent. Dry-run reuse succeeded on the reported pane with process inspection unavailable. |
| `dispatch-work --wait` hangs after completion (Aug 27) | Wait now captures terminal attributes and distinguishes dim suggestions from real drafts. The reported pane reached input-ready after its stable window; regression tests keep drafts from satisfying input-ready. |
| Long prompt loses its beginning (Sep 1) | Mitigated, original agent-side loss not reproduced. Single-line payloads over 1KB now use bracketed paste; a transport regression test verifies all UTF-8 bytes and the target. Submission confirmation refuses to retry an unmatched styled draft/partial paste. Working-state evidence still cannot prove the agent received every byte. |
| Transient generic confirmations interrupt agent_dev (Aug 17) | Recheck after 500ms without sending keys. Tests cover disappearance and persistent prompts; permission-specific prompts retain immediate stopping behavior. |
| `ask` needs context-preserving first dispatch (Sep 3) | Added `--no-clear` for a new question. Existing `--thread` follow-ups already preserve context. CLI forwarding test added. |
| Delayed loop dry-run omits schedule and text (Aug 26) | `--dry-run --once` emits schedule previews with eligible times, intervals, and complete steps. Next-day flow regression test verifies the preview without input delivery. |
| `capture --session` unclear (Aug 26) | Help explicitly documents that capture takes one exact target: `work:0.0` or `%7`. No new session-selection behavior. |
| `usage --provider claude --json` produces no output (Aug 18) | Not reproduced: current command returned valid JSON with Claude data in 8.45s. No speculative change. |
| Capture exposes ghost input as a draft (Aug 13) | Previously fixed in `c03b284`; existing capture annotation remains in place. |
| statusd fails to recover from unavailable bind address (Aug 14) | Previously fixed in `04ac86b`; no further change. |

Validation: frontend build, 276 frontend tests, full Go suite, skill installer
checks, and macOS signing-script tests passed. Runtime smoke checks used only
dry-run dispatch and read-only wait; no live agent received a test prompt.
