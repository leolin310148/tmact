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
- `tmact loop` and `tmact watch` run narrow automation against panes, with
  dry-run support.
- `tmact dispatch-work` creates or reuses a tmux session, launches an agent CLI,
  and sends it a prompt.
- `tmact statusd` maintains the cached pane snapshot used by status lines and
  the web UI.

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

### Federation (multi-host)

statusd can pull snapshots from other statusd instances and merge them into
its own `/api/snapshot`, so one web UI can list tmux sessions from several
machines. Add a `peers` array to `~/.tmact/statusd.json`:

```json
{
  "web_addr": "0.0.0.0:7890",
  "peers": [
    { "name": "z13", "url": "http://100.65.95.50:7890" }
  ],
  "peer_interval": "1s",
  "peer_timeout": "2s"
}
```

Remote sessions and panes appear with a `<name>@` prefix on their ids (e.g.
`z13@probe`, pane id `z13@%0`) and carry a `"peer": "z13"` field. Selecting a
remote pane in the web UI proxies its live stream, text/key input, uploads, and
image previews through that peer's statusd. If a peer goes unreachable, its
last successful snapshot stays visible as stale while the fetch error is
reported in `/api/snapshot`.

## Safety

Most commands preview actions first and require `--execute` before pressing
keys. Watchers keep allowlist checks, and loops stop on known permission,
approval, trust-folder, and confirmation prompts instead of auto-confirming
them.

For source builds, tests, examples, and release notes, see
[`docs/development.md`](docs/development.md).
