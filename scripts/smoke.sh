#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

BIN_PATH="${IGW_BIN:-$ROOT_DIR/bin/igw}"
PROFILE="${IGW_PROFILE:-}"
TIMEOUT="${IGW_TIMEOUT:-8s}"
WAIT_INTERVAL="${IGW_WAIT_INTERVAL:-2s}"
WAIT_TIMEOUT="${IGW_WAIT_TIMEOUT:-30s}"
SPEC_FILE="${IGW_SPEC_FILE:-$ROOT_DIR/openapi.json}"
SKIP_WAIT="${IGW_SMOKE_SKIP_WAIT:-0}"
SKIP_API_SYNC="${IGW_SMOKE_SKIP_API_SYNC:-0}"
INCLUDE_MUTATIONS="${IGW_SMOKE_INCLUDE_MUTATIONS:-0}"

CURRENT_STEP="initialization"

on_err() {
  local code=$?
  echo "Smoke check failed at step: $CURRENT_STEP" >&2
  exit "$code"
}
trap on_err ERR

echo "Building igw binary..."
go build -o "$BIN_PATH" ./cmd/igw

run_cmd() {
  local step="$1"
  shift
  CURRENT_STEP="$step"
  echo "==> $CURRENT_STEP"
  echo "+ $*"
  "$@"
}

run_capture() {
  local __var_name="$1"
  shift
  local step="$1"
  shift
  local out
  CURRENT_STEP="$step"
  echo "==> $CURRENT_STEP"
  echo "+ $*"
  out="$("$@")"
  printf -v "$__var_name" "%s" "$out"
}

assert_non_empty() {
  local step="$1"
  local value="$2"
  CURRENT_STEP="$step"
  if [[ -z "$value" ]]; then
    echo "Expected non-empty output." >&2
    return 1
  fi
}

assert_single_line() {
  local step="$1"
  local value="$2"
  CURRENT_STEP="$step"
  if [[ "$value" == *$'\n'* ]]; then
    echo "Expected single-line output." >&2
    return 1
  fi
}

PROFILE_ARGS=()
if [[ -n "$PROFILE" ]]; then
  PROFILE_ARGS=(--profile "$PROFILE")
fi

run_cmd "config show" "$BIN_PATH" config show
run_cmd "doctor" "$BIN_PATH" doctor --timeout "$TIMEOUT" "${PROFILE_ARGS[@]}"
run_cmd "gateway info json" "$BIN_PATH" gateway info --timeout "$TIMEOUT" --json "${PROFILE_ARGS[@]}"
run_cmd "call gateway info json" "$BIN_PATH" call --method GET --path /data/api/v1/gateway-info --timeout "$TIMEOUT" --json "${PROFILE_ARGS[@]}"

run_capture raw_status "call select/raw response.status" "$BIN_PATH" call --method GET --path /data/api/v1/gateway-info --timeout "$TIMEOUT" --json --select response.status --raw "${PROFILE_ARGS[@]}"
assert_non_empty "call select/raw response.status validation" "$raw_status"

run_capture compact_subset "call select/select compact" "$BIN_PATH" call --method GET --path /data/api/v1/gateway-info --timeout "$TIMEOUT" --json --select ok --select response.status --compact "${PROFILE_ARGS[@]}"
assert_non_empty "call select/select compact validation" "$compact_subset"
assert_single_line "call select/select compact validation" "$compact_subset"

run_cmd "logs list json" "$BIN_PATH" logs list --timeout "$TIMEOUT" --query limit=5 --json "${PROFILE_ARGS[@]}"
run_cmd "diagnostics bundle status json" "$BIN_PATH" diagnostics bundle status --timeout "$TIMEOUT" --json "${PROFILE_ARGS[@]}"
run_cmd "restart tasks json" "$BIN_PATH" restart tasks --timeout "$TIMEOUT" --json "${PROFILE_ARGS[@]}"
if [[ "$INCLUDE_MUTATIONS" == "1" ]]; then
  run_cmd "scan config json" "$BIN_PATH" scan config --timeout "$TIMEOUT" --yes --json "${PROFILE_ARGS[@]}"
else
  echo "Skipping mutating wrapper checks (IGW_SMOKE_INCLUDE_MUTATIONS=0)."
fi

if [[ "$SKIP_WAIT" == "1" ]]; then
  echo "Skipping wait checks (IGW_SMOKE_SKIP_WAIT=1)."
else
  run_capture wait_gateway_ok "wait gateway select/raw ok" "$BIN_PATH" wait gateway --interval "$WAIT_INTERVAL" --wait-timeout "$WAIT_TIMEOUT" --json --select ok --raw "${PROFILE_ARGS[@]}"
  assert_non_empty "wait gateway select/raw ok validation" "$wait_gateway_ok"
fi

if [[ "$SKIP_API_SYNC" == "1" ]]; then
  echo "Skipping API sync checks (IGW_SMOKE_SKIP_API_SYNC=1)."
else
  run_cmd "api sync json" "$BIN_PATH" api sync --timeout "$TIMEOUT" --json "${PROFILE_ARGS[@]}"
  run_capture op_count "api refresh select/raw operationCount" "$BIN_PATH" api refresh --timeout "$TIMEOUT" --json --select operationCount --raw "${PROFILE_ARGS[@]}"
  assert_non_empty "api refresh select/raw operationCount validation" "$op_count"
fi

if [[ -f "$SPEC_FILE" ]]; then
  run_cmd "api search from spec" "$BIN_PATH" api search --spec-file "$SPEC_FILE" --query gateway-info
  run_cmd "call op gatewayInfo from spec" "$BIN_PATH" call --spec-file "$SPEC_FILE" --op gatewayInfo --timeout "$TIMEOUT" --json "${PROFILE_ARGS[@]}"
fi

echo "Smoke checks passed."
