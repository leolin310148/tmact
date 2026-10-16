---
name: tmact-cred
description: Get a secret (API token, password, PAT, key) from the user through a tmact web push and use it without the value ever entering agent context or pane output, via `tmact cred request` and `tmact cred run`. Use when a command needs a credential that is not already available, when an auth step fails for lack of a token, or when the user says to ask them for one. Trigger on "need a token", "ask me for the password", "credential", "API key", "PAT", "secret", "要 token", "跟我要密碼", "輸入 credential", "推播給我輸入", and "tmact cred". Do not use to read, display, copy, or move an existing credential's value; that is never allowed.
---

# tmact-cred

`tmact cred` lets an agent ask the user for a named credential. The user gets a
phone push, opens the tmact web UI, and types the value; statusd stores it as a
0600 file under `~/.tmact/.cred/NAME`. The agent only ever receives the file
path, and uses the value through `tmact cred run`, which puts it into a child
process's environment.

## Hard rules

The value must never pass through your context or a terminal pane. Pane text is
captured by statusd and shown in the web UI and to peers.

- Never `cat`, `echo`, `head`, `less`, `printf`, `base64`, `xxd`, `grep`, `cp`,
  or otherwise read, print, copy, or transform a file under `~/.tmact/.cred/`.
- Never expand a credential into a command line (`curl -H "Authorization: $(cat …)"`,
  `TOKEN=$(cat …) cmd`), write it into another file, a config, a commit, a log,
  or a chat message, or pass it to another agent.
- Never ask the user to paste a secret into the chat or the pane; use
  `cred request` instead.
- If a tool can only take the secret on its command line or prints it back,
  stop and tell the user rather than working around these rules.

## Pre-flight

Honor active agent and repository instructions first, including any required
shell-command prefix.

```bash
tmact version
tmact cred ls
```

`cred ls` lists names and modification times only. If the needed name is
already listed, skip the request and go straight to `cred run`. If any flag or
exit code is uncertain, run `tmact help cred --json`; the installed CLI is
authoritative.

## Request a credential

```bash
tmact cred request NAME --reason "WHY, in a few words" [--timeout 10m] [--refresh] [--json]
```

- `NAME`: 1-64 characters of letters, digits, `.`, `_`, `-`, starting with a
  letter or digit. Pick a stable, descriptive name (`github-pat`,
  `npm-publish-token`) so later runs reuse it.
- `--reason` is shown in the notification; say what the credential is for.
  Never put a secret or a guess of one in it.
- If the credential is already stored, the command returns its path at once
  without a push. Use `--refresh` only when the stored value is known to be
  wrong or expired (for example an auth failure with it).
- It blocks until the user answers or the request expires (default 10m, max
  1h). Run it in the foreground; do not background or poll it.

On success it prints only the path. Exit codes:

| Exit | Meaning | What to do |
|---|---|---|
| 0 | Stored | Continue with `cred run`. |
| 3 | User declined | Stop that task step and report it; do not ask again unprompted. |
| 4 | Expired | Report it; ask the user whether to request again. |
| 1 | Error | Report the message (for example statusd not running). |

Requests go through the local statusd. On a peer machine the push is sent by
the hub (studio), so the peer must be reachable from it.

## Use a credential

```bash
tmact cred run --env VAR=NAME [--env VAR2=NAME2] -- COMMAND [ARGS...]
```

`cred run` execs `COMMAND` with each `VAR` set to the stored value and prints
nothing itself. Choose the variable the tool already reads:

```bash
tmact cred run --env GITHUB_TOKEN=github-pat -- gh release create v1.2.0
tmact cred run --env NPM_TOKEN=npm-publish-token -- npm publish
```

If the tool needs a file path instead (for example a key file), pass the path
from `tmact cred path NAME`; never read the file yourself. Make sure the
command does not echo its environment (`env`, `set -x`, verbose debug flags).

## Other commands

```bash
tmact cred path NAME   # path of a stored credential, never the value
tmact cred ls [--json] # names and modification times
tmact cred rm NAME     # delete a stored credential when the user asks
```

## CLI feedback

When a tmact command is confusing, lacks a needed flag, produces unparseable
output, or returns an unhelpful error, immediately record it with
`tmact feedback "<what was awkward and what you expected>" --category ux|bug|feature|docs|perf --command <cmd>`.
Never include a credential value or name-value pair in feedback.
