#!/usr/bin/env bash
set -euo pipefail
ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
if [[ "${INCLUDE_SCAN:-0}" != 0 ]]; then
  echo "error: INCLUDE_SCAN is removed; performance checks are read-only" >&2
  exit 2
fi
args=(--binary "${IGW_BIN:-$ROOT_DIR/bin/igw}" --iterations "${ITERATIONS:-5}")
if [[ "${IGW_PERF_LIVE:-0}" == 1 ]]; then args+=(--live); fi
if [[ -n "${IGW_PROFILE:-}" ]]; then args+=(--profile "$IGW_PROFILE"); fi
python3 "$ROOT_DIR/scripts/perf-baseline.py" "${args[@]}"
