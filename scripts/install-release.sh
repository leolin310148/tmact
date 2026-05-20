#!/usr/bin/env sh
#
# Install the latest macOS tmact binary from GitHub Releases.
#
# Usage:
#   curl -fsSL https://raw.githubusercontent.com/leolin310148/tmact/main/scripts/install-release.sh | sh
#
# Optional environment:
#   TMACT_REPO=owner/repo           GitHub repository (default: leolin310148/tmact)
#   TMACT_VERSION=v0.1.0            Release tag to install (default: latest)
#   TMACT_BIN_DIR=$HOME/.local/bin  Install directory
#   TMACT_INSTALL_STATUSD=1         Also install the macOS LaunchAgent
#   GH_TOKEN=...                    Token for private release downloads
#
# statusd reads ~/.tmact/statusd.json itself and seeds defaults on first run.
# To change the web bind address, edit that file and reload:
#   launchctl kickstart -k gui/$(id -u)/com.tmact.statusd

set -eu

repo="${TMACT_REPO:-leolin310148/tmact}"
version="${TMACT_VERSION:-latest}"
bin_dir="${TMACT_BIN_DIR:-$HOME/.local/bin}"

case "$(uname -s)" in
  Darwin) os="darwin" ;;
  *) echo "tmact release installer currently supports macOS only" >&2; exit 1 ;;
esac

case "$(uname -m)" in
  arm64) arch="arm64" ;;
  x86_64) arch="amd64" ;;
  *) echo "unsupported macOS architecture: $(uname -m)" >&2; exit 1 ;;
esac

asset="tmact_${os}_${arch}.tar.gz"
tmp_dir="$(mktemp -d)"
trap 'rm -rf "$tmp_dir"' EXIT INT TERM

download() {
  src="$1"
  dst="$2"
  if [ -n "${GH_TOKEN:-}" ]; then
    curl -fsSL -H "Authorization: Bearer ${GH_TOKEN}" "$src" -o "$dst"
  else
    curl -fsSL "$src" -o "$dst"
  fi
}

echo "==> Downloading ${repo} ${version} (${os}/${arch})"

if command -v gh >/dev/null 2>&1; then
  if [ "$version" = "latest" ]; then
    gh release download -R "$repo" -p "$asset" -p checksums.txt -D "$tmp_dir" --clobber 2>/dev/null || true
  else
    gh release download "$version" -R "$repo" -p "$asset" -p checksums.txt -D "$tmp_dir" --clobber 2>/dev/null || true
  fi
fi

if [ ! -f "$tmp_dir/$asset" ]; then
  if [ "$version" = "latest" ]; then
    base_url="https://github.com/${repo}/releases/latest/download"
  else
    base_url="https://github.com/${repo}/releases/download/${version}"
  fi
  download "${base_url}/${asset}" "$tmp_dir/$asset"
  download "${base_url}/checksums.txt" "$tmp_dir/checksums.txt" || true
fi

if [ -f "$tmp_dir/checksums.txt" ]; then
  if grep " ${asset}\$" "$tmp_dir/checksums.txt" > "$tmp_dir/checksum.txt"; then
    (cd "$tmp_dir" && shasum -a 256 -c checksum.txt)
  fi
else
  echo "    WARNING: checksums.txt not available; skipping verification" >&2
fi
tar -xzf "$tmp_dir/$asset" -C "$tmp_dir"

mkdir -p "$bin_dir"
install "$tmp_dir/tmact" "$bin_dir/tmact"
echo "==> Installed $bin_dir/tmact"

case ":$PATH:" in
  *":$bin_dir:"*) ;;
  *) echo "    WARNING: $bin_dir is not on your PATH" ;;
esac

if [ "${TMACT_INSTALL_STATUSD:-0}" = "1" ]; then
  label="com.tmact.statusd"
  plist="$HOME/Library/LaunchAgents/${label}.plist"
  bin_path="$bin_dir/tmact"

  echo "==> Installing ${label} LaunchAgent"
  echo "    statusd reads ${HOME}/.tmact/statusd.json (auto-seeded on first run)"
  mkdir -p "$HOME/Library/LaunchAgents"
  cat > "$plist" <<EOF
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key>
  <string>${label}</string>
  <key>ProgramArguments</key>
  <array>
    <string>${bin_path}</string>
    <string>statusd</string>
    <string>start</string>
  </array>
  <key>WorkingDirectory</key>
  <string>${HOME}</string>
  <key>EnvironmentVariables</key>
  <dict>
    <key>PATH</key>
    <string>${PATH}</string>
  </dict>
  <key>RunAtLoad</key>
  <true/>
  <key>KeepAlive</key>
  <true/>
  <key>StandardOutPath</key>
  <string>/tmp/tmact-statusd.out.log</string>
  <key>StandardErrorPath</key>
  <string>/tmp/tmact-statusd.err.log</string>
</dict>
</plist>
EOF
  domain="gui/$(id -u)"
  launchctl bootout "$domain/$label" 2>/dev/null || true
  launchctl bootstrap "$domain" "$plist"
  echo "==> Loaded $plist"
fi

"$bin_dir/tmact" version
