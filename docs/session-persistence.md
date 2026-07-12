# Tmux Session Persistence

`tmact statusd` provides built-in tmux layout and working-directory
persistence. It is designed to replace tmux-resurrect and tmux-continuum for
unattended statusd hosts without inheriting their empty-save failure mode.

## Defaults

Normal `tmact statusd start` processes:

- save all local tmux sessions every 5 minutes;
- keep 10 historical versions under `~/.tmact/tmux-sessions`;
- maintain `~/.tmact/tmux-sessions/last` as the filename of the last valid
  snapshot;
- attempt restore once at daemon startup, and only after tmux was queried
  successfully and reported zero sessions.

`tmact statusd once` never saves or restores sessions. Session include/exclude
filters only affect the pane-status view; persistence always captures every
local session and never includes federated peers.

Configure the behavior in `~/.tmact/statusd.json`:

```json
{
  "session_save": true,
  "session_restore": true,
  "session_save_interval": "5m",
  "session_snapshot_retention": 10,
  "session_snapshot_dir": "/Users/example/.tmact/tmux-sessions"
}
```

`session_snapshot_dir` must be absolute when set. An absent or empty value uses
the default under the current user's home directory. The equivalent daemon
flags are available from `tmact help statusd start`.

## Valid-save guarantee

A snapshot is published only after tmux capture and structural validation both
succeed and at least one session, window, and pane exist. The JSON snapshot is
written and synced through a temporary file, then renamed atomically. Only
after that succeeds is `last` replaced atomically. Retention runs last.

Zero sessions, a missing tmux server, malformed tmux output, invalid layout
data, or any write failure leaves the existing `last` untouched. If `last` is
missing or corrupt during restore, statusd scans historical files newest first,
uses the first valid non-empty version, and repairs the pointer when possible.

## Restore and safety boundary

Restore recreates session, window, and pane structure, assigns each pane its
saved cwd, reapplies the tmux window layout, and selects the previously active
pane and window. Missing cwd directories fall back to the user's home directory
and are logged.

Snapshot files contain no command line or process-replay field. Restore starts
only tmux's default shell and never restarts an AI agent or any other saved
program. Saved values are passed to tmux as argv, not evaluated by a shell.
Names, indexes, cwd paths, counts, and layout strings are validated before the
first tmux mutation. A failed restore removes the sessions created by that
attempt on a best-effort basis so it does not intentionally leave a partial
layout.

Restore is startup-only. If sessions are deleted while statusd continues
running, they are not recreated immediately. This preserves intentional live
operator actions; the last non-empty snapshot remains available for the next
daemon startup.

## Migrating from tmux-resurrect and tmux-continuum

Do not disable the old mechanism until statusd has run for at least one save
interval and `~/.tmact/tmux-sessions/last` points to a non-empty JSON snapshot.
Then, on the current macOS setup:

1. Keep the installed `com.tmact.statusd` LaunchAgent enabled so statusd runs
   at login and performs the startup restore itself.
2. Disable and remove the separate `com.puni.tmux-continuum-save` LaunchAgent.
   Its `continuum-save-agent.sh` timer is replaced by statusd's save interval.
3. Disable and remove the separate `com.puni.tmux-server` LaunchAgent. Its
   `tmux-boot-restore.sh` and `tmux-headless-restore.sh` paths are replaced by
   statusd's headless-safe startup restore.
4. Remove the tmux-resurrect/tmux-continuum plugin entries,
   `@continuum-restore`, and the `continuum_save_timed.sh` status-format hook
   from `~/.tmux.conf`.
5. `exit-empty off` is no longer required for boot restore and may be removed
   if the normal tmux empty-server behavior is preferred.

The old resurrect snapshots can be retained during rollout; statusd neither
reads nor modifies them. Rollback is therefore possible until those files and
plugins are removed manually.
