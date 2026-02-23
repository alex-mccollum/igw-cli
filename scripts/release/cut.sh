#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=./lib.sh
source "${SCRIPT_DIR}/lib.sh"

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
release_require_semver_tag "$VERSION"
release_cd_repo_root
release_require_clean_worktree
release_require_changelog_heading "$VERSION"

echo "==> release dry-run"
./scripts/release/dry-run.sh "$VERSION"

HEAD_COMMIT="$(release_head_commit)"
if release_local_tag_exists "$VERSION"; then
  TAG_COMMIT="$(release_tag_commit "$VERSION")"
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
IGW_SKIP_PUSH_AUTH_CHECKS=1 ./scripts/release/checklist.sh "$VERSION"

echo "==> push branch and tag"
git push origin HEAD "refs/tags/${VERSION}"

echo "ok: release ${VERSION} pushed"
