# Design: Credential Request Push

## Overview

```
agent ── unix socket ──▶ statusd (origin host)
                           │ pending request in memory
                           │ exposed in snapshot as metadata
hub statusd ◀── peer snapshot poll (existing, 1s) ──┘
   │ push (VAPID, existing subscriptions)
   ▼
phone ── /?cred=<peer@>id ──▶ hub web UI ── submit ──▶ origin statusd (?peer= proxy)
                                                         │ credstore write 0600
agent CLI ◀── long-poll resolves with path ──────────────┘
```

Today only studio holds VAPID keys and push subscriptions; mini and nb429 have
neither. A statusd pushes only when it has both, so peers never push on their
own and the hub pushes for everything it can see.

## Credential Store

`internal/credstore`:

- Root `~/.tmact/.cred/`, created 0700; ownership and non-symlink checks on the
  root and on each file before read or write (same pattern as
  `askreply.ensureOwnedPrivateDir`).
- Names match `^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$`; no path separators, no
  leading dot.
- Write: temp file in the same dir, 0600, fsync, rename. Value bytes are
  written as submitted (no trailing newline added); empty values are rejected.
- `Path`, `List` (name + mtime only), `Remove`, `Read` (used only by
  `cred run` in-process; never exposed over HTTP or socket).

## Request Lifecycle (origin statusd)

In memory only; a restart drops pending requests and the waiting CLI gets an
error it can retry.

| Field | Notes |
|---|---|
| `id` | `c_` + 26 base32 chars (128-bit, `crypto/rand`) |
| `name`, `reason` | reason capped at 200 chars |
| `pane_id`, `cwd`, `host` | from the CLI (`$TMUX_PANE`, cwd, hostname) |
| `created_at`, `expires_at` | default timeout 10m, max 1h |
| `state` | `pending` → `submitted` / `declined` / `expired` |

Endpoints (all JSON):

- `POST /api/cred/requests` — socket only (TCP → 403). Creates a request;
  one pending request per name (a second create joins the existing one).
- `GET /api/cred/requests/{id}/wait` — socket only; long-poll that returns the
  final state and, on submit, the file path.
- `GET /api/cred/requests/{id}` — metadata only, for the dialog.
- `POST /api/cred/requests/{id}/submit` — body `{"value": "..."}`; guarded by
  the `requireSessionMutationRequest` checks (Sec-Fetch-Site, Origin vs Host,
  JSON content type). One-shot: any state other than `pending` → 409.
- `POST /api/cred/requests/{id}/decline` — same guard, one-shot.

Pending requests appear in `Snapshot.CredRequests` as metadata. `MergePeers`
prefixes their ids with `peer@` the same way it prefixes pane ids.

Submit body size is capped (64 KiB). The handler never logs request bodies;
the frontend log path is not used by the dialog.

## Push

A notifier in statusd watches the merged snapshot. For each `CredRequest` id
it has not pushed yet, it sends one push through the existing `push.go`
sender with `tag = "cred-" + id` (unprefixed id, so duplicates collapse on the
device), `url = "/?cred=" + mergedID`, and title/body built from host, name,
and reason. Pushed ids are kept in memory until the request leaves the
snapshot; a hub restart may re-push a still-pending request, which is
acceptable.

## Peer Forwarding

The dialog calls `/api/cred/requests/{id}...` with `?peer=mini` when the merged
id has a peer prefix, reusing `maybeProxyPeerHTTP`. The hub runs the
`requireSessionMutationRequest` guard on the browser request before proxying.
The proxy copies the browser's `Sec-Fetch-Site: same-origin`, `Origin`, and
`Content-Type`, and the guard already lets an explicit `same-origin` pass
despite an Origin/Host mismatch, so the same guard also passes on the peer.
Socket-only routes are
never proxied.

## Web UI

- Boot reads `?cred=`; `sw.js` posts a new `OPEN_CRED` message when a window
  is already open (today it only posts `SELECT_PANE`, which does not change
  the URL).
- `CredentialDialog`: shows name, reason, host, pane, cwd, time left; password
  input with `autocomplete="off"`; Submit and Decline. Only the top-level
  window opens it (split-view slots are iframes).
- A small badge lists pending requests from the snapshot so a missed push can
  still be answered.

## CLI

- `tmact cred request NAME --reason TEXT [--timeout 10m] [--refresh] [--json]`
  — exits 0 with the path, 3 declined, 4 timeout/expired, 1 other errors. If
  the file exists and `--refresh` is not set, prints the path without a push.
- `tmact cred path NAME`, `tmact cred ls`, `tmact cred rm NAME`.
- `tmact cred run --env VAR=NAME [--env ...] -- CMD ...` — reads the files and
  execs the command with those variables set; prints nothing of the values.

## Safety

- Threat model: the user's own agents running as the same OS user, plus anyone
  on the LAN/tailnet who can reach port 7890.
- Same-user processes can read `~/.tmact/.cred/` by design; the protection is
  keeping values out of agent context and pane text. The skill forbids `cat`,
  `echo`, or printing credential files and points agents to `cred run`.
- A LAN client cannot create requests (socket-only) or read values (no
  endpoint). It can see pending request metadata in the snapshot and could
  submit a bogus value to a pending id; that is accepted under the existing
  LAN trust model. The request is one-shot, so a bogus submit shows up as the
  user's real submit failing with 409.
- Tests assert that a submitted marker value never appears in snapshot JSON,
  metadata responses, statusd logs, push payloads, or CLI output.
