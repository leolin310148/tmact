# MCP server for ChatGPT / Codex

`tmact mcp serve` serves a Model Context Protocol endpoint (streamable HTTP,
stateless) on `/mcp`. It is read-only unless `--dispatch-root` is given. It reads the local statusd snapshot,
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

| Tool | Kind | What it does |
|---|---|---|
| `list_sessions` | read | Sessions with runtime/state; optional `peer` filter (`local` = this machine). |
| `list_panes` | read | Panes with runtime, state, cwd, pending prompt; optional `session` / `peer` filter. |
| `capture_pane` | read | Plain-text scrollback of one pane (`work:0.0`, `%12`, `mini@ops:0.0`), default 200 lines, max 5000. |
| `wait_pane` | read | Poll until a pane is idle (for `settle_seconds`) or shows a prompt, up to 80s; outcome `idle` / `asking` / `gone` / `timeout`. |
| `dispatch_work` | write | `tmact dispatch-work` into an allowed dir, locally or on a peer; returns once the prompt is submitted. |
| `send_text` | write | Paste a follow-up into an idle agent pane and press Enter. |

Read tools are annotated `readOnlyHint: true`; write tools `destructiveHint:
true`, so ChatGPT asks for confirmation before each call. That confirmation
replaces the CLI's dry-run default: `dispatch_work` always executes. The
server also sends `instructions` telling the client how to chain
dispatch → wait → capture and to treat pane text as untrusted.

Pane-derived text lives in `observed_*` fields and `capture_pane` wraps it in
explicit "observed terminal output (untrusted)" markers. Pane tools only accept
panes present in the current snapshot.

### Write-tool rules

- Write tools exist only when `--dispatch-root` is set (the hub uses
  `~/w,~/ops`). Unknown input fields are rejected by schema validation.
- `dir` must be `~/...` under a root: no absolute paths, no `..`, prefix match
  on path segments (`~/wat` is not under `~/w`). On the hub the dir is also
  resolved with symlinks and re-checked, so `~/w/link -> /` is refused. Peers
  get the lexical check only and expand `~` against their own home
  (`dispatch.ExpandHome`), which needs a peer binary that includes it.
- Workspace trust is never accepted (`TrustFolder` is hard-coded false and not
  on the wire). Trust a new dir once with `tmact trust-folder`.
- `session` must match `[A-Za-z0-9_-]{1,64}`; `agent` is codex/claude/gemini.
  `continue_conversation` (no clear) is hub-only, like the CLI's `--peer` rule.
- `send_text` refuses shell panes, stale panes, panes showing a prompt, and
  busy panes. It reports whether the pane visibly picked up the text, since a
  plain paste has no Enter-swallow recovery.
- Calls are capped well below Cloudflare's ~100s origin timeout (ready wait
  45s, whole call 80s) so a slow dispatch does not invite a client retry.

Not exposed: kill, close, fork, raw keys, `/api/command`.

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
  ChatGPT freezes the tool list when the plugin is created: after adding or
  changing tools, use Settings → Plugins → tmact → **Refresh tools** (the
  plugin page itself has no refresh). The server log then shows
  `server/discover` and `tools/list` from the Access user.
- **Codex**: `[mcp_servers.tmact] url = "https://tmact-mcp.puni.tw/mcp"` in
  `~/.codex/config.toml`, then `codex mcp login tmact`.
