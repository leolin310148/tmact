# MCP server for ChatGPT / Codex

`tmact mcp serve` serves a read-only Model Context Protocol endpoint
(streamable HTTP, stateless) on `/mcp`. It reads the local statusd snapshot,
so sessions and panes from configured peers are included.

```
ChatGPT / Codex ──HTTPS──> Cloudflare Access (Managed OAuth)
                               │ Cloudflare Tunnel
                               ▼
        cloudflared ──> 127.0.0.1:7895  tmact mcp serve  (/mcp only)
                               │ unix socket / tmux / peer HTTP
                               ▼
                           statusd (unchanged)
```

It is a separate process from statusd on purpose: the tunnel only ever reaches
`/mcp`, never the statusd web UI, `/ws/pane`, or `/api/*`, which have no
authentication.

## Tools

| Tool | What it returns |
|---|---|
| `list_sessions` | Sessions with runtime/state; optional `peer` filter (`local` = this machine). |
| `list_panes` | Panes with runtime, state, cwd, pending prompt; optional `session` / `peer` filter. |
| `capture_pane` | Plain-text scrollback of one pane id (`%12`, `mini@%3`), default 200 lines, max 5000. |

All tools are annotated `readOnlyHint: true`. Pane-derived text lives in
`observed_*` fields and `capture_pane` wraps it in explicit
"observed terminal output (untrusted)" markers. `capture_pane` only accepts
pane ids present in the current snapshot.

Not exposed, by design: dispatch, pane input, keys, kill, `/api/command`,
workspace trust. Adding a write tool must keep the repo safety rules: dry-run
by default (`execute: false`), no `trust_folder` on the wire, and a directory
allowlist for dispatch.

## Authentication

ChatGPT custom MCP servers on a public endpoint require OAuth 2.1 + PKCE with
RFC 9728 protected-resource metadata; API keys are not supported. Cloudflare
Access Managed OAuth is the authorization server, and the server verifies the
`Cf-Access-Jwt-Assertion` header (RS256 against the team JWKS, `iss`, `aud`,
`exp`) on every request. Requests without a valid token get 401 at the
Cloudflare edge before reaching the tunnel.

`--no-access` disables the check and is refused unless `--addr` is loopback.

## Cloudflare setup (as deployed for tmact-mcp.puni.tw)

1. **Tunnel**: remote-managed tunnel `tmact-mcp`, ingress
   `tmact-mcp.puni.tw → http://127.0.0.1:7895`, catch-all `http_status:404`.
   Proxied CNAME `tmact-mcp` → `<tunnel-id>.cfargotunnel.com`. Save the
   connector token to `~/.tmact/mcp-tunnel.token` (0600).
2. **Access application**: self-hosted, domain `tmact-mcp.puni.tw`, policy
   allowing only the owner's email (One-time PIN login).
3. **Managed OAuth** on that application, with dynamic client registration
   allowing:
   - `https://chatgpt.com/connector_platform_oauth_redirect`
   - `https://chatgpt.com/connector/oauth/*`
   - loopback / localhost redirects (Codex desktop uses
     `http://127.0.0.1:<port>/callback`). Changes take about a minute.
4. **WAF**: the zone's custom rule `tw only` blocks non-TW countries. OpenAI
   connects from US Azure IPs, so the rule carries
   `and http.host ne "tmact-mcp.puni.tw"`; Access is the gate for that host.
   (Free plan allows 5 custom rules, so this is an exclusion in the existing
   rule rather than a separate skip rule.)
5. Note the Zero Trust team name (`<team>.cloudflareaccess.com`) and the
   application AUD tag; the server needs both.

## Install (hub machine only)

```sh
scripts/install.sh                       # tmact binary + statusd
scripts/install-mcp.sh --access-team puni-tw --access-aud <aud-tag>
```

This installs `com.tmact.mcp` and `com.tmact.mcp-tunnel` launchd agents. Logs:
`/tmp/tmact-mcp.err.log`, `/tmp/tmact-mcp-tunnel.err.log`. The server logs
JSON-RPC method and tool names with the Access user, never arguments or pane
text. `scripts/install.sh` does not install these, so deploying to every
machine does not open more tunnels. `scripts/install-mcp.sh --uninstall`
removes them.

## Connecting

- **ChatGPT** (Developer mode on): Plugins → New → Add custom MCP server, URL
  `https://tmact-mcp.puni.tw/mcp`, OAuth with client id left empty (DCR). No
  plugin package upload is needed for personal use.
- **Codex**: `[mcp_servers.tmact] url = "https://tmact-mcp.puni.tw/mcp"` in
  `~/.codex/config.toml`, then `codex mcp login tmact`.
