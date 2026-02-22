#!/usr/bin/env bash
set -euo pipefail

if [[ $# -lt 3 || $# -gt 4 ]]; then
  echo "usage: $0 <version> <goos> <goarch> [dist-dir]" >&2
  exit 2
fi

VERSION="$1"
TARGET_GOOS="$2"
TARGET_GOARCH="$3"
DIST_DIR="${4:-dist}"

NAME="igw_${VERSION}_${TARGET_GOOS}_${TARGET_GOARCH}"

if [[ "$TARGET_GOOS" != "linux" || "$TARGET_GOARCH" != "amd64" ]]; then
  echo "skip: post-package executable verification is only run for linux/amd64 on this runner"
  exit 0
fi

ARCHIVE_PATH="${DIST_DIR}/${NAME}.tar.gz"
if [[ ! -f "$ARCHIVE_PATH" ]]; then
  echo "error: archive not found: $ARCHIVE_PATH" >&2
  exit 1
fi

TMP_DIR="$(mktemp -d)"
trap 'rm -rf "$TMP_DIR"' EXIT

tar -C "$TMP_DIR" -xzf "$ARCHIVE_PATH"
BIN_PATH="${TMP_DIR}/${NAME}/igw"

./scripts/check-version-contract.sh "$BIN_PATH" "$VERSION"
"$BIN_PATH" help >/dev/null 2>&1

echo "ok: packaged artifact smoke check passed (${ARCHIVE_PATH})"
