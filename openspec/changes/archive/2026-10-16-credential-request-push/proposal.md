# Proposal: Credential Request Push

## Intent

Let an agent ask the human for a credential it does not have, deliver that ask
as a web push notification, collect the value in the tmact web UI, and store it
under `~/.tmact/.cred/` where the agent can use it without the value passing
through the agent's context.

## User

The user drives Codex and Claude panes on studio and its peers (mini, nb429,
mbp26), mostly from a phone through the statusd web UI. Agents regularly stall
on "I need a token for X". Today the user has to come back to a terminal, find
the right machine, and write the secret into a file by hand.

## Problem

- There is no tmact channel for an agent to ask for a secret; agents either
  stop and wait, or ask in the pane and the user pastes the value into the
  pane, which puts it in the agent transcript, the tmux scrollback, and every
  surface that reads pane text (statusd snapshot, `/ws/pane`,
  `/api/pane/capture`).
- tmact has no credential store, so each agent invents its own location.
- The only push sender today is the external `notify.sh` hook; statusd never
  sends a push itself.

## Scope

In scope:

- `internal/credstore`: named files under `~/.tmact/.cred/` (dir 0700, file
  0600), atomic write, list, remove, owner and symlink checks.
- statusd credential requests: created over the unix socket only, held in
  memory, described and answered over HTTP, one-shot.
- Push from statusd for pending requests, including requests that originate on
  a peer.
- Peer support: the hub that holds push subscriptions observes peer requests
  through the existing snapshot poll and forwards the submit to the peer.
- Web UI dialog opened from the notification or from a pending-request badge.
- CLI `tmact cred request|path|ls|rm|run`.
- Skill `skills/tmact-cred` that tells agents how to request and use a
  credential without printing it.

Out of scope:

- Authentication on statusd endpoints (the LAN/tailnet trust model stands).
- macOS Keychain or any encrypted store; plain files were chosen.
- Excluding `~/.tmact/.cred/` from Time Machine; the user chose to keep it in
  backups.
- Fixing the Jev permission hook prefix-match gap (lives in dotfiles; tracked
  separately).
- Credential rotation, expiry, or sharing one value across machines.

## Proposed User Flow

1. An agent on mini runs
   `tmact cred request github-pat --reason "push the release tag"`.
2. mini's statusd records a pending request with a random 128-bit id and the
   requesting pane, cwd, and host; the CLI blocks on the socket.
3. studio's peer poll sees the request in mini's snapshot and sends one push:
   "mini needs github-pat — push the release tag", linking to
   `/?cred=mini@<id>`.
4. The user taps it; the web UI shows the name, reason, machine, pane, and cwd,
   with a password field, Submit, and Decline.
5. Submit goes to studio, which forwards it to mini; mini writes
   `~/.tmact/.cred/github-pat` atomically and resolves the request.
6. The CLI on mini prints only the path and exits 0. The agent then runs
   `tmact cred run --env GITHUB_TOKEN=github-pat -- git push ...`.

## Decisions

- **No read-back.** No HTTP or socket endpoint returns a stored value. The
  browser can only write.
- **No secrets in pushes.** A push carries the request id, name, reason, and
  location only.
- **Create is socket-only.** `POST` to create a request is rejected on TCP,
  like `/api/hook-event`, so only a local process can start a push.
- **Peers are supported by forwarding**, reusing `?peer=` proxying. The peer
  writes the file on its own disk; the value never lands on the hub's disk.
- **Plain files**, kept in Time Machine backups.
- **CLI never prints a value**; `cred run` is the primary way to use one.

## Success Criteria

- An agent on studio or on a peer can request a credential and receive a path
  after the user submits from a phone, with exactly one notification.
- No endpoint, log line, snapshot, or CLI output contains a submitted value
  (tested).
- A request id can be answered once; a second submit, a submit after timeout,
  or a submit after statusd restart fails cleanly and the CLI reports it.
- An existing credential short-circuits `cred request` without a push unless
  `--refresh` is passed.
