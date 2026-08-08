#!/usr/bin/env bash
# Create the stable, local-only certificate used to sign tmact on macOS.

set -euo pipefail

IDENTITY="${TMACT_CODESIGN_IDENTITY:-tmact-signing}"
IDENTIFIER="${TMACT_CODESIGN_IDENTIFIER:-com.leolin.tmact}"
KEYCHAIN="${TMACT_CODESIGN_KEYCHAIN:-}"

usage() {
  cat <<'EOF'
Usage: scripts/setup-macos-codesigning.sh

Creates a self-signed code-signing identity in the current user's default
keychain. This is a one-time local setup; subsequent tmact installs reuse the
same private key and certificate. macOS may prompt once to let /usr/bin/codesign
use the private key; choose Always Allow so later installs can run unattended.

Environment:
  TMACT_CODESIGN_IDENTITY certificate name (default: tmact-signing)
  TMACT_CODESIGN_IDENTIFIER identifier used by the verification test
  TMACT_CODESIGN_KEYCHAIN target keychain (default: user default keychain)
EOF
}

die() {
  echo "setup-macos-codesigning: $*" >&2
  exit 1
}

case "${1:-}" in
  -h|--help)
    usage
    exit 0
    ;;
  '') ;;
  *)
    usage >&2
    exit 2
    ;;
esac

[[ "$(uname -s)" == "Darwin" ]] || die "this helper only runs on macOS"

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

for tool in security codesign openssl; do
  command -v "$tool" >/dev/null 2>&1 || die "$tool is required"
done

find_identity_hash() {
  { security find-identity -v -p codesigning 2>/dev/null || true; } | awk -v wanted="\"$IDENTITY\"" '
    {
      sub(/[[:space:]]+$/, "", $0)
      if (length($0) >= length(wanted) && substr($0, length($0) - length(wanted) + 1) == wanted) {
        print $2
        exit
      }
    }
  '
}

umask 077
tmp_dir="$(mktemp -d "${TMPDIR:-/tmp}/tmact-codesign.XXXXXX")"
trap 'rm -rf "$tmp_dir"' EXIT INT TERM

identity_hash="$(find_identity_hash)"
if [[ "$identity_hash" =~ ^[[:xdigit:]]{40}$ ]]; then
  echo "macOS code-signing identity ready: $IDENTITY ($identity_hash)"
  echo "The existing identity was left unchanged; verifying private-key access."
else
  if [[ -z "$KEYCHAIN" ]]; then
    KEYCHAIN="$(security default-keychain -d user | sed -e 's/^[[:space:]]*"//' -e 's/"[[:space:]]*$//')"
  fi
  [[ -n "$KEYCHAIN" ]] || die "could not determine the user's default keychain"

  # Avoid importing a second certificate with the same name when a stale,
  # untrusted, or private-key-less certificate already exists. Removing or
  # repairing that certificate is a decision for the keychain owner.
  if security find-certificate -c "$IDENTITY" "$KEYCHAIN" >/dev/null 2>&1; then
    die "a certificate named '$IDENTITY' exists but is not a valid code-signing identity; inspect it in Keychain Access before retrying"
  fi

  config="$tmp_dir/openssl.cnf"
  key="$tmp_dir/key.pem"
  certificate="$tmp_dir/certificate.pem"
  archive="$tmp_dir/identity.p12"
  archive_password="$(openssl rand -hex 24)"

  cat > "$config" <<EOF
[req]
distinguished_name = subject
x509_extensions = codesign
prompt = no

[subject]
CN = $IDENTITY
O = tmact local signing

[codesign]
basicConstraints = critical, CA:true
keyUsage = critical, digitalSignature, keyCertSign
extendedKeyUsage = critical, codeSigning
subjectKeyIdentifier = hash
authorityKeyIdentifier = keyid:always
EOF

  echo "==> Creating local code-signing identity '$IDENTITY'"
  openssl req \
    -new -newkey rsa:2048 -nodes -x509 -sha256 -days 36500 \
    -config "$config" \
    -keyout "$key" \
    -out "$certificate"

  openssl pkcs12 \
    -export \
    -name "$IDENTITY" \
    -inkey "$key" \
    -in "$certificate" \
    -out "$archive" \
    -passout "pass:$archive_password"

  echo "==> Importing identity into $KEYCHAIN"
  security import "$archive" \
    -k "$KEYCHAIN" \
    -f pkcs12 \
    -P "$archive_password" \
    -T /usr/bin/codesign

  # A user-scoped codeSign trust setting makes the self-signed certificate a
  # valid identity without granting system-wide trust or requiring sudo.
  security add-trusted-cert \
    -r trustRoot \
    -p codeSign \
    -k "$KEYCHAIN" \
    "$certificate"

  echo "==> Verifying identity"
  identity_hash="$(find_identity_hash)"
  [[ "$identity_hash" =~ ^[[:xdigit:]]{40}$ ]] || die "the imported certificate is not available as a valid code-signing identity"
  echo "macOS code-signing identity ready: $IDENTITY ($identity_hash)"
fi

# Exercise private-key access now, while setup is interactive, so any Keychain
# prompt can be handled once rather than during a later unattended install.
test_binary="$tmp_dir/tmact-signing-test"
cp /usr/bin/true "$test_binary"
requirement="identifier \"$IDENTIFIER\" and certificate leaf = H\"$identity_hash\""
codesign \
  --force \
  --sign "$identity_hash" \
  --identifier "$IDENTIFIER" \
  --requirements "=designated => $requirement" \
  --timestamp=none \
  "$test_binary"
codesign --verify --strict --verbose=2 -R "=$requirement" "$test_binary"

cat <<EOF
==> Done
Future tmact installs will sign with '$IDENTITY' and identifier
'$IDENTIFIER'. Keep this identity in the keychain; replacing it changes
tmact's code identity and invalidates prior TCC
grants.
EOF
