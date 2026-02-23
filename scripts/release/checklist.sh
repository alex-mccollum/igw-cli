#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=./lib.sh
source "${SCRIPT_DIR}/lib.sh"

if [[ $# -ne 1 ]]; then
  echo "usage: $0 <version-tag>" >&2
  exit 2
fi

VERSION="$1"
release_require_semver_tag "$VERSION"
release_cd_repo_root
release_require_changelog_heading "$VERSION"
release_require_local_tag "$VERSION"
release_require_tag_points_to_head "$VERSION"

if ! git push --dry-run origin HEAD >/dev/null 2>&1; then
  echo "error: push auth check failed for origin HEAD" >&2
  exit 1
fi

if ! git push --dry-run origin "refs/tags/${VERSION}" >/dev/null 2>&1; then
  echo "error: push auth check failed for refs/tags/${VERSION}" >&2
  exit 1
fi

echo "ok: release checklist passed for ${VERSION}"
