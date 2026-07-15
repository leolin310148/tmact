# Running Loops

Start background automation with `tmact loop start --config PATH`. tmact owns
the detached `tmact-loops` session and runtime metadata; do not create the
session, window, PID file, or shell wrapper yourself.

## Active Tasks

No project-specific loops are tracked in this shared repository.

For a local machine, keep any live inventory in an untracked note or in your
own operations docs. Include the loop config, target pane, start time, expected
stop time, and stop command.

## Useful Commands

Check managed loop state:

```sh
tmact loop list
```

Follow events and stop cleanly:

```sh
tmact loop logs --config PATH --follow
tmact loop stop LOOP_ID
```

Inspect target panes:

```sh
tmux capture-pane -pt TARGET -S -80
```
