#!/usr/bin/env bash
set -euo pipefail

ITERATIONS="${ITERATIONS:-20}"
if [[ -z "${IGW_BIN:-}" ]]; then
  if [[ -x "./bin/igw" ]]; then
    IGW_BIN="./bin/igw"
  else
    IGW_BIN="igw"
  fi
else
  IGW_BIN="${IGW_BIN}"
fi
INCLUDE_SCAN="${INCLUDE_SCAN:-0}"
PROFILE="${IGW_PROFILE:-}"

if ! [[ "$ITERATIONS" =~ ^[0-9]+$ ]] || [[ "$ITERATIONS" -lt 1 ]]; then
  echo "ITERATIONS must be a positive integer" >&2
  exit 2
fi

profile_args=()
if [[ -n "$PROFILE" ]]; then
  profile_args=(--profile "$PROFILE")
fi

measure() {
  local label="$1"
  shift

  local -a samples=()
  local failures=0

  for ((i = 0; i < ITERATIONS; i++)); do
    local start_ms end_ms elapsed_ms
    start_ms="$(date +%s%3N)"
    if "$@" >/dev/null 2>&1; then
      end_ms="$(date +%s%3N)"
      elapsed_ms=$((end_ms - start_ms))
      samples+=("$elapsed_ms")
    else
      failures=$((failures + 1))
    fi
  done

  if [[ "${#samples[@]}" -eq 0 ]]; then
    printf '{"name":"%s","iterations":%d,"success":0,"failures":%d}' "$label" "$ITERATIONS" "$failures"
    return
  fi

  IFS=$'\n' read -r -d '' -a sorted < <(printf '%s\n' "${samples[@]}" | sort -n && printf '\0')
  local count="${#sorted[@]}"
  local p50_index=$(( (count - 1) / 2 ))
  local p95_index=$(( (count - 1) * 95 / 100 ))
  local min="${sorted[0]}"
  local p50="${sorted[$p50_index]}"
  local p95="${sorted[$p95_index]}"
  local max="${sorted[$((count - 1))]}"

  printf '{"name":"%s","iterations":%d,"success":%d,"failures":%d,"minMs":%d,"p50Ms":%d,"p95Ms":%d,"maxMs":%d}' \
    "$label" "$ITERATIONS" "$count" "$failures" "$min" "$p50" "$p95" "$max"
}

results=()
results+=("$(measure "call.gateway_info" "$IGW_BIN" call "${profile_args[@]}" --path /data/api/v1/gateway-info --json)")
results+=("$(measure "api.list" "$IGW_BIN" api list "${profile_args[@]}" --json)")

if [[ "$INCLUDE_SCAN" == "1" ]]; then
  results+=("$(measure "scan.projects" "$IGW_BIN" scan projects "${profile_args[@]}" --yes --json)")
fi

printf '{'
printf '"generatedAt":"%s",' "$(date -u +"%Y-%m-%dT%H:%M:%SZ")"
printf '"iterations":%d,' "$ITERATIONS"
printf '"results":['
for ((i = 0; i < ${#results[@]}; i++)); do
  if [[ "$i" -gt 0 ]]; then
    printf ','
  fi
  printf '%s' "${results[$i]}"
done
printf ']'
printf '}\n'
