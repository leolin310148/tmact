#!/usr/bin/env bash
# Sign a tmact binary with the stable local macOS code-signing identity.

set -euo pipefail

IDENTITY="${TMACT_CODESIGN_IDENTITY:-tmact-signing}"
IDENTIFIER="${TMACT_CODESIGN_IDENTIFIER:-com.leolin.tmact}"

usage() {
  cat <<'EOF'
Usage: scripts/macos-codesign.sh [--check | BINARY]

Checks for the tmact macOS code-signing identity, or signs and verifies BINARY.
On non-macOS systems the command is a no-op.

Environment:
  TMACT_CODESIGN_IDENTITY    certificate name (default: tmact-signing)
  TMACT_CODESIGN_IDENTIFIER code identifier (default: com.leolin.tmact)
EOF
}

die() {
  echo "macos-codesign: $*" >&2
  exit 1
}

case "$IDENTITY" in
  ''|*[!A-Za-z0-9._-]*)
    die "TMACT_CODESIGN_IDENTITY must contain only letters, digits, '.', '_', or '-'"
    ;;
esac

case "$IDENTIFIER" in
  ''|*[!A-Za-z0-9.-]*|.*|-*|*.)
    die "TMACT_CODESIGN_IDENTIFIER must be a dot-separated identifier"
    ;;
esac

mode="sign"
case "${1:-}" in
  --check)
    mode="check"
    shift
    ;;
  -h|--help)
    usage
    exit 0
    ;;
esac

if [[ "$#" -gt 1 ]] || { [[ "$mode" == "sign" ]] && [[ "$#" -ne 1 ]]; } || { [[ "$mode" == "check" ]] && [[ "$#" -ne 0 ]]; }; then
  usage >&2
  exit 2
fi

if [[ "$(uname -s)" != "Darwin" ]]; then
  exit 0
fi

command -v security >/dev/null 2>&1 || die "security is required on macOS"
command -v codesign >/dev/null 2>&1 || die "codesign is required on macOS"

# Resolve the exact certificate hash rather than passing a potentially
# ambiguous display name to codesign. security's output ends each identity
# line with the quoted certificate common name.
identity_hash="$({ security find-identity -v -p codesigning 2>/dev/null || true; } | awk -v wanted="\"$IDENTITY\"" '
  {
    sub(/[[:space:]]+$/, "", $0)
    if (length($0) >= length(wanted) && substr($0, length($0) - length(wanted) + 1) == wanted) {
      print $2
      exit
    }
  }
')"

if [[ ! "$identity_hash" =~ ^[[:xdigit:]]{40}$ ]]; then
  cat >&2 <<EOF
macos-codesign: no valid code-signing identity named "$IDENTITY" was found
Create it once with:
  scripts/setup-macos-codesigning.sh
EOF
  exit 1
fi

if [[ "$mode" == "check" ]]; then
  echo "macOS code-signing identity ready: $IDENTITY ($identity_hash)"
  exit 0
fi

binary="$1"
[[ -f "$binary" ]] || die "binary not found: $binary"

requirement="identifier \"$IDENTIFIER\" and certificate leaf = H\"$identity_hash\""

codesign \
  --force \
  --sign "$identity_hash" \
  --identifier "$IDENTIFIER" \
  --requirements "=designated => $requirement" \
  --timestamp=none \
  "$binary"

codesign --verify --strict --verbose=2 -R "=$requirement" "$binary"
echo "    signed: $binary ($IDENTIFIER, $IDENTITY)"
