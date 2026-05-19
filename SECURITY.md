# Security

`tmact` presses keys in live tmux panes and can expose a browser UI for pane
inspection and input. Treat it as a local automation tool, not a hardened
network service.

## Supported Use

- Run it on machines and networks you trust.
- Keep the statusd web UI bound to `127.0.0.1:7890` unless you intentionally
  need LAN access.
- Use explicit tmux targets and dry-runs before `--execute`.
- Keep watcher allowlists narrow.

## Reporting

For now, report issues directly to the repository owner or in the internal
issue tracker. Include the command, config file, expected behavior, observed
behavior, and whether a live tmux pane was modified.
