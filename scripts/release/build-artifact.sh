#!/usr/bin/env bash
set -euo pipefail

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

NAME="igw_${VERSION}_${TARGET_GOOS}_${TARGET_GOARCH}"
OUT_DIR="${DIST_DIR}/${NAME}"
BIN_NAME="igw"
if [[ "$TARGET_GOOS" == "windows" ]]; then
  BIN_NAME="igw.exe"
fi

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
    zip -rq "${NAME}.zip" "${NAME}"
  )
  echo "${DIST_DIR}/${NAME}.zip"
else
  tar -C "$DIST_DIR" -czf "${DIST_DIR}/${NAME}.tar.gz" "${NAME}"
  echo "${DIST_DIR}/${NAME}.tar.gz"
fi
