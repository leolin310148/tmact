# Security

`tmact` presses keys in live tmux panes and can expose a browser UI for pane
inspection and input. Treat it as a local automation tool, not a hardened
network service.

## Supported Use

- Run it on machines and networks you trust.
- Keep the statusd web UI bound to `127.0.0.1:7890` unless you intentionally
  need LAN access.
- Treat remote dispatch as live remote control: `dispatch-work --peer` asks the
  peer statusd to create/reuse tmux sessions and send keys on that machine.
  Only expose statusd TCP binds on trusted networks.
- Use explicit tmux targets and dry-runs before `--execute`.
- Keep watcher allowlists narrow.

## Reporting

Report vulnerabilities with GitHub Security Advisories when available, or open
a GitHub issue if the report does not need to stay private. Include the
command, config file, expected behavior, observed behavior, and whether a live
tmux pane was modified.
