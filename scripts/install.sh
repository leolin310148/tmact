#!/usr/bin/env bash
#
# Install tmact: build the binary into ~/.local/bin and refresh the statusd
# service so it runs the installed binary instead of a repo-local build:
#   macOS  -> launchd user agent  (launchd/com.tmact.statusd.plist.in)
#   Linux  -> systemd user unit   (systemd/tmact-statusd.service.in)
#            (Linux without `systemctl --user` — e.g. WSL with systemd
#             disabled — installs the binary only and prints a manual hint.)
#
# Usage:
#   scripts/install.sh              build binary + refresh statusd service
#   scripts/install.sh --bin-only   build/install the binary only
#
# Overridable via env:
#   TMACT_BIN_DIR   install directory for the binary (default: ~/.local/bin)
#   TMACT_PATH      PATH written into the service unit
#   TMACT_CODESIGN_IDENTITY    macOS certificate name (default: tmact-signing)
#   TMACT_CODESIGN_IDENTIFIER  macOS code identifier (default: com.leolin.tmact)
#
# statusd reads ~/.tmact/statusd.json itself and seeds defaults on first run.
# To change the web bind address, edit that file and reload the service:
#   macOS:  launchctl kickstart -k gui/$(id -u)/com.tmact.statusd
#   Linux:  systemctl --user restart tmact-statusd.service

set -euo pipefail

REPO_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
BIN_DIR="${TMACT_BIN_DIR:-$HOME/.local/bin}"
BIN_PATH="$BIN_DIR/tmact"
SERVICE_PATH="${TMACT_PATH:-$BIN_DIR:/opt/homebrew/bin:/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin}"

PLIST_LABEL="com.tmact.statusd"
PLIST_TEMPLATE="$REPO_DIR/launchd/$PLIST_LABEL.plist.in"
PLIST_DST="$HOME/Library/LaunchAgents/$PLIST_LABEL.plist"

SYSTEMD_UNIT="tmact-statusd.service"
SYSTEMD_TEMPLATE="$REPO_DIR/systemd/$SYSTEMD_UNIT.in"
SYSTEMD_DST="$HOME/.config/systemd/user/$SYSTEMD_UNIT"

# launchctl bootout returns before the service finishes tearing down, and a
# bootstrap issued during teardown fails with "Bootstrap failed: 5:
# Input/output error". Wait for the label to disappear and retry bootstrap.
wait_label_gone() {
  i=0
  while launchctl print "$1/$2" >/dev/null 2>&1; do
    i=$((i + 1))
    [ "$i" -ge 20 ] && return 1
    sleep 1
  done
  return 0
}
bootstrap_retry() {
  i=0
  until launchctl bootstrap "$1" "$2"; do
    i=$((i + 1))
    [ "$i" -ge 5 ] && return 1
    sleep 2
  done
  return 0
}

BIN_ONLY=0
for arg in "$@"; do
  case "$arg" in
    --bin-only) BIN_ONLY=1 ;;
    -h|--help) sed -n '2,18p' "${BASH_SOURCE[0]}" | sed 's/^# \{0,1\}//'; exit 0 ;;
    *) echo "unknown argument: $arg" >&2; exit 2 ;;
  esac
done

cd "$REPO_DIR"

echo "==> Building frontend assets"
make web

echo "==> Building tmact"
mkdir -p "$BIN_DIR"
BUILD_PATH="$(mktemp "$BIN_DIR/.tmact-build.XXXXXX")"
trap 'rm -f "$BUILD_PATH"' EXIT INT TERM
go build -o "$BUILD_PATH" ./cmd/tmact
chmod 755 "$BUILD_PATH"

if [[ "$(uname -s)" == "Darwin" ]]; then
  echo "==> Signing tmact"
  "$REPO_DIR/scripts/macos-codesign.sh" "$BUILD_PATH"
fi

mv -f "$BUILD_PATH" "$BIN_PATH"
trap - EXIT INT TERM
echo "    installed: $BIN_PATH"

case ":$PATH:" in
  *":$BIN_DIR:"*) ;;
  *) echo "    WARNING: $BIN_DIR is not on your PATH — add it to your shell profile" ;;
esac

if [[ "$BIN_ONLY" -eq 0 ]]; then
  case "$(uname)" in
    Darwin)
      if [[ -f "$PLIST_TEMPLATE" ]]; then
        echo "==> Refreshing statusd launchd agent"
        mkdir -p "$HOME/Library/LaunchAgents"
        sed \
          -e "s#__TMACT_BIN__#$BIN_PATH#g" \
          -e "s#__TMACT_WORKDIR__#$REPO_DIR#g" \
          -e "s#__TMACT_PATH__#${SERVICE_PATH}#g" \
          "$PLIST_TEMPLATE" > "$PLIST_DST"
        echo "    statusd reads $HOME/.tmact/statusd.json (auto-seeded on first run)"
        # Use the modern bootout/bootstrap API: the legacy load/unload calls fail
        # with "Input/output error" on recent macOS once the agent is loaded.
        domain="gui/$(id -u)"
        launchctl bootout "$domain/$PLIST_LABEL" 2>/dev/null || true
        wait_label_gone "$domain" "$PLIST_LABEL" \
          || echo "    WARNING: $PLIST_LABEL still registered after bootout; retrying bootstrap anyway"
        bootstrap_retry "$domain" "$PLIST_DST"
        echo "    loaded: $PLIST_DST"
      fi
      ;;
    Linux)
      if [[ ! -f "$SYSTEMD_TEMPLATE" ]]; then
        :
      elif ! command -v systemctl >/dev/null 2>&1 \
        || ! systemctl --user show-environment >/dev/null 2>&1; then
        echo "==> Skipping statusd service: systemctl --user not available"
        echo "    (WSL? enable systemd in /etc/wsl.conf, or start statusd manually:"
        echo "       $BIN_PATH statusd start &)"
      else
        echo "==> Refreshing statusd systemd user unit"
        mkdir -p "$(dirname "$SYSTEMD_DST")"
        sed \
          -e "s#__TMACT_BIN__#$BIN_PATH#g" \
          -e "s#__TMACT_WORKDIR__#$REPO_DIR#g" \
          -e "s#__TMACT_PATH__#${SERVICE_PATH}#g" \
          "$SYSTEMD_TEMPLATE" > "$SYSTEMD_DST"
        echo "    statusd reads $HOME/.tmact/statusd.json (auto-seeded on first run)"
        systemctl --user daemon-reload
        systemctl --user enable --now "$SYSTEMD_UNIT"
        echo "    loaded: $SYSTEMD_DST"
      fi
      ;;
    *)
      echo "==> Skipping statusd auto-start: unsupported OS $(uname)"
      echo "    start manually: $BIN_PATH statusd start"
      ;;
  esac
fi

echo "==> Done"
"$BIN_PATH" help >/dev/null && echo "    tmact is callable: $(command -v tmact || echo "$BIN_PATH")"
