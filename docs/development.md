# Development

This page is for people working from the source tree. The user-facing install
and usage path lives in `README.md`.

## Prerequisites

- Go 1.26 or newer
- `tmux` on `PATH`
- Optional agent CLIs when testing panel launchers: `codex`, `claude`,
  `copilot`, or `gemini`

## First Check

```sh
git clone https://github.com/leolin310148/tmact.git
cd tmact
go test ./...
go build -o .cache/tmact ./cmd/tmact
.cache/tmact version
```

Use read-only commands first:

```sh
.cache/tmact ls
.cache/tmact inspect --all
```

Commands that send input to tmux panes should stay dry-run until the printed
plan is correct.

## Repo Layout

```text
cmd/tmact/main.go             # CLI entrypoint; subcommand dispatch and flags
internal/tmux/                # tmux command wrappers
internal/prompt/              # permission and question prompt detection
internal/panestate/           # pane runtime + idle/running/asking classifier
internal/panestatus/          # pane snapshot + status rollup
internal/statusd/             # status daemon and snapshot generation
internal/runmeta/             # metadata for long-running loops/workflows
internal/agents/              # agents.yaml config consumers
internal/loop/                # single-pane scheduled action loop
internal/workflow/            # OpenSpec workflow runner
internal/watch/               # allowlisted prompt watcher
internal/web/                 # statusd web UI server and static assets
examples/                     # sample YAML configs
docs/                         # release and smoke-test notes
launchd/                      # macOS LaunchAgent template
```

## Useful Commands

```sh
go test ./...
go build -o .cache/tmact ./cmd/tmact
.cache/tmact loop --config examples/night-loop.yaml --dry-run --once
.cache/tmact watch --config examples/accept-question-watch.yaml --dry-run --once
```

When changing examples, keep them parseable and runnable with `--dry-run
--once`.

## Status Daemon

Local source installs can build and refresh the macOS LaunchAgent:

```sh
scripts/install.sh
```

Install only the binary:

```sh
scripts/install.sh --bin-only
```

The generated LaunchAgent runs the installed binary from `~/.local/bin/tmact`.

## Contribution Notes

See `AGENTS.md` for repository conventions, safety rules, and PR expectations.
