# tmact

`tmact` is a local tmux automation tool for running and supervising AI agents
in terminal panes. It can inspect panes, detect when an agent is idle or asking
for input, send guarded input, create agent panes, run repeatable loops, and
publish a live pane-status snapshot.

## CLI

Use the CLI when you want scriptable tmux control:

- `tmact ls` lists panes and creates numbered targets like `-t 0`.
- `tmact inspect --all` classifies panes by runtime and idle/running/asking
  state.
- `tmact -t 0 send --text "status?" --enter` previews input; add `--execute`
  to actually send it.
- `tmact detect --target session:0.0 --json` detects directory-access prompts.
- `tmact status`, `inbox`, `summarize`, `broadcast`, and `panels` work from an
  agent YAML config.
- `tmact loop` validates, starts, observes, pauses, resumes, restarts, and stops
  detached pane-automation loops; `tmact watch` runs narrow allowlisted prompt
  automation.
- `tmact dispatch-work` creates or reuses a tmux session, launches an agent CLI,
  sends it a prompt, and can explicitly handle exact-directory Claude/Codex
  workspace trust.
- `tmact statusd` maintains the cached pane snapshot used by status lines and
  the web UI.
- `tmact hook init zsh|bash|fish` prints an opt-in shell snippet whose
  preexec/precmd hooks emit structured events (via `tmact hook emit`) to the
  local statusd, sharpening its running/idle classification. tmact never edits
  shell rc files — source the snippet yourself, e.g.
  `eval "$(tmact hook init zsh)"` in `~/.zshrc`. Panes without hook events
  keep the capture-based heuristics. `tmact hook doctor` diagnoses the pipeline
  (tmux, socket, daemon reachability, per-pane emits) and `tmact hook state`
  dumps the daemon's recorded per-pane command state; both are read-only and
  reach the daemon over the local IPC socket only.
- `tmact commands --json` exposes command metadata for tooling, and
  `tmact llm instructions` prints an LLM-facing operating guide.

### Managed loop lifecycle

Use tmact itself as the loop supervisor. Do not wrap loops in `nohup`, shell
backgrounding, PID files, `while` loops, or hand-written tmux sessions.

```sh
# 1. Generate a complete template, edit target/prompt, then validate it.
tmact loop example --quota > loop.yaml
tmact loop validate --config loop.yaml
tmact loop run --config loop.yaml --dry-run --once

# 2. Start an idempotent detached runtime in the tmact-loops tmux session.
tmact loop start --config loop.yaml

# 3. Observe or control it from any shell using the same --run-dir.
tmact loop status --json
tmact loop logs --config loop.yaml --follow
tmact loop pause --config loop.yaml
tmact loop resume --config loop.yaml
tmact loop restart --config loop.yaml

# 4. Request a cooperative stop and wait for final state.
tmact loop stop --config loop.yaml --wait
```

`loop start` validates the config before launching, creates or reuses the
detached `tmact-loops` session, waits for runtime registration, and returns an
existing active runtime instead of starting a duplicate. `loop run` is the
foreground/debugging form; the legacy `tmact loop --config ...` syntax remains
an alias for it. Permission and approval prompts are never auto-confirmed.

For machine-readable flags, safety notes, and the exact lifecycle contract,
use `tmact help loop --json` or `tmact llm instructions --json`.

### Exact-directory workspace trust

Workspace trust is opt-in and limited to Claude/Codex. `dispatch-work` can
handle the startup prompt while it waits for the agent:

```sh
tmact dispatch-work work --dir ~/work/repo --agent codex \
  --prompt "run the tests" --trust-folder --execute
```

For a pane created by another tool, inspect first (dry-run), then execute:

```sh
tmact trust-folder --target work:0.0 --dir ~/work/repo --agent claude
tmact trust-folder --target work:0.0 --dir ~/work/repo --agent claude --execute
```

