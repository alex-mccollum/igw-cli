#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat >&2 <<'EOF'
usage: scripts/install.sh [--version vX.Y.Z] [--dir PATH] [--repo OWNER/REPO]

Installs igw from GitHub release artifacts (Linux/macOS).

Defaults:
  --dir  $HOME/.local/bin
  --repo alex-mccollum/igw-cli
  --version latest release tag
EOF
}

require_cmd() {
  local cmd="$1"
  if ! command -v "$cmd" >/dev/null 2>&1; then
    echo "error: required command not found: $cmd" >&2
    exit 1
  fi
}

http_get_to_file() {
  local url="$1"
  local out="$2"
  if command -v curl >/dev/null 2>&1; then
    curl -fsSL "$url" -o "$out"
    return
  fi
  if command -v wget >/dev/null 2>&1; then
    wget -qO "$out" "$url"
    return
  fi
  echo "error: curl or wget is required" >&2
  exit 1
}

http_get_text() {
  local url="$1"
  if command -v curl >/dev/null 2>&1; then
    curl -fsSL "$url"
    return
  fi
  if command -v wget >/dev/null 2>&1; then
    wget -qO- "$url"
    return
  fi
  echo "error: curl or wget is required" >&2
  exit 1
}

detect_os() {
  case "$(uname -s)" in
    Linux)
      echo "linux"
      ;;
    Darwin)
      echo "darwin"
      ;;
    *)
      echo "unsupported"
      ;;
  esac
}

detect_arch() {
  case "$(uname -m)" in
    x86_64|amd64)
      echo "amd64"
      ;;
    arm64|aarch64)
      echo "arm64"
      ;;
    *)
      echo "unsupported"
      ;;
  esac
}

sha256_file() {
  local file="$1"
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$file" | awk '{print $1}'
    return
  fi
  if command -v shasum >/dev/null 2>&1; then
    shasum -a 256 "$file" | awk '{print $1}'
    return
  fi
  echo "error: sha256sum or shasum is required" >&2
  exit 1
}

VERSION=""
REPO="alex-mccollum/igw-cli"
INSTALL_DIR="${HOME}/.local/bin"

while [[ $# -gt 0 ]]; do
  case "$1" in
    --version)
      [[ $# -ge 2 ]] || { usage; exit 2; }
      VERSION="$2"
      shift 2
      ;;
    --dir)
      [[ $# -ge 2 ]] || { usage; exit 2; }
      INSTALL_DIR="$2"
      shift 2
      ;;
    --repo)
      [[ $# -ge 2 ]] || { usage; exit 2; }
      REPO="$2"
      shift 2
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "error: unknown argument: $1" >&2
      usage
      exit 2
      ;;
  esac
done

OS="$(detect_os)"
ARCH="$(detect_arch)"
if [[ "$OS" == "unsupported" ]]; then
  echo "error: unsupported OS for this installer (supported: Linux, macOS)" >&2
  echo "hint: on Windows, use scripts/install.ps1" >&2
  exit 1
fi
if [[ "$ARCH" == "unsupported" ]]; then
  echo "error: unsupported CPU architecture: $(uname -m)" >&2
  exit 1
fi

if [[ -z "$VERSION" ]]; then
  latest_url="https://api.github.com/repos/${REPO}/releases/latest"
  latest_json="$(http_get_text "$latest_url")"
  VERSION="$(printf '%s\n' "$latest_json" | sed -n 's/.*"tag_name":[[:space:]]*"\([^"]*\)".*/\1/p' | head -n 1)"
  if [[ -z "$VERSION" ]]; then
    echo "error: failed to resolve latest release tag for ${REPO}" >&2
    exit 1
  fi
fi

if [[ ! "$VERSION" =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
  echo "error: version must use semantic tag format vMAJOR.MINOR.PATCH" >&2
  exit 2
fi

ARCHIVE="igw_${VERSION}_${OS}_${ARCH}.tar.gz"
BASE_URL="https://github.com/${REPO}/releases/download/${VERSION}"
ARCHIVE_URL="${BASE_URL}/${ARCHIVE}"
CHECKSUMS_URL="${BASE_URL}/checksums.txt"

require_cmd tar

TMP_DIR="$(mktemp -d)"
trap 'rm -rf "$TMP_DIR"' EXIT

ARCHIVE_PATH="${TMP_DIR}/${ARCHIVE}"
CHECKSUMS_PATH="${TMP_DIR}/checksums.txt"

echo "==> downloading ${ARCHIVE_URL}"
http_get_to_file "$ARCHIVE_URL" "$ARCHIVE_PATH"

echo "==> downloading ${CHECKSUMS_URL}"
http_get_to_file "$CHECKSUMS_URL" "$CHECKSUMS_PATH"

expected_sha="$(awk -v n="$ARCHIVE" '$2==n {print $1; exit}' "$CHECKSUMS_PATH")"
if [[ -z "$expected_sha" ]]; then
  echo "error: checksum for ${ARCHIVE} not found in checksums.txt" >&2
  exit 1
fi
actual_sha="$(sha256_file "$ARCHIVE_PATH")"
if [[ "$actual_sha" != "$expected_sha" ]]; then
  echo "error: checksum mismatch for ${ARCHIVE}" >&2
  echo "expected: ${expected_sha}" >&2
  echo "actual:   ${actual_sha}" >&2
  exit 1
fi

echo "==> extracting ${ARCHIVE}"
tar -C "$TMP_DIR" -xzf "$ARCHIVE_PATH"

BIN_PATH="${TMP_DIR}/igw_${VERSION}_${OS}_${ARCH}/igw"
if [[ ! -f "$BIN_PATH" ]]; then
  echo "error: extracted binary not found: ${BIN_PATH}" >&2
  exit 1
fi

mkdir -p "$INSTALL_DIR"
if command -v install >/dev/null 2>&1; then
  install -m 0755 "$BIN_PATH" "${INSTALL_DIR}/igw"
else
  cp "$BIN_PATH" "${INSTALL_DIR}/igw"
  chmod 0755 "${INSTALL_DIR}/igw"
fi

echo "ok: installed igw ${VERSION} to ${INSTALL_DIR}/igw"
echo "verify: ${INSTALL_DIR}/igw version"
