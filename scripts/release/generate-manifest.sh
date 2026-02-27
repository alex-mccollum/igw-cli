#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=./lib.sh
source "${SCRIPT_DIR}/lib.sh"

usage() {
  echo "usage: $0 <version-tag> [dist-dir] [base-url]" >&2
}

json_escape() {
  printf '%s' "$1" | sed -e 's/\\/\\\\/g' -e 's/"/\\"/g'
}

if [[ $# -lt 1 || $# -gt 3 ]]; then
  usage
  exit 2
fi

if [[ "${1:-}" == "-h" || "${1:-}" == "--help" ]]; then
  usage
  exit 0
fi

VERSION="$1"
DIST_DIR="${2:-dist}"
BASE_URL="${3:-}"
OUT_PATH="${DIST_DIR}/release-manifest.json"
CHECKSUMS_PATH="${DIST_DIR}/checksums.txt"

release_require_semver_tag "$VERSION"

if [[ ! -d "$DIST_DIR" ]]; then
  echo "error: dist directory not found: $DIST_DIR" >&2
  exit 1
fi

if [[ ! -f "$CHECKSUMS_PATH" ]]; then
  echo "error: checksums manifest not found: $CHECKSUMS_PATH" >&2
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

{
  printf '{\n'
  printf '  "name": "igw",\n'
  printf '  "version": "%s",\n' "$(json_escape "$VERSION")"
  printf '  "checksumsFile": "checksums.txt",\n'
  printf '  "artifacts": [\n'
} > "$OUT_PATH"

for i in "${!ARTIFACTS[@]}"; do
  artifact_path="${ARTIFACTS[$i]}"
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

  sha256="$(awk -v n="$artifact_name" '$2==n {print $1; exit}' "$CHECKSUMS_PATH")"
  if [[ -z "$sha256" ]]; then
    echo "error: missing checksum entry for ${artifact_name} in ${CHECKSUMS_PATH}" >&2
    exit 1
  fi

  comma=","
  if [[ "$i" -eq "$((${#ARTIFACTS[@]} - 1))" ]]; then
    comma=""
  fi

  {
    printf '    {\n'
    printf '      "name": "%s",\n' "$(json_escape "$artifact_name")"
    printf '      "os": "%s",\n' "$(json_escape "$artifact_os")"
    printf '      "arch": "%s",\n' "$(json_escape "$artifact_arch")"
    printf '      "archive": "%s",\n' "$(json_escape "$artifact_archive")"
    printf '      "sha256": "%s"' "$(json_escape "$sha256")"
    if [[ -n "$BASE_URL" ]]; then
      printf ',\n      "url": "%s"' "$(json_escape "${BASE_URL}/${artifact_name}")"
    fi
    printf '\n    }%s\n' "$comma"
  } >> "$OUT_PATH"
done

{
  printf '  ]\n'
  printf '}\n'
} >> "$OUT_PATH"

echo "ok: wrote release manifest to ${OUT_PATH}"