For configured panels, set `trust_folder: true` on an individual Claude/Codex
agent or pass `panels ensure --trust-folders --execute`. tmact accepts only a
recognized trust prompt when the detected runtime matches and the canonical
pane cwd is exactly the allowed directory. Parent directories, child
directories, ambiguous choices, and every other permission prompt are refused.

Install the release binary (macOS or Linux/WSL, amd64/arm64):

```sh
curl -fsSL https://raw.githubusercontent.com/leolin310148/tmact/main/scripts/install-release.sh | sh
```

On WSL, statusd auto-start requires `systemd` (set `systemd=true` in
`/etc/wsl.conf` and `wsl --shutdown`); without it the binary still installs
fine and you can launch statusd manually with `tmact statusd start &`.

## Web Interface

`statusd` can serve a browser UI for local pane monitoring and input. It shows
tmux sessions and panes, streams the selected pane, sends text or keys back to
that pane, offers quick buttons for common prompts, uploads files or clipboard
images and pastes their saved paths, and can use configured speech-to-text for
voice input.

Start it with the service installer (LaunchAgent on macOS, systemd `--user`
unit on Linux/WSL):

```sh
curl -fsSL https://raw.githubusercontent.com/leolin310148/tmact/main/scripts/install-release.sh | env TMACT_INSTALL_STATUSD=1 sh
```

Then open:

```text
http://127.0.0.1:7890
```

The web UI can send input to tmux panes, so it binds to `127.0.0.1` by default.
Only bind it to `0.0.0.0` on a trusted network.

### PWA Web Push

The web UI ships a same-origin PWA Service Worker at `/sw.js`. Web Push
notifications are shown by that worker, and notification clicks focus an
existing same-origin `vibe.puni.tw` window or open `/`, so installed PWA clients
are woken by the vibe origin itself.

Generate a VAPID key pair for this app, separate from any other notification
service:

```sh
npx web-push generate-vapid-keys
```

Configure the public key and subject in `~/.tmact/statusd.json` or with
environment variables; keep the private key out of git:

```json
{
  "webpush_vapid_public_key": "BASE64URL_PUBLIC_KEY",
  "webpush_vapid_subject": "mailto:you@example.com"
}
```

```sh
export TMACT_WEBPUSH_VAPID_PRIVATE_KEY=BASE64URL_PRIVATE_KEY
export TMACT_WEBPUSH_VAPID_PUBLIC_KEY=BASE64URL_PUBLIC_KEY
export TMACT_WEBPUSH_VAPID_SUBJECT=mailto:you@example.com
```

Subscriptions are stored by endpoint in
`~/.tmact/webpush_subscriptions.json` by default. Override with
`webpush_subscriptions_path` or `TMACT_WEBPUSH_SUBSCRIPTIONS_PATH`.

After subscribing in the web UI settings panel, send a test push:

```sh
curl -sS http://127.0.0.1:7890/api/push \
  -H 'Content-Type: application/json' \
  -d '{"title":"tmact","body":"hello from vibe","url":"/","tag":"tmact-test"}'
```

`POST /api/push` returns `{ "sent": N, "failed": N, "total": N }` and removes
subscriptions whose push endpoint returns 404 or 410. On iOS, install the site
to the Home Screen first; iOS Web Push is only delivered to installed PWAs.

Notifications can deep-link to a tmux pane by pane id. Prefer sending the raw
tmux pane id in `paneId`:

```sh
curl -sS http://127.0.0.1:7890/api/push \
  -H 'Content-Type: application/json' \
  -d '{"title":"tmact","body":"done","paneId":"%60","url":"/?pane=%2560","tag":"tmact-%60","session_id":"1","cwd":"tmact"}'
```

`paneId` is the raw tmux pane id (`%60`). If you only provide `url`, encode the
percent sign in the query (`/?pane=%2560`). `session_id` and `cwd` are accepted
as optional metadata for sender hooks. Sender hooks may keep using per-pane tags
like `claude-%60`; the server sends the Web Push `Topic` header as
`claude-pane-60` so iOS/APNs can collapse same-pane notifications at the system
layer. The Service Worker also normalizes pane tags to the same safe internal
form and closes existing same-pane notifications before showing the latest one
as a desktop/Android fallback. On notification click, the Service Worker focuses
an existing vibe PWA window and posts `SELECT_PANE`; if no window is open, it
opens `/?pane=...` so the frontend selects the pane on boot.

