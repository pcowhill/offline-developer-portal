#!/bin/sh
# Phase 2: serve the portal and the generated corpus on http://localhost:8080
# Extra arguments are passed through, e.g.: ./serve-linux.sh --port 9000 --open
set -eu
HERE=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)

APP="$HERE/offline-docs"
[ -x "$APP" ] || APP="$HERE/bin/offline-docs"
if [ ! -x "$APP" ]; then
  echo "ERROR: the offline-docs binary was not found next to this script." >&2
  echo "Use the Linux release package (offline-developer-portal-VERSION-linux-amd64.tar.gz)," >&2
  echo "or build it from source with Go 1.25.1+: scripts/build-go.sh" >&2
  exit 1
fi

if [ ! -f "$HERE/corpus/manifest.json" ]; then
  echo "NOTE: no index found yet; run ./index-linux.sh first. Until then the portal only shows setup instructions."
fi

exec "$APP" serve --dist "$HERE/dist" --corpus "$HERE/corpus" "$@"
