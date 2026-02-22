#!/usr/bin/env bash
set -euo pipefail

if [[ $# -ne 1 ]]; then
  echo "usage: $0 <version-tag>" >&2
  exit 2
fi

VERSION="$1"
if [[ ! "$VERSION" =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
  echo "error: version must use semantic tag format vMAJOR.MINOR.PATCH" >&2
  exit 2
fi

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$ROOT_DIR"

if ! grep -q "^## \[${VERSION}\]" CHANGELOG.md; then
  echo "error: CHANGELOG.md is missing release heading for ${VERSION}" >&2
  exit 1
fi

if ! git rev-parse -q --verify "refs/tags/${VERSION}^{commit}" >/dev/null; then
  echo "error: missing local tag ${VERSION}" >&2
  exit 1
fi

TAG_COMMIT="$(git rev-list -n 1 "refs/tags/${VERSION}")"
HEAD_COMMIT="$(git rev-parse HEAD)"
if [[ "${TAG_COMMIT}" != "${HEAD_COMMIT}" ]]; then
  echo "error: ${VERSION} points to ${TAG_COMMIT}, expected HEAD ${HEAD_COMMIT}" >&2
  exit 1
fi

if ! git push --dry-run origin HEAD >/dev/null 2>&1; then
  echo "error: push auth check failed for origin HEAD" >&2
  exit 1
fi

if ! git push --dry-run origin "refs/tags/${VERSION}" >/dev/null 2>&1; then
  echo "error: push auth check failed for refs/tags/${VERSION}" >&2
  exit 1
fi

echo "ok: release checklist passed for ${VERSION}"