### Federation (multi-host)

statusd can pull snapshots from other statusd instances and merge them into
its own `/api/snapshot`, so one web UI can list tmux sessions from several
machines. Add a `peers` array to `~/.tmact/statusd.json`:

```json
{
  "web_addr": "0.0.0.0:7890",
  "peers": [
    { "name": "peer-a", "url": "http://peer-a.example:7890" }
  ],
  "peer_interval": "1s",
  "peer_timeout": "2s"
}
```

Remote sessions and panes appear with a `<name>@` prefix on their ids (e.g.
`peer-a@probe`, pane id `peer-a@%0`) and carry a `"peer": "peer-a"` field. Selecting a
remote pane in the web UI proxies its live stream, text/key input, uploads, and
image previews through that peer's statusd. If a peer goes unreachable, its
last successful snapshot stays visible as stale while the fetch error is
reported in `/api/snapshot`.

The CLI can also send to peer panes through the coordinator statusd config:

```sh
tmact -t peer-a@%0 send --text "status?" --enter --execute
tmact -t %0 send --peer peer-a --command "go test ./..." --execute
```

Loops can target a peer pane from the coordinator too:

```yaml
peer: peer-a
target: "%0"
# statusd_config: ~/.tmact/statusd.json  # optional; this is the default
actions:
  - type: send_text
    text: "continue"
    enter: true
```

Remote send and loop targets must use a canonical pane id like `%0`; the peer
statusd receives the request and acts on its local tmux pane.

Remote dispatch is configured separately from snapshot federation. Use
`dispatch_peers` when a machine should be able to call another machine's
statusd without also pulling that machine's snapshot:

```sh
tmact dispatch-work work --peer peer-a --dir /repo --agent codex --prompt "run the tests" --execute
```

```json
{
  "dispatch_peers": [
    { "name": "peer-a", "url": "http://peer-a.example:7890" }
  ]
}
```

For example, a peer that should dispatch back to the hub without pulling hub
panes can configure:

```json
{
  "dispatch_peers": [
    { "name": "hub", "url": "http://hub.example:7890" }
  ]
}
```

Then run:

```sh
tmact dispatch-work work --peer hub --dir /repo --agent codex --prompt "run the tests" --execute
```

The caller reads `dispatch_peers` from `~/.tmact/statusd.json`; the target
machine validates `--dir` on its own filesystem and creates or reuses the tmux
session locally. For compatibility, `dispatch-work --peer` also falls back to
`peers` when no matching `dispatch_peers` entry exists.

## Safety

Most commands preview actions first and require `--execute` before pressing
keys. Watchers keep allowlist checks, and loops stop on known permission,
approval, trust-folder, and confirmation prompts instead of auto-confirming
them. The only exception is explicit exact-directory workspace trust through
`dispatch-work --trust-folder`, `panels ensure --trust-folders`, an agent's
`trust_folder: true`, or the dry-run-first `tmact trust-folder` command.

A loop can also back off on quota: an optional `quota` block (see
[`examples/quota-aware-loop.yaml`](examples/quota-aware-loop.yaml)) reads the
target agent's real rate-limit usage. `session_min_remaining_percent: 20`
requires the 5-hour/session window to have strictly more than 20% remaining;
20% exactly is skipped. `weekly_require_headroom: true` requires positive
weekly headroom, meaning expected linear usage is greater than actual usage and
there is conserved weekly allowance available. When both are configured, both
must pass before the cycle runs. `weekly_skip_at_percent` remains available as
an absolute weekly ceiling. Quota checks fail open (keep running) when data or
pace cannot be read unless `fail_closed: true` is configured.

For source builds, tests, examples, and release notes, see
[`docs/development.md`](docs/development.md).
