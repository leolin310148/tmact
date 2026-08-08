#!/usr/bin/env bash

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
TEST_ROOT="$(mktemp -d "${TMPDIR:-/tmp}/tmact-codesign-test.XXXXXX")"
trap 'rm -rf "$TEST_ROOT"' EXIT INT TERM

MOCK_BIN="$TEST_ROOT/bin"
LOG="$TEST_ROOT/codesign.log"
BINARY="$TEST_ROOT/tmact"
mkdir -p "$MOCK_BIN"
touch "$BINARY"

cat > "$MOCK_BIN/uname" <<'EOF'
#!/usr/bin/env sh
case "${1:-}" in
  -m) echo "${MOCK_ARCH:-arm64}" ;;
  *) echo "${MOCK_UNAME:-Darwin}" ;;
esac
EOF

cat > "$MOCK_BIN/security" <<'EOF'
#!/usr/bin/env sh
if [ "${MOCK_IDENTITY_FOUND:-0}" = "1" ]; then
  echo '  1) 0123456789ABCDEF0123456789ABCDEF01234567 "tmact-signing"'
  echo '     1 valid identities found'
else
  echo '     0 valid identities found'
fi
EOF

cat > "$MOCK_BIN/codesign" <<'EOF'
#!/usr/bin/env sh
printf '%s\n' "$*" >> "$MOCK_CODESIGN_LOG"
EOF

chmod +x "$MOCK_BIN/uname" "$MOCK_BIN/security" "$MOCK_BIN/codesign"

PATH="$MOCK_BIN:$PATH" MOCK_UNAME=Linux \
  "$REPO_ROOT/scripts/macos-codesign.sh" "$BINARY"

missing_output="$TEST_ROOT/missing.out"
if PATH="$MOCK_BIN:$PATH" MOCK_IDENTITY_FOUND=0 \
  "$REPO_ROOT/scripts/macos-codesign.sh" "$BINARY" >"$missing_output" 2>&1; then
  echo "expected missing identity check to fail" >&2
  exit 1
fi
grep -F 'scripts/setup-macos-codesigning.sh' "$missing_output" >/dev/null

PATH="$MOCK_BIN:$PATH" \
MOCK_IDENTITY_FOUND=1 \
MOCK_CODESIGN_LOG="$LOG" \
  "$REPO_ROOT/scripts/macos-codesign.sh" "$BINARY" >/dev/null

grep -F -- '--sign 0123456789ABCDEF0123456789ABCDEF01234567' "$LOG" >/dev/null
grep -F -- '--identifier com.leolin.tmact' "$LOG" >/dev/null
grep -F -- '=designated => identifier "com.leolin.tmact" and certificate leaf = H"0123456789ABCDEF0123456789ABCDEF01234567"' "$LOG" >/dev/null
grep -F -- '--verify --strict --verbose=2' "$LOG" >/dev/null
if ! grep -F -- '-R =identifier "com.leolin.tmact" and certificate leaf' "$LOG" >/dev/null; then
  echo "codesign verification requirement must be passed as literal source beginning with '='" >&2
  exit 1
fi

# Re-running setup with an existing identity must still exercise private-key
# access and verify the designated requirement instead of exiting early.
cat > "$MOCK_BIN/openssl" <<'EOF'
#!/usr/bin/env sh
exit 0
EOF
chmod +x "$MOCK_BIN/openssl"
: > "$LOG"
PATH="$MOCK_BIN:/usr/bin:/bin" \
MOCK_IDENTITY_FOUND=1 \
MOCK_CODESIGN_LOG="$LOG" \
  bash "$REPO_ROOT/scripts/setup-macos-codesigning.sh" >/dev/null

grep -F -- '--sign 0123456789ABCDEF0123456789ABCDEF01234567' "$LOG" >/dev/null
grep -F -- '--verify --strict --verbose=2 -R =identifier "com.leolin.tmact"' "$LOG" >/dev/null

# Source installs build to a temporary file, sign it, and only then replace the
# installed binary.
cat > "$MOCK_BIN/make" <<'EOF'
#!/usr/bin/env sh
exit 0
EOF

cat > "$MOCK_BIN/go" <<'EOF'
#!/usr/bin/env sh
destination=""
while [ "$#" -gt 0 ]; do
  if [ "$1" = "-o" ]; then
    destination="$2"
    shift 2
  else
    shift
  fi
done
cat > "$destination" <<'SCRIPT'
#!/usr/bin/env sh
echo "tmact test source build"
SCRIPT
chmod +x "$destination"
EOF

chmod +x "$MOCK_BIN/make" "$MOCK_BIN/go"

SOURCE_BIN_DIR="$TEST_ROOT/source-bin"
: > "$LOG"
PATH="$MOCK_BIN:/usr/bin:/bin" \
MOCK_IDENTITY_FOUND=1 \
MOCK_CODESIGN_LOG="$LOG" \
TMACT_BIN_DIR="$SOURCE_BIN_DIR" \
  bash "$REPO_ROOT/scripts/install.sh" --bin-only >/dev/null

test -x "$SOURCE_BIN_DIR/tmact"
grep -F -- '--identifier com.leolin.tmact' "$LOG" >/dev/null
grep -F -- '--verify --strict --verbose=2' "$LOG" >/dev/null

# The standalone release installer carries the same signing behavior because
# it must work when piped directly to sh without a source checkout.
cat > "$MOCK_BIN/curl" <<'EOF'
#!/usr/bin/env sh
destination=""
while [ "$#" -gt 0 ]; do
  if [ "$1" = "-o" ]; then
    destination="$2"
    shift 2
  else
    shift
  fi
done
case "$destination" in
  *checksums.txt) exit 1 ;;
  '') exit 2 ;;
  *) : > "$destination" ;;
esac
EOF

cat > "$MOCK_BIN/tar" <<'EOF'
#!/usr/bin/env sh
destination=""
while [ "$#" -gt 0 ]; do
  if [ "$1" = "-C" ]; then
    destination="$2"
    shift 2
  else
    shift
  fi
done
cat > "$destination/tmact" <<'SCRIPT'
#!/usr/bin/env sh
echo "tmact test release"
SCRIPT
chmod +x "$destination/tmact"
EOF

chmod +x "$MOCK_BIN/curl" "$MOCK_BIN/tar"

RELEASE_BIN_DIR="$TEST_ROOT/release-bin"
: > "$LOG"
PATH="$MOCK_BIN:/usr/bin:/bin" \
MOCK_IDENTITY_FOUND=1 \
MOCK_CODESIGN_LOG="$LOG" \
TMACT_BIN_DIR="$RELEASE_BIN_DIR" \
TMACT_REPO="example/tmact" \
  sh "$REPO_ROOT/scripts/install-release.sh" >/dev/null 2>&1

test -x "$RELEASE_BIN_DIR/tmact"
grep -F -- '--identifier com.leolin.tmact' "$LOG" >/dev/null
grep -F -- '--verify --strict --verbose=2' "$LOG" >/dev/null

echo "macOS code-signing script checks passed"
