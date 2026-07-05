#!/usr/bin/env bash
# Bump the f9 version: write VERSION, commit, and create an annotated tag.
# Usage: make bump V=1.2.3   (or: bash scripts/bump.sh 1.2.3)
set -euo pipefail

NEW="${1:-}"
if [ -z "$NEW" ]; then
  echo "usage: make bump V=<version>   (e.g. make bump V=1.2.3)" >&2
  exit 1
fi
if ! printf '%s' "$NEW" | grep -Eq '^[0-9]+\.[0-9]+\.[0-9]+$'; then
  echo "error: version must be MAJOR.MINOR.PATCH (e.g. 1.2.3)" >&2
  exit 1
fi

root="$(git rev-parse --show-toplevel)"
cd "$root"

if git rev-parse -q --verify "refs/tags/v$NEW" >/dev/null; then
  echo "error: tag v$NEW already exists" >&2
  exit 1
fi

printf '%s\n' "$NEW" > VERSION
git add VERSION
git commit -m "release: v$NEW" -- VERSION
git tag -a "v$NEW" -m "f9 v$NEW"

echo "bumped to v$NEW (committed + annotated tag v$NEW)."
echo "push to trigger the release workflow:"
echo "  git push --follow-tags"
