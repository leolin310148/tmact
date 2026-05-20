# Repository Guidelines

`tmact` is a Go CLI for local tmux automation, mainly for people running AI
agents in terminal panes. It can list and inspect panes, detect idle/running or
asking states, send guarded input, create/reuse agent sessions, run scheduled
loops, answer narrow allowlisted prompts, and publish pane status for a local
web UI.

## CLI

The entrypoint is `cmd/tmact/main.go`; most behavior lives under `internal/`.
Common commands:

- `tmact ls` lists tmux panes and caches numbered targets like `-t 0`.
- `tmact inspect --all` classifies panes by runtime and activity state.
- `tmact -t 0 send --text "status?" --enter` previews input; `--execute`
  actually sends it.
- `tmact detect --target session:0.0 --json` detects directory-access prompts.
- `tmact status`, `inbox`, `summarize`, `broadcast`, and `panels` use an agent
  YAML config.
- `tmact loop` and `tmact watch` run pane automation with dry-run support.
- `tmact dispatch-work` starts or reuses a tmux session, launches an agent CLI,
  and sends it a prompt.
- `tmact statusd` maintains the cached pane snapshot used by status lines and
  the browser UI.

## Web Interface

`statusd` can serve a local browser UI, bound to `127.0.0.1` by default. The UI
shows sessions and panes, streams selected pane output, sends text or keys back
to panes, offers quick prompt buttons, uploads files or clipboard images and
pastes their saved paths, and can use configured speech-to-text for voice input.

## Development

- Run tests with `go test ./...`.
- Build with `go build -o .cache/tmact ./cmd/tmact`.
- Run local commands with `go run ./cmd/tmact ...`.
- Use `gofmt` on edited Go files.
- Keep dependencies minimal; current external deps are YAML config and the web
  socket package.

## Safety

This tool presses keys in live tmux panes. Default to dry-runs, keep targets
explicit, preserve watcher allowlists, and do not auto-confirm permission,
approval, trust-folder, or broad path prompts. Keep the web UI on `127.0.0.1`
unless the operator explicitly chooses a trusted-network bind.
