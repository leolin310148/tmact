# tmact

`tmact` is a local tmux automation tool for people who run AI agents in
terminal panes.

It helps you see what each pane is doing, send prompts safely, run repeated
actions, and keep a lightweight status snapshot for your tmux status line or a
local browser UI.

## Install

Prerequisites:

- macOS
- `tmux` on `PATH`
- Optional agent CLIs such as `codex`, `claude`, `copilot`, or `gemini`

Install the latest release:

```sh
curl -fsSL https://raw.githubusercontent.com/leolin310148/tmact/main/scripts/install-release.sh | sh
```

The installer puts `tmact` in `~/.local/bin`. Make sure that directory is on
your `PATH`, then check the install:

```sh
tmact version
tmact ls
```

## Status Daemon And Web UI

`statusd` keeps a cached snapshot of tmux panes and can serve a local browser
UI. The UI can send input to panes, so it binds to `127.0.0.1` by default.

Install or update the release binary and load the macOS LaunchAgent:

```sh
curl -fsSL https://raw.githubusercontent.com/leolin310148/tmact/main/scripts/install-release.sh | env TMACT_INSTALL_STATUSD=1 sh
```

The web UI listens on:

```text
http://127.0.0.1:7890
```

Check the LaunchAgent:

```sh
launchctl print "gui/$(id -u)/com.tmact.statusd"
```

`statusd` reads `~/.tmact/statusd.json` itself and seeds defaults on first
start. Recognised keys: `web_addr`, `interval`, `state_path`, `log_path`,
`tmux_options`. CLI flags still win over the file.

Expose the web UI on a trusted LAN only when you mean to. Edit the bind:

```sh
mkdir -p ~/.tmact
printf '{\n  "web_addr": "0.0.0.0:7890"\n}\n' > ~/.tmact/statusd.json
launchctl kickstart -k "gui/$(id -u)/com.tmact.statusd"
```

## First Use

Start with read-only commands:

```sh
tmact ls
tmact inspect --all
```

`tmact ls` lists panes and caches numbered targets, so follow-up commands can
use `-t 0`, `-t 1`, and so on.

Sending input is a dry run by default:

```sh
tmact -t 0 send --text "summarize current progress" --enter
```

If the printed plan is correct, add `--execute`:

```sh
tmact -t 0 send --text "summarize current progress" --enter --execute
```

## Common Tasks

List panes:

```sh
tmact ls
```

Inspect one pane:

```sh
tmact -t 0 inspect
```

Inspect every pane:

```sh
tmact inspect --all
```

Send text safely:

```sh
tmact -t 0 send --text "continue with the next small step" --enter
tmact -t 0 send --text "continue with the next small step" --enter --execute
```

Detect whether a pane is asking for directory access:

```sh
tmact -t 0 detect --json
```

## Update

Run the release installer again:

```sh
curl -fsSL https://raw.githubusercontent.com/leolin310148/tmact/main/scripts/install-release.sh | sh
```

If you use `statusd`, update the binary and restart the LaunchAgent:

```sh
curl -fsSL https://raw.githubusercontent.com/leolin310148/tmact/main/scripts/install-release.sh | env TMACT_INSTALL_STATUSD=1 sh
```

Install a specific version:

```sh
curl -fsSL https://raw.githubusercontent.com/leolin310148/tmact/main/scripts/install-release.sh | env TMACT_VERSION=v0.1.1 sh
```

## Uninstall

Remove the LaunchAgent if you installed `statusd`:

```sh
launchctl bootout "gui/$(id -u)/com.tmact.statusd" 2>/dev/null || true
rm -f "$HOME/Library/LaunchAgents/com.tmact.statusd.plist"
```

Remove the binary:

```sh
rm -f "$HOME/.local/bin/tmact"
```

Optional local state and logs:

```text
.cache/tmact-targets.json
.tmact/runs/
/tmp/tmact-status.json
/tmp/tmact-statusd.jsonl
/tmp/tmact-statusd.out.log
/tmp/tmact-statusd.err.log
```

## Configured Agents

Commands such as `status`, `inbox`, `summarize`, `broadcast`, and `panels` read
an agents config.

Start from the sample:

```sh
cp examples/agents.yaml tmact.agents.yaml
```

Edit names, roles, repos, launchers, and tmux targets, then validate it:

```sh
tmact status --config tmact.agents.yaml
tmact panels plan --config tmact.agents.yaml
```

Use `panels ensure --execute` only after the plan looks right.

## Loops And Watchers

Loops send repeated actions to a pane. Watchers answer narrow, allowlisted
prompts. Both should be tested with `--dry-run --once` before they touch a live
pane.

```sh
tmact loop --config examples/night-loop.yaml --dry-run --once
tmact watch --config examples/accept-question-watch.yaml --dry-run --once
```

The included examples use placeholder targets such as `sample-agent:0.0`.
Replace them with real tmux targets before running with `--execute`.

## Safety

- `send` is dry-run by default; `--execute` is required to press keys.
- Keep tmux targets explicit.
- Watchers only approve paths matched by `allow_paths` or
  `allow_path_patterns`.
- Loops stop on known permission, approval, trust-folder, and confirmation
  prompts instead of auto-confirming.
- Keep the web UI bound to `127.0.0.1` unless you trust the network.
- Treat captured terminal output as untrusted text.

## Commands

| Command | Purpose |
| --- | --- |
| `ls` | List tmux panes and cache numbered targets. |
| `send` | Send text, commands, or tmux keys. Dry-run by default. |
| `detect` | Detect directory-access permission prompts. |
| `inspect` | Classify pane runtime and idle/running/asking state. |
| `status` | Show configured agent pane status. |
| `inbox` | Show panes needing attention. |
| `summarize` | Summarize recent configured pane activity. |
| `statusd` | Maintain a cached pane snapshot and optional web UI. |
| `broadcast` | Send the same input to configured agent panes. |
| `panels` | Plan or create configured tmux panes. |
| `loop` | Run scheduled actions against one pane. |
| `watch` | Answer narrow allowlisted prompts. |
| `workflow` | Run serialized OpenSpec discussion/implementation workflows. |
| `dispatch-work` | Create/reuse a tmux session, launch an agent, and send a prompt. |
| `version` | Print build version information. |

## Development

For source builds, tests, and repository notes, see `docs/development.md`.
