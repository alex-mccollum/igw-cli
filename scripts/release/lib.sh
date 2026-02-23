#!/usr/bin/env bash

RELEASE_SEMVER_TAG_PATTERN='v[0-9]+\.[0-9]+\.[0-9]+'
RELEASE_LIB_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
RELEASE_ROOT_DIR="$(cd "${RELEASE_LIB_DIR}/../.." && pwd)"

release_cd_repo_root() {
  cd "${RELEASE_ROOT_DIR}"
}

release_require_semver_tag() {
  local version="${1:-}"
  if [[ ! "$version" =~ ^${RELEASE_SEMVER_TAG_PATTERN}$ ]]; then
    echo "error: version must use semantic tag format vMAJOR.MINOR.PATCH" >&2
    exit 2
  fi
}

release_require_clean_worktree() {
  if [[ -n "$(git status --porcelain)" ]]; then
    echo "error: working tree must be clean before cutting a release" >&2
    exit 1
  fi
}

release_require_changelog_heading() {
  local version="$1"
  if ! grep -q "^## \[${version}\]" CHANGELOG.md; then
    echo "error: CHANGELOG.md is missing release heading for ${version}" >&2
    exit 1
  fi
}

release_local_tag_exists() {
  local version="$1"
  git rev-parse -q --verify "refs/tags/${version}^{commit}" >/dev/null
}

release_require_local_tag() {
  local version="$1"
  if ! release_local_tag_exists "$version"; then
    echo "error: missing local tag ${version}" >&2
    exit 1
  fi
}

release_tag_commit() {
  local version="$1"
  git rev-list -n 1 "refs/tags/${version}"
}

release_head_commit() {
  git rev-parse HEAD
}

release_require_tag_points_to_head() {
  local version="$1"
  local tag_commit
  local head_commit

  tag_commit="$(release_tag_commit "$version")"
  head_commit="$(release_head_commit)"
  if [[ "$tag_commit" != "$head_commit" ]]; then
    echo "error: ${version} points to ${tag_commit}, expected HEAD ${head_commit}" >&2
    exit 1
  fi
}
