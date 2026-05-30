# Smoke Test Notes

Use this file for reproducible checks that are safe to share. Keep live pane
names, private repository paths, and personal machine details out of committed
notes.

## Suggested Checks

Build the local binary:

```sh
go build -o .cache/tmact ./cmd/tmact
```

Run unit tests:

```sh
go test ./...
```

List panes and refresh the target cache:

```sh
.cache/tmact ls
```

Run dry-run config checks:

```sh
.cache/tmact loop --config examples/night-loop.yaml --dry-run --once
.cache/tmact loop --config examples/maintenance-loop.yaml --dry-run --once --assume-idle-on-start
.cache/tmact watch --config examples/accept-question-watch.yaml --dry-run --once
.cache/tmact workflow discuss --config examples/openspec-workflow.yaml --dry-run --once
.cache/tmact workflow implement --config examples/openspec-implementation.yaml --dry-run --once
```

## statusd web UI (manual / browser)

The React UI's layout-dependent behavior is not unit-testable (jsdom has no
layout engine — `scrollHeight`/`clientHeight` are 0), so verify these in a real
browser. Build the UI first (`make web`), then run statusd with a web address:

```sh
.cache/tmact statusd start --web-addr 127.0.0.1:7890
```

Short-pane no-scroll + bottom bars pinned (most telling at a narrow ~390 px
viewport; install as a PWA to exercise real safe-area insets):

1. Select a pane idling at a shell prompt (a few real lines; tmux pads the rest
   of the grid with blank rows).
2. Expect: `#content` does NOT scroll (`scrollHeight <= clientHeight`), output is
   top-aligned, and `nav.statusline` + `.input-bar` stay visible (they do not
   overflow the `overflow:hidden` body).

Long-pane stick-to-bottom:

3. Select a pane with 400+ lines of output.
4. Expect: `#content` scrolls and stays pinned to the bottom
   (`scrollTop + clientHeight ≈ scrollHeight`), newest line visible.

Boot placeholder (fresh load, no saved selection): on a narrow viewport the
`#draft` placeholder reads "Type a prompt, then tap Send" (not the desktop
⌘/Ctrl hint) and the mode strip shows "Select a pane to enable input".

## Notes Template

```text
Date:
Command:
Target:
Result:
Follow-up:
```
