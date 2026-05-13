# Proposal: Generalize Permission Prompt Stops

## Intent

Make tmact stop unattended automation when any known interactive permission or
approval prompt is visible, not only Codex directory-access prompts.

## User

The user is a tmact operator running local tmux automation against Codex,
Claude, Copilot, Gemini, or shell panes. They need unattended loops and
OpenSpec workflows to stop before an agent approves a command, patch, folder
trust prompt, or other interactive approval request.

## Problem

The prompt package already detects several interactive prompt types, including
command approval, patch approval, trust-folder prompts, generic confirmations,
and waiting-for-approval states. However, loop and workflow safety checks still
call the directory-access-specific detector. This leaves a gap where a pane can
show another known approval prompt without the runner treating it as a stop
condition.

## Scope

In scope:

- Reuse the existing generic prompt detector for loop and workflow stop checks.
- Preserve existing directory-access details where callers already expose them.
- Record generic prompt metadata in stop events and workflow block reasons.
- Add tests for command approval, patch approval, trust-folder, waiting
  approval, and directory-access compatibility.
- Update docs to describe the broader stop behavior.

Out of scope:

- Automatically accepting command, patch, trust-folder, or generic confirmation
  prompts.
- Broadening watcher allowlists to approve arbitrary commands or patches.
- Changing pane runtime classification beyond what is required for stop checks.
- Adding new dependencies or a background service.

## Success Criteria

- `tmact loop` stops when `prompt.Detect` identifies any known interactive
  prompt and still reports directory-access metadata for existing consumers.
- `tmact workflow discuss` and `tmact workflow implement` stop when any
  configured role pane shows a known interactive prompt.
- JSON event/state output includes enough prompt type/title detail for the
  operator to understand why automation stopped.
- Existing directory-access watcher behavior remains unchanged.
- `go test ./...` passes.
