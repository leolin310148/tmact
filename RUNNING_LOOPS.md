# Running Loops

Background automation should run in a detached tmux session such as
`tmact-loops`. Do not start loop daemons in the same session/window where you
are actively developing, because the foreground process will block that pane.

## Active Tasks

No project-specific loops are tracked in this shared repository.

For a local machine, keep any live inventory in an untracked note or in your
own operations docs. Include the loop config, target pane, start time, expected
stop time, and stop command.

## Useful Commands

Check running background windows:

```sh
tmux list-windows -t tmact-loops -F '#{window_index} #{window_name} #{pane_current_command} dead=#{pane_dead} status=#{pane_dead_status}'
```

Stop a specific daemon:

```sh
tmux send-keys -t tmact-loops:0.0 C-c
```

Inspect target panes:

```sh
tmux capture-pane -pt TARGET -S -80
```
