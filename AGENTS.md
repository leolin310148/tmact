# Repository Guidelines

## Project Structure & Module Organization

`tmact` is a Go CLI for local tmux automation. The entrypoint is
`cmd/tmact/main.go`; subcommand parsing uses the stdlib `flag` package and
delegates to packages under `internal/`. Notable packages:

- `internal/tmux` — tmux command wrappers (list, capture-pane, send-keys, options).
- `internal/prompt` — directory-access prompt detection.
- `internal/panestate` / `internal/panestatus` — runtime + idle/running/asking classification and rollups.
- `internal/statusd` — long-running status daemon that publishes a JSON snapshot.
- `internal/loop` — single-pane scheduled action loop.
- `internal/watch` — narrow prompt watcher (allowlisted answerer).
- `internal/agents` — `agents.yaml` config used by panels/broadcast/status/inbox/summarize.
- `internal/runmeta` — `.tmact/runs/` metadata for inspect/stop of long-running processes.

Example YAML configs live in `examples/`, operational notes in `docs/` and
`RUNNING_LOOPS.md`, and the macOS launchd template for `statusd` in `launchd/`.

## Build, Test, and Development Commands

- `go test ./...` — run all unit tests (config parsing, prompt detection, classifiers, runners).
- `go build -o .cache/tmact ./cmd/tmact` — build the local binary used by smoke tests and long-running loops.
- `scripts/install.sh` — build into `~/.local/bin/tmact` and refresh the `statusd` launchd agent so it runs the installed binary (`--bin-only` skips the agent).
- `go run ./cmd/tmact ls` — list tmux panes and refresh the numbered-target cache.
- `go run ./cmd/tmact detect --target session:0.0 --json` — capture a pane and detect a directory-access prompt.
- `go run ./cmd/tmact inspect --all --json` — classify runtime + idle state for every pane.
- `go run ./cmd/tmact loop --config examples/night-loop.yaml --dry-run --once` — validate one loop pass without sending keys.
- `go run ./cmd/tmact watch --config examples/accept-question-watch.yaml --dry-run --once` — validate one watcher pass.

External dependencies are `gopkg.in/yaml.v3` for YAML config and
`github.com/coder/websocket` for the statusd web UI (Go 1.26). No SQLite,
cobra, or gocron despite earlier design notes — keep new dependencies minimal.

## Coding Style & Naming Conventions

Use standard Go formatting: run `gofmt` on edited Go files before committing.
Keep package names short and lowercase (`loop`, `watch`, `prompt`,
`panestate`). Prefer table-driven or focused `TestXxx` tests. Config field
names should match the YAML keys already used in `examples/` (`idle_after`,
`post_delay`, `allow_path_patterns`, `clear_before_prompt`). Keep tmux side
effects isolated behind `internal/tmux` helpers; the rest of the code should be
testable without a live tmux session.

## Testing Guidelines

Place tests next to the package they cover using `_test.go`. Tests should
exercise validation, defaults, and safety decisions without requiring tmux.
For tmux-facing behavior, prefer `--dry-run --once` smoke checks and record
notable manual findings in `docs/smoke-test.md`. When adding or changing an
example config, include a loader test so it stays parseable.

## Commit & Pull Request Guidelines

The history uses concise imperative commit subjects (e.g. `Add prompt watcher
automation`, `Improve pane status detection`). Follow that style and keep
each commit scoped to one behavior or documentation change. PRs should
describe the user-visible effect, list the validation commands run, and call
out any live tmux smoke testing. Include screenshots only when terminal output
or external UI behavior is relevant.

## Safety & Configuration Notes

This tool presses keys in live tmux panes. While developing:

- Default to dry-run; only add `--execute` once the printed plan is correct.
- Keep target panes explicit; do not broaden a config's target glob without
  the user asking for it.
- Preserve allowlist checks (`allow_paths`, `allow_path_patterns`) in the
  watcher; never add bypasses.
- Loops should stop on permission prompts rather than auto-confirming.
- Do not add actions that execute arbitrary shell commands or approve
  unbounded paths without clear local safety controls.
- Keep the statusd web UI on `127.0.0.1` by default. Binding it to `0.0.0.0`
  is a local operator decision for trusted networks because the UI can send
  input to tmux panes.

Long-running daemons belong in the detached `tmact-loops` tmux session — not
the working `tmact` session, which would block development. See
`RUNNING_LOOPS.md` for the local inventory template.
