#!/bin/sh
# Verifies that release archives contain everything an end user needs and
# nothing that must not be shipped (sources.yaml, corpus, source code).
# Usage: scripts/check-release.sh RELEASE_DIR VERSION
set -eu
OUT=${1:?release dir}
VERSION=${2:?version}
fail() { echo "RELEASE CHECK FAILED: $*" >&2; exit 1; }

WIN="offline-developer-portal-$VERSION-windows-amd64"
LIN="offline-developer-portal-$VERSION-linux-amd64"
[ -f "$OUT/$WIN.zip" ] || fail "missing $WIN.zip"
[ -f "$OUT/$LIN.tar.gz" ] || fail "missing $LIN.tar.gz"
[ -f "$OUT/SHA256SUMS.txt" ] || fail "missing SHA256SUMS.txt"
(cd "$OUT" && sha256sum -c SHA256SUMS.txt >/dev/null) || fail "checksum mismatch"

WLIST=$(unzip -Z1 "$OUT/$WIN.zip")
LLIST=$(tar -tzf "$OUT/$LIN.tar.gz")

for f in offline-docs.exe index-windows.cmd serve-windows.cmd sources.example.yaml QUICKSTART.txt dist/index.html dist/favicon.svg; do
  echo "$WLIST" | grep -qx "$WIN/$f" || fail "windows package lacks $f"
done
for f in offline-docs index-linux.sh serve-linux.sh sources.example.yaml QUICKSTART.md dist/index.html dist/favicon.svg; do
  echo "$LLIST" | grep -qx "$LIN/$f" || fail "linux package lacks $f"
done
echo "$WLIST" | grep -q "^$WIN/dist/assets/.*\.js$" || fail "windows package lacks JS assets"
echo "$LLIST" | grep -q "^$LIN/dist/assets/.*\.js$" || fail "linux package lacks JS assets"

for list in "$WLIST" "$LLIST"; do
  if echo "$list" | grep -Eq '/(sources\.yaml|corpus/|node_modules/|internal/|src/|\.git/)'; then
    fail "package contains files that must not be shipped"
  fi
done

TMP=$(mktemp -d)
trap 'rm -rf "$TMP"' EXIT
unzip -q "$OUT/$WIN.zip" -d "$TMP"
grep -q "$(printf '\r')" "$TMP/$WIN/index-windows.cmd" || fail "index-windows.cmd must use CRLF line endings"
head -c 2 "$TMP/$WIN/offline-docs.exe" | grep -q "MZ" || fail "offline-docs.exe is not a Windows executable"
tar -xzf "$OUT/$LIN.tar.gz" -C "$TMP"
[ -x "$TMP/$LIN/offline-docs" ] || fail "linux binary not executable"
[ -x "$TMP/$LIN/index-linux.sh" ] || fail "index-linux.sh not executable"
head -c 4 "$TMP/$LIN/offline-docs" | grep -q "ELF" || fail "offline-docs is not a Linux executable"
if [ "$(uname -s)" = Linux ] && [ "$(uname -m)" = x86_64 ]; then
  "$TMP/$LIN/offline-docs" version | grep -q "$VERSION" || fail "linux binary reports wrong version"
fi
echo "Release archives OK."
