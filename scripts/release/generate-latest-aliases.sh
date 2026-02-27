#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=./lib.sh
source "${SCRIPT_DIR}/lib.sh"
# shellcheck source=./asset-lib.sh
source "${SCRIPT_DIR}/asset-lib.sh"

usage() {
  echo "usage: $0 <version-tag> [dist-dir]" >&2
}

if [[ $# -lt 1 || $# -gt 2 ]]; then
  usage
  exit 2
fi

if [[ "${1:-}" == "-h" || "${1:-}" == "--help" ]]; then
  usage
  exit 0
fi

VERSION="$1"
DIST_DIR="${2:-dist}"

release_require_semver_tag "$VERSION"

if [[ ! -d "$DIST_DIR" ]]; then
  echo "error: dist directory not found: $DIST_DIR" >&2
  exit 1
fi

shopt -s nullglob
ARTIFACTS=("${DIST_DIR}/igw_${VERSION}_"*.tar.gz "${DIST_DIR}/igw_${VERSION}_"*.zip)
shopt -u nullglob
if [[ ${#ARTIFACTS[@]} -eq 0 ]]; then
  echo "error: no release artifacts found for ${VERSION} in ${DIST_DIR}" >&2
  exit 1
fi

IFS=$'\n' ARTIFACTS=($(printf '%s\n' "${ARTIFACTS[@]}" | sort))
unset IFS

for artifact_path in "${ARTIFACTS[@]}"; do
  artifact_name="$(basename "$artifact_path")"
  if [[ "$artifact_name" =~ ^igw_(v[0-9]+\.[0-9]+\.[0-9]+)_([a-z0-9]+)_([a-z0-9]+)\.(tar\.gz|zip)$ ]]; then
    artifact_version="${BASH_REMATCH[1]}"
    artifact_os="${BASH_REMATCH[2]}"
    artifact_arch="${BASH_REMATCH[3]}"
    artifact_archive="${BASH_REMATCH[4]}"
  else
    echo "error: artifact name does not match expected pattern: ${artifact_name}" >&2
    exit 1
  fi

  if [[ "$artifact_version" != "$VERSION" ]]; then
    echo "error: artifact version mismatch in ${artifact_name}" >&2
    exit 1
  fi

  alias_name="$(release_latest_alias_name "$artifact_os" "$artifact_arch")"
  if [[ "${alias_name##*.}" != "${artifact_archive##*.}" ]]; then
    echo "error: unexpected archive extension for ${artifact_name}" >&2
    exit 1
  fi
  cp -f "$artifact_path" "${DIST_DIR}/${alias_name}"
  echo "ok: wrote ${DIST_DIR}/${alias_name}"
done
