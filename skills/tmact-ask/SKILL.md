---
name: tmact-ask
description: Ask another local AI agent for a definitive result with the blocking `tmact ask` and explicit `tmact reply` protocol. Use when the caller must receive an investigation result, verdict, implementation summary, review, second opinion, or other explicit answer in the current workflow. Trigger on "ask another agent", "get another agent's answer", "delegate and report back", "second opinion", "definitive answer", "問另一個 agent", "請 agent 回覆", "派工並回傳結果", "等 agent 的答案", and "找另一個 agent 調查". Do not use for background or fire-and-monitor work, peer dispatch, recurring schedules, multi-round implement/review loops, or context handoffs; use `tmact-dispatch`, `tmact-loop`, `agent-loop`, or `handoff` respectively.
---

# tmact-ask

Use `tmact ask` when the local caller must block until the answering agent
explicitly returns a result with `tmact reply`. Do not infer completion from pane
state, captured output, or process exit status.

`ask` is local-only. For work on a configured peer, use `tmact-dispatch` and
report that explicit request/reply delivery is unavailable.

## Pre-flight

Honor active agent and repository instructions first, including any required
shell-command prefix.

```bash
tmact version
```

Skip repeated environment inventory when this succeeds. If any flag, agent/model
allowlist, timeout, storage, or safety behavior is uncertain, run
`tmact help ask --json`; the installed CLI is authoritative.

## CLI feedback

When a tmact command is confusing, lacks a needed flag, produces unparseable
output, or returns an unhelpful error, immediately record it with
`tmact feedback "<what was awkward and what you expected>" --category ux|bug|feature|docs|perf --command <cmd>`.
Feedback stays local in `~/.tmact/feedback.jsonl` and is never uploaded.

## Ask for an explicit result

```bash
tmact ask SESSION --dir DIR --agent claude|codex|gemini \
  [--model MODEL] --prompt TEXT [--trust-folder] \
  [--timeout 30m] [--execute] [--json]
```

- `SESSION` is the first positional argument.
- `--dir`, `--agent`, and `--prompt` are required.
- `--model` is allowed only for a newly launched Claude or Codex agent and must
  match the installed CLI's allowlist.
- `--execute` enables side effects; without it the command is a dry-run.
- `--timeout` bounds the entire wait for an explicit reply.
- `--trust-folder` is the only opt-in trust exception. It supports Claude and
  Codex only and requires exact canonical pane-cwd/`--dir` equality.
- `--json` returns a machine-readable report.

Dry-run the exact session, directory, agent, model, prompt, timeout, and trust
choice first:

```bash
tmact ask investigation --dir ~/w/proj --agent codex \
  --prompt "find the root cause and return a concise verdict"
```

Once the plan is correct and authorized, repeat it with `--execute --json`.
Do not replace `ask` with `dispatch-work --wait` when a returned answer is
required.

If the user did not choose an agent, select one based on the task and active
repository guidance. Prefer Claude for implementation-heavy work and Codex for
review or a second opinion when no stronger signal exists.

## Respect the reply protocol

`ask` creates a random question ID and appends the required `tmact reply`
command to the dispatched prompt. The answering agent must call that command
exactly once. Do not manually invent a question ID, inject a reply into the
asker's pane, add shell polling, or infer an answer from pane activity.

A successful JSON report has `status: "answered"` and an explicit `reply`.
Return that reply to the user with any necessary caveats. Timeout, interrupt,
dispatch failure, pane disappearance, and `needs_human` are terminal blockers;
report the exact failure instead of fabricating or extracting a substitute
answer from terminal output.

When acting as the answering agent, follow the injected command exactly:

```bash
tmact reply QUESTION_ID --text "FINAL ANSWER"
```

Use `--file PATH` instead of `--text` only when the final answer already exists
as a file. Run `tmact reply` exactly once before giving the normal final chat
response; merely mentioning the command does not satisfy the protocol.

## Safety

- Preserve dry-run as the default. Add `--execute` only after the plan is
  correct or the user authorized the exact action.
- Do not interrupt busy agents, hijack a session running a different agent, or
  bypass permission and approval prompts.
- Never broaden `--trust-folder` beyond its exact-directory contract.
- Confirm session, directory, agent, model, prompt, and timeout when ambiguous.
- Stop after repeated CLI or tmux failures instead of retrying blindly.
