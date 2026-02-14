#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

BIN_PATH="${IGW_BIN:-$ROOT_DIR/bin/igw}"
PROFILE="${IGW_PROFILE:-}"
TIMEOUT="${IGW_TIMEOUT:-8s}"
SPEC_FILE="${IGW_SPEC_FILE:-$ROOT_DIR/openapi.json}"

echo "Building igw binary..."
go build -o "$BIN_PATH" ./cmd/igw

run_cmd() {
  echo "+ $*"
  "$@"
}

PROFILE_ARGS=()
if [[ -n "$PROFILE" ]]; then
  PROFILE_ARGS=(--profile "$PROFILE")
fi

run_cmd "$BIN_PATH" config show
run_cmd "$BIN_PATH" doctor --timeout "$TIMEOUT" "${PROFILE_ARGS[@]}"
run_cmd "$BIN_PATH" gateway info --timeout "$TIMEOUT" --json "${PROFILE_ARGS[@]}"
run_cmd "$BIN_PATH" call --method GET --path /data/api/v1/gateway-info --timeout "$TIMEOUT" --json "${PROFILE_ARGS[@]}"
run_cmd "$BIN_PATH" logs list --timeout "$TIMEOUT" --query limit=5 --json "${PROFILE_ARGS[@]}"
run_cmd "$BIN_PATH" diagnostics bundle status --timeout "$TIMEOUT" --json "${PROFILE_ARGS[@]}"
run_cmd "$BIN_PATH" restart tasks --timeout "$TIMEOUT" --json "${PROFILE_ARGS[@]}"

if [[ -f "$SPEC_FILE" ]]; then
  run_cmd "$BIN_PATH" api search --spec-file "$SPEC_FILE" --query gateway-info
  run_cmd "$BIN_PATH" call --spec-file "$SPEC_FILE" --op gatewayInfo --timeout "$TIMEOUT" --json "${PROFILE_ARGS[@]}"
fi

echo "Smoke checks passed."
