#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat >&2 <<'EOF'
usage: ./scripts/release/cut.sh <version-tag>

Creates and pushes a release tag with local safety gates:
  1) changelog heading check
  2) release dry-run
  3) local tag creation
  4) release checklist
  5) push branch and tag
EOF
}

if [[ $# -ne 1 ]]; then
  usage
  exit 2
fi

VERSION="$1"
if [[ ! "$VERSION" =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
  echo "error: version must use semantic tag format vMAJOR.MINOR.PATCH" >&2
  exit 2
fi

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$ROOT_DIR"

if [[ -n "$(git status --porcelain)" ]]; then
  echo "error: working tree must be clean before cutting a release" >&2
  exit 1
fi

if ! grep -q "^## \[${VERSION}\]" CHANGELOG.md; then
  echo "error: CHANGELOG.md is missing release heading for ${VERSION}" >&2
  exit 1
fi

echo "==> release dry-run"
./scripts/release/dry-run.sh "$VERSION"

HEAD_COMMIT="$(git rev-parse HEAD)"
if git rev-parse -q --verify "refs/tags/${VERSION}^{commit}" >/dev/null; then
  TAG_COMMIT="$(git rev-list -n 1 "refs/tags/${VERSION}")"
  if [[ "$TAG_COMMIT" != "$HEAD_COMMIT" ]]; then
    echo "error: local tag ${VERSION} already exists at ${TAG_COMMIT}, expected HEAD ${HEAD_COMMIT}" >&2
    exit 1
  fi
  echo "==> local tag ${VERSION} already points to HEAD"
else
  echo "==> create local tag ${VERSION}"
  git tag "$VERSION"
fi

echo "==> release checklist"
./scripts/release/checklist.sh "$VERSION"

echo "==> push branch"
git push origin HEAD

echo "==> push tag"
git push origin "refs/tags/${VERSION}"

echo "ok: release ${VERSION} pushed"
