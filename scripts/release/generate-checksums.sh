#!/usr/bin/env bash
set -euo pipefail

if [[ $# -gt 1 ]]; then
  echo "usage: $0 [dist-dir]" >&2
  exit 2
fi

DIST_DIR="${1:-dist}"
OUT_PATH="${DIST_DIR}/checksums.txt"

if [[ ! -d "$DIST_DIR" ]]; then
  echo "error: dist directory not found: $DIST_DIR" >&2
  exit 1
fi

shopt -s nullglob
ARTIFACTS=("${DIST_DIR}"/*.tar.gz "${DIST_DIR}"/*.zip)
shopt -u nullglob
if [[ ${#ARTIFACTS[@]} -eq 0 ]]; then
  echo "error: no release artifacts found in ${DIST_DIR}" >&2
  exit 1
fi

BASENAMES=()
for artifact in "${ARTIFACTS[@]}"; do
  BASENAMES+=("$(basename "$artifact")")
done

if command -v sha256sum >/dev/null 2>&1; then
  (
    cd "$DIST_DIR"
    sha256sum "${BASENAMES[@]}" | sort > "checksums.txt"
  )
elif command -v shasum >/dev/null 2>&1; then
  (
    cd "$DIST_DIR"
    shasum -a 256 "${BASENAMES[@]}" | sort > "checksums.txt"
  )
else
  echo "error: no sha256 checksum tool found (need sha256sum or shasum)" >&2
  exit 1
fi

echo "ok: wrote checksums manifest to ${OUT_PATH}"
