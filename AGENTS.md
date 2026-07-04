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
  Loops accept an optional `quota` block that skips cycles when the target
  agent's weekly/session rate-limit usage is too high (see
  `examples/quota-aware-loop.yaml`).
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

The browser UI is a Vite + React + TypeScript app under `internal/web/frontend/`.
Its production build is emitted into `internal/web/static/` (embedded via
`go:embed`) and is **not committed** — `internal/web/static/` holds only
`.gitkeep` in git. Build it before building/testing the Go binary.

- Build everything: `make build` (runs the frontend build, then `go build ./...`).
- Build just the UI: `make web` (Vite → `internal/web/static/`). Requires Node
  (the repo was set up with Node 22 + npm 10).
- Run tests: `make test` (frontend Vitest + `go test ./...`). For Go-only after a
  build, `go test ./...` works; the build-dependent web tests `t.Skip` when the UI
  has not been built.
- Live UI dev loop: `make web-dev` runs the Vite dev server on `:5234` and proxies
  `/api` + `/ws` to a running statusd (default `127.0.0.1:7890`; override with
  `TMACT_STATUSD=host:port`). Lets you iterate on the UI without rebuilding the
  Go binary.
- Run local commands with `go run ./cmd/tmact ...` (build the UI first if you need
  the web server).
- Use `gofmt` on edited Go files; the frontend is TypeScript-strict (`make web`
  type-checks via `tsc` before bundling).
- Keep Go dependencies minimal; current external Go deps are YAML config and the
  web socket package. The frontend's npm deps live in `internal/web/frontend/`.

## Safety

This tool presses keys in live tmux panes. Default to dry-runs, keep targets
explicit, preserve watcher allowlists, and do not auto-confirm permission,
approval, trust-folder, or broad path prompts. Keep the web UI on `127.0.0.1`
unless the operator explicitly chooses a trusted-network bind.
