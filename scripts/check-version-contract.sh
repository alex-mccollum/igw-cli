#!/usr/bin/env bash
set -euo pipefail

if [[ $# -ne 2 ]]; then
  echo "usage: $0 <binary-path> <expected-version>" >&2
  exit 2
fi

BIN_PATH="$1"
EXPECTED_VERSION="$2"

if [[ ! -x "$BIN_PATH" ]]; then
  echo "error: binary not executable: $BIN_PATH" >&2
  exit 2
fi

OUTPUT="$("$BIN_PATH" version)"
EXPECTED_PREFIX="igw version ${EXPECTED_VERSION}"

if [[ "$OUTPUT" != "$EXPECTED_PREFIX"* ]]; then
  echo "error: version contract check failed" >&2
  echo "  expected prefix: $EXPECTED_PREFIX" >&2
  echo "  actual output:   $OUTPUT" >&2
  exit 1
fi

echo "ok: version contract passed (${OUTPUT})"
