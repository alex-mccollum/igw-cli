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

DIST_DIR="${DIST_DIR:-dist}"
mkdir -p "$DIST_DIR"

echo "==> docs checks"
./scripts/check-command-docs.sh
./scripts/lint-docs.sh

echo "==> tests"
go test ./...

echo "==> build release artifacts"
COMMIT="$(git rev-parse --short HEAD)"
DATE="$(date -u +%Y-%m-%d)"
TARGETS=(
  "linux/amd64"
  "linux/arm64"
  "darwin/amd64"
  "darwin/arm64"
  "windows/amd64"
  "windows/arm64"
)

if ! command -v zip >/dev/null 2>&1; then
  echo "warn: zip not found, skipping windows packaging targets in local dry-run"
  FILTERED=()
  for target in "${TARGETS[@]}"; do
    if [[ "${target%/*}" != "windows" ]]; then
      FILTERED+=("$target")
    fi
  done
  TARGETS=("${FILTERED[@]}")
fi

for target in "${TARGETS[@]}"; do
  GOOS="${target%/*}"
  GOARCH="${target#*/}"
  ./scripts/release/build-artifact.sh "$VERSION" "$COMMIT" "$DATE" "$GOOS" "$GOARCH" "$DIST_DIR" >/dev/null
done

echo "==> verify packaged linux/amd64 artifact"
./scripts/release/verify-artifact.sh "$VERSION" linux amd64 "$DIST_DIR"

echo "==> generate checksums manifest"
./scripts/release/generate-checksums.sh "$DIST_DIR"

echo "dry-run complete for ${VERSION}"
echo "artifacts:"
shopt -s nullglob
ARTIFACTS=(
  "${DIST_DIR}/igw_${VERSION}_"*.tar.gz
  "${DIST_DIR}/igw_${VERSION}_"*.zip
  "${DIST_DIR}/checksums.txt"
)
shopt -u nullglob
if [[ ${#ARTIFACTS[@]} -eq 0 ]]; then
  echo "error: no artifacts generated" >&2
  exit 1
fi
printf '%s\n' "${ARTIFACTS[@]}"
