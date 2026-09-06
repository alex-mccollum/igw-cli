#!/usr/bin/env bash
set -euo pipefail
ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"
BIN_PATH="${IGW_BIN:-$ROOT_DIR/bin/igw}"
if [[ "${IGW_SMOKE_INCLUDE_MUTATIONS:-0}" != 0 ]]; then
  echo "error: mutating smoke mode is removed; use guarded Gateway qualification" >&2
  exit 2
fi
go build -o "$BIN_PATH" ./cmd/igw
args=(--binary "$BIN_PATH")
if [[ "${IGW_SMOKE_LIVE:-0}" == 1 ]]; then args+=(--live); fi
if [[ -n "${IGW_PROFILE:-}" ]]; then args+=(--profile "$IGW_PROFILE"); fi
python3 "$ROOT_DIR/scripts/smoke.py" "${args[@]}"
