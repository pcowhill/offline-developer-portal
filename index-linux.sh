#!/bin/sh
# Phase 1: crawl the sites listed in sources.yaml into a local corpus.
# Extra arguments are passed through, e.g.: ./index-linux.sh --source my-docs
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

if [ ! -f "$HERE/sources.yaml" ]; then
  echo "ERROR: $HERE/sources.yaml not found." >&2
  echo "Create it from the example and edit it first:" >&2
  echo "  cp \"$HERE/sources.example.yaml\" \"$HERE/sources.yaml\"" >&2
  exit 1
fi

exec "$APP" index --config "$HERE/sources.yaml" --corpus "$HERE/corpus" "$@"
