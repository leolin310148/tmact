#!/usr/bin/env bash
#
# Install tmact: build the binary into ~/.local/bin and (on macOS) refresh the
# statusd launchd agent so it runs the installed binary instead of a repo-local
# build.
#
# Usage:
#   scripts/install.sh              build binary + refresh statusd agent
#   scripts/install.sh --bin-only   build/install the binary only
#
# Overridable via env:
#   TMACT_BIN_DIR   install directory for the binary (default: ~/.local/bin)
#   TMACT_PATH      PATH written into the LaunchAgent
#
# statusd reads ~/.tmact/statusd.json itself and seeds defaults on first run.
# To change the web bind address, edit that file and reload:
#   launchctl kickstart -k gui/$(id -u)/com.tmact.statusd

set -euo pipefail

REPO_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
BIN_DIR="${TMACT_BIN_DIR:-$HOME/.local/bin}"
BIN_PATH="$BIN_DIR/tmact"
LAUNCHD_PATH="${TMACT_PATH:-$BIN_DIR:/opt/homebrew/bin:/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin}"

PLIST_LABEL="com.tmact.statusd"
PLIST_TEMPLATE="$REPO_DIR/launchd/$PLIST_LABEL.plist.in"
PLIST_DST="$HOME/Library/LaunchAgents/$PLIST_LABEL.plist"

BIN_ONLY=0
for arg in "$@"; do
  case "$arg" in
    --bin-only) BIN_ONLY=1 ;;
    -h|--help) sed -n '2,13p' "${BASH_SOURCE[0]}" | sed 's/^# \{0,1\}//'; exit 0 ;;
    *) echo "unknown argument: $arg" >&2; exit 2 ;;
  esac
done

cd "$REPO_DIR"

echo "==> Building tmact"
mkdir -p "$BIN_DIR"
go build -o "$BIN_PATH" ./cmd/tmact
echo "    installed: $BIN_PATH"

case ":$PATH:" in
  *":$BIN_DIR:"*) ;;
  *) echo "    WARNING: $BIN_DIR is not on your PATH — add it to your shell profile" ;;
esac

if [[ "$BIN_ONLY" -eq 0 && "$(uname)" == "Darwin" && -f "$PLIST_TEMPLATE" ]]; then
  echo "==> Refreshing statusd launchd agent"
  mkdir -p "$HOME/Library/LaunchAgents"
  sed \
    -e "s#__TMACT_BIN__#$BIN_PATH#g" \
    -e "s#__TMACT_WORKDIR__#$REPO_DIR#g" \
    -e "s#__TMACT_PATH__#${LAUNCHD_PATH}#g" \
    "$PLIST_TEMPLATE" > "$PLIST_DST"
  echo "    statusd reads $HOME/.tmact/statusd.json (auto-seeded on first run)"
  # Use the modern bootout/bootstrap API: the legacy load/unload calls fail
  # with "Input/output error" on recent macOS once the agent is loaded.
  domain="gui/$(id -u)"
  launchctl bootout "$domain/$PLIST_LABEL" 2>/dev/null || true
  launchctl bootstrap "$domain" "$PLIST_DST"
  echo "    loaded: $PLIST_DST"
fi

echo "==> Done"
"$BIN_PATH" help >/dev/null && echo "    tmact is callable: $(command -v tmact || echo "$BIN_PATH")"
