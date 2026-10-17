#!/usr/bin/env bash
#
# Install the tmact MCP server and its Cloudflare Tunnel connector as macOS
# launchd user agents. Run this only on the hub machine that should be
# reachable from ChatGPT; scripts/install.sh deliberately does not do it.
# See docs/mcp.md for the Cloudflare Tunnel + Access setup.
#
# Usage:
#   scripts/install-mcp.sh --access-team TEAM --access-aud AUD
#   scripts/install-mcp.sh --uninstall
#
# Overridable via env:
#   TMACT_BIN          tmact binary (default: ~/.local/bin/tmact)
#   TMACT_MCP_ADDR     listen address (default: 127.0.0.1:7895)
#   TMACT_MCP_TOKEN    tunnel token file (default: ~/.tmact/mcp-tunnel.token)
#   TMACT_MCP_DISPATCH_ROOTS
#                      comma-separated ~/ roots for dispatch_work/send_text
#                      (default: ~/w,~/ops; empty keeps the server read-only)
#   CLOUDFLARED_BIN    cloudflared binary (default: from PATH)

set -euo pipefail

REPO_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
TMACT_BIN="${TMACT_BIN:-$HOME/.local/bin/tmact}"
MCP_ADDR="${TMACT_MCP_ADDR:-127.0.0.1:7895}"
TOKEN_FILE="${TMACT_MCP_TOKEN:-$HOME/.tmact/mcp-tunnel.token}"
DISPATCH_ROOTS="${TMACT_MCP_DISPATCH_ROOTS-~/w,~/ops}"
SERVICE_PATH="${TMACT_PATH:-$(dirname "$TMACT_BIN"):/opt/homebrew/bin:/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin}"
AGENTS_DIR="$HOME/Library/LaunchAgents"
domain="gui/$(id -u)"

team="" aud="" uninstall=0
while [ $# -gt 0 ]; do
  case "$1" in
    --access-team) team="$2"; shift 2 ;;
    --access-aud) aud="$2"; shift 2 ;;
    --uninstall) uninstall=1; shift ;;
    -h|--help) sed -n '2,20p' "${BASH_SOURCE[0]}" | sed 's/^# \{0,1\}//'; exit 0 ;;
    *) echo "unknown argument: $1" >&2; exit 2 ;;
  esac
done

[ "$(uname -s)" = Darwin ] || { echo "install-mcp.sh supports macOS only" >&2; exit 1; }

wait_label_gone() {
  i=0
  while launchctl print "$domain/$1" >/dev/null 2>&1; do
    i=$((i + 1))
    [ "$i" -ge 20 ] && return 1
    sleep 1
  done
}
bootstrap_retry() {
  i=0
  until launchctl bootstrap "$domain" "$1"; do
    i=$((i + 1))
    [ "$i" -ge 5 ] && return 1
    sleep 2
  done
}
reload() {
  label="$1" dst="$AGENTS_DIR/$1.plist"
  launchctl bootout "$domain/$label" 2>/dev/null || true
  wait_label_gone "$label"
  bootstrap_retry "$dst"
  echo "loaded $label"
}

if [ "$uninstall" = 1 ]; then
  for label in com.tmact.mcp-tunnel com.tmact.mcp; do
    launchctl bootout "$domain/$label" 2>/dev/null || true
    rm -f "$AGENTS_DIR/$label.plist"
    echo "removed $label"
  done
  exit 0
fi

[ -n "$team" ] && [ -n "$aud" ] || { echo "need --access-team and --access-aud" >&2; exit 2; }
[ -x "$TMACT_BIN" ] || { echo "tmact binary not found at $TMACT_BIN (run scripts/install.sh first)" >&2; exit 1; }
[ -s "$TOKEN_FILE" ] || { echo "tunnel token file missing: $TOKEN_FILE" >&2; exit 1; }
CLOUDFLARED_BIN="${CLOUDFLARED_BIN:-$(command -v cloudflared || true)}"
[ -n "$CLOUDFLARED_BIN" ] || { echo "cloudflared not found" >&2; exit 1; }
chmod 600 "$TOKEN_FILE"
mkdir -p "$AGENTS_DIR"

sed \
  -e "s|__TMACT_BIN__|$TMACT_BIN|g" \
  -e "s|__MCP_ADDR__|$MCP_ADDR|g" \
  -e "s|__ACCESS_TEAM__|$team|g" \
  -e "s|__ACCESS_AUD__|$aud|g" \
  -e "s|__DISPATCH_ROOTS__|$DISPATCH_ROOTS|g" \
  -e "s|__TMACT_PATH__|$SERVICE_PATH|g" \
  "$REPO_DIR/launchd/com.tmact.mcp.plist.in" >"$AGENTS_DIR/com.tmact.mcp.plist"
sed \
  -e "s|__CLOUDFLARED_BIN__|$CLOUDFLARED_BIN|g" \
  -e "s|__TOKEN_FILE__|$TOKEN_FILE|g" \
  "$REPO_DIR/launchd/com.tmact.mcp-tunnel.plist.in" >"$AGENTS_DIR/com.tmact.mcp-tunnel.plist"

reload com.tmact.mcp
reload com.tmact.mcp-tunnel
echo "logs: /tmp/tmact-mcp.err.log /tmp/tmact-mcp-tunnel.err.log"
