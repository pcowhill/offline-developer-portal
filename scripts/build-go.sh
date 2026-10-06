#!/bin/sh
# Builds the offline-docs binary for this machine into bin/offline-docs.
# Requires Go 1.25.1 or newer. Dependencies are vendored (vendor/), so no
# network access is needed.
#
#   scripts/build-go.sh                 # native build
#   GOOS=windows scripts/build-go.sh    # cross-compile (bin/offline-docs.exe)
set -eu
ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
cd "$ROOT"
VERSION=${VERSION:-$(git describe --tags --always --dirty 2>/dev/null || echo dev)}
EXT=""
if [ "$(go env GOOS)" = "windows" ]; then EXT=".exe"; fi
mkdir -p bin
CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X main.version=$VERSION" -o "bin/offline-docs$EXT" ./cmd/offline-docs
echo "Built bin/offline-docs$EXT ($VERSION)"
