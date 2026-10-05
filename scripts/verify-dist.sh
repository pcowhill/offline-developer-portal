#!/bin/sh
# Rebuilds the frontend and fails if the result differs from the committed
# dist/ directory. Run after `npm ci`.
set -eu
ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
cd "$ROOT"
npm run build
if [ -n "$(git status --porcelain --untracked-files=all -- dist)" ]; then
  echo "ERROR: dist/ is stale. Run 'npm run build' and commit the updated dist/ directory." >&2
  git status --porcelain --untracked-files=all -- dist >&2
  git --no-pager diff --stat -- dist >&2
  exit 1
fi
echo "dist/ is up to date."
