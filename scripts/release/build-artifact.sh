#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=./asset-lib.sh
source "${SCRIPT_DIR}/asset-lib.sh"

if [[ $# -lt 5 || $# -gt 6 ]]; then
  echo "usage: $0 <version> <commit> <date> <goos> <goarch> [dist-dir]" >&2
  exit 2
fi

VERSION="$1"
COMMIT="$2"
DATE="$3"
TARGET_GOOS="$4"
TARGET_GOARCH="$5"
DIST_DIR="${6:-dist}"

DIR_NAME="$(release_extract_dir_name "$VERSION" "$TARGET_GOOS" "$TARGET_GOARCH")"
ARCHIVE_NAME="$(release_versioned_asset_name "$VERSION" "$TARGET_GOOS" "$TARGET_GOARCH")"
BIN_NAME="igw"
if [[ "$TARGET_GOOS" == "windows" ]]; then
  BIN_NAME="igw.exe"
fi

OUT_DIR="${DIST_DIR}/${DIR_NAME}"

mkdir -p "$OUT_DIR"

GOOS="$TARGET_GOOS" GOARCH="$TARGET_GOARCH" CGO_ENABLED=0 go build -trimpath \
  -ldflags "-s -w -X github.com/alex-mccollum/igw-cli/internal/buildinfo.Version=${VERSION} -X github.com/alex-mccollum/igw-cli/internal/buildinfo.Commit=${COMMIT} -X github.com/alex-mccollum/igw-cli/internal/buildinfo.Date=${DATE}" \
  -o "${OUT_DIR}/${BIN_NAME}" ./cmd/igw

cp LICENSE README.md "$OUT_DIR/"

if [[ "$TARGET_GOOS" == "windows" ]]; then
  if ! command -v zip >/dev/null 2>&1; then
    echo "error: zip command is required to package windows artifacts" >&2
    exit 1
  fi
  (
    cd "$DIST_DIR"
    zip -rq "${ARCHIVE_NAME}" "${DIR_NAME}"
  )
  echo "${DIST_DIR}/${ARCHIVE_NAME}"
else
  tar -C "$DIST_DIR" -czf "${DIST_DIR}/${ARCHIVE_NAME}" "${DIR_NAME}"
  echo "${DIST_DIR}/${ARCHIVE_NAME}"
fi
