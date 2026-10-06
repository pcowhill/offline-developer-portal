#!/bin/sh
# Builds ready-to-run release archives:
#   release/offline-developer-portal-VERSION-windows-amd64.zip
#   release/offline-developer-portal-VERSION-linux-amd64.tar.gz
#   release/SHA256SUMS.txt
#
# Usage: scripts/package-release.sh VERSION [OUTPUT_DIR]
# Requires Go, zip, tar and sha256sum. Uses the committed dist/ directory.
set -eu
VERSION=${1:?usage: scripts/package-release.sh VERSION [OUTPUT_DIR]}
ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
OUT=${2:-$ROOT/release}
cd "$ROOT"

case "$VERSION" in
  *[!A-Za-z0-9._-]*) echo "invalid version: $VERSION" >&2; exit 1 ;;
esac
[ -f dist/index.html ] || { echo "dist/index.html missing; run npm run build" >&2; exit 1; }

rm -rf "$OUT"
mkdir -p "$OUT"

crlf() { sed 's/\r*$/\r/' "$1" > "$2"; }

for OS in windows linux; do
  NAME="offline-developer-portal-$VERSION-$OS-amd64"
  STAGE="$OUT/$NAME"
  mkdir -p "$STAGE"
  BIN="offline-docs"
  [ "$OS" = windows ] && BIN="offline-docs.exe"
  echo "Building $OS/amd64 binary..."
  GOOS=$OS GOARCH=amd64 CGO_ENABLED=0 go build -trimpath \
    -ldflags "-s -w -X main.version=$VERSION" -o "$STAGE/$BIN" ./cmd/offline-docs
  cp -R dist "$STAGE/dist"
  if [ "$OS" = windows ]; then
    crlf index-windows.cmd "$STAGE/index-windows.cmd"
    crlf serve-windows.cmd "$STAGE/serve-windows.cmd"
    crlf sources.example.yaml "$STAGE/sources.example.yaml"
    crlf packaging/QUICKSTART-windows.txt "$STAGE/QUICKSTART.txt"
    (cd "$OUT" && zip -qrX "$NAME.zip" "$NAME")
  else
    cp index-linux.sh serve-linux.sh sources.example.yaml "$STAGE/"
    cp packaging/QUICKSTART-linux.md "$STAGE/QUICKSTART.md"
    chmod 755 "$STAGE/$BIN" "$STAGE/index-linux.sh" "$STAGE/serve-linux.sh"
    tar --owner=0 --group=0 --numeric-owner -C "$OUT" -czf "$OUT/$NAME.tar.gz" "$NAME"
  fi
  rm -rf "$STAGE"
done

(cd "$OUT" && sha256sum ./*.zip ./*.tar.gz | sed 's# \./# #' > SHA256SUMS.txt)

echo "Verifying archive contents..."
"$ROOT/scripts/check-release.sh" "$OUT" "$VERSION"
echo
ls -l "$OUT"
cat "$OUT/SHA256SUMS.txt"
