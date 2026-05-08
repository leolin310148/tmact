# Repository Guidelines

## Project Structure & Module Organization

`tmact` is a small Go CLI for local tmux automation. The entrypoint lives in
`cmd/tmact/main.go`; command parsing is intentionally simple and delegates to
packages under `internal/`. Core packages include `internal/loop` for scheduled
agent loops, `internal/watch` for prompt watchers, `internal/prompt` for prompt
detection, and `internal/tmux` for tmux command wrappers. Example YAML configs
are in `examples/`, and operational notes are in `docs/` and `RUNNING_LOOPS.md`.

## Build, Test, and Development Commands

- `go test ./...`: run all unit tests, including config parsing and prompt
  decision tests.
- `go run ./cmd/tmact detect --target session:0.0 --json`: capture a tmux pane
  and report detected directory-access prompts.
- `go run ./cmd/tmact loop --config examples/night-loop.yaml --dry-run --once`:
  validate one loop pass without sending keys to tmux.
- `go run ./cmd/tmact watch --config examples/accept-question-watch.yaml --dry-run --once`:
  validate watcher decisions without pressing keys.
- `go build -o .cache/tmact ./cmd/tmact`: build a local binary used by smoke
  tests and long-running loops.

## Coding Style & Naming Conventions

Use standard Go formatting: run `gofmt` on edited Go files before committing.
Keep package names short and lowercase (`loop`, `watch`, `prompt`). Prefer
table-driven or focused `TestXxx` tests and descriptive config field names that
match YAML keys already used in `examples/`, such as `idle_after`,
`post_delay`, and `allow_path_patterns`. Keep tmux side effects isolated behind
`internal/tmux` helpers when possible.

## Testing Guidelines

Place tests next to the package they cover using `_test.go` files. Tests should
exercise validation, defaults, and safety decisions without requiring a live tmux
session. For tmux-facing behavior, prefer `--dry-run --once` smoke checks and
record notable manual findings in `docs/smoke-test.md`. When adding or changing
example configs, include a test that loads them successfully.

## Commit & Pull Request Guidelines

The existing history uses concise imperative commit subjects, for example
`Add prompt watcher automation`. Follow that style and keep each commit scoped
to one behavior or documentation change. Pull requests should describe the user
visible effect, list validation commands run, and call out any live tmux smoke
testing. Include screenshots only when terminal output or external UI behavior
is relevant.

## Safety & Configuration Notes

This tool can press keys in live tmux panes. Default to dry-run commands while
developing, keep target panes explicit, and preserve allowlist checks for prompt
acceptance. Do not add actions that execute arbitrary shell commands or approve
unbounded paths without clear local safety controls.
