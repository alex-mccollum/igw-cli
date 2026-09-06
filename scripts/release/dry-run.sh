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
bash ./scripts/check-command-docs.sh
bash ./scripts/lint-docs.sh

echo "==> tests"
go test ./...

echo "==> build release artifacts"
COMMIT="$(git rev-parse --short HEAD)"
if [[ -n "$(git status --porcelain)" ]]; then COMMIT="${COMMIT}-dirty"; fi
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
  echo "error: zip is required to qualify all six artifact targets" >&2
  exit 2
fi

for target in "${TARGETS[@]}"; do
  target_goos="${target%/*}"
  target_goarch="${target#*/}"
  bash ./scripts/release/build-artifact.sh "$VERSION" "$COMMIT" "$DATE" "$target_goos" "$target_goarch" "$DIST_DIR" >/dev/null
done

echo "==> verify packaged linux/amd64 artifact"
bash ./scripts/release/verify-artifact.sh "$VERSION" linux amd64 "$DIST_DIR"

echo "==> generate latest aliases"
bash ./scripts/release/generate-latest-aliases.sh "$VERSION" "$DIST_DIR"

echo "==> generate checksums manifest"
bash ./scripts/release/generate-checksums.sh "$DIST_DIR"

echo "==> generate release manifest"
bash ./scripts/release/generate-manifest.sh "$VERSION" "$DIST_DIR"

echo "dry-run complete for ${VERSION}"
echo "artifacts:"
shopt -s nullglob
ARTIFACTS=(
  "${DIST_DIR}/igw_${VERSION}_"*.tar.gz
  "${DIST_DIR}/igw_${VERSION}_"*.zip
  "${DIST_DIR}/checksums.txt"
  "${DIST_DIR}/release-manifest.json"
)
shopt -u nullglob
if [[ ${#ARTIFACTS[@]} -eq 0 ]]; then
  echo "error: no artifacts generated" >&2
  exit 1
fi
printf '%s\n' "${ARTIFACTS[@]}"
