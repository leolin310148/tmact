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

## Notes Template

```text
Date:
Command:
Target:
Result:
Follow-up:
```
