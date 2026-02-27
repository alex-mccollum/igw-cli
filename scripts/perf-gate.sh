#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
cd "${REPO_ROOT}"

BENCH_PATTERN='^(BenchmarkExecuteCallCore|BenchmarkRunCallBatchSingleItem|BenchmarkHandleRPCCall)$'

THRESHOLDS_FILE="${IGW_PERF_THRESHOLDS_FILE:-${SCRIPT_DIR}/perf-thresholds.env}"
if [[ -f "${THRESHOLDS_FILE}" ]]; then
  # shellcheck disable=SC1090
  source "${THRESHOLDS_FILE}"
fi

MAX_EXECUTE_CALL_CORE_NS="${IGW_PERF_MAX_EXECUTE_CALL_CORE_NS:-${MAX_EXECUTE_CALL_CORE_NS:-1500000}}"
MAX_CALL_BATCH_SINGLE_ITEM_NS="${IGW_PERF_MAX_CALL_BATCH_SINGLE_ITEM_NS:-${MAX_CALL_BATCH_SINGLE_ITEM_NS:-5000000}}"
MAX_HANDLE_RPC_CALL_NS="${IGW_PERF_MAX_HANDLE_RPC_CALL_NS:-${MAX_HANDLE_RPC_CALL_NS:-2500000}}"

OUTPUT_FILE="$(mktemp)"
trap 'rm -f "${OUTPUT_FILE}"' EXIT

echo "==> running perf benchmarks"
go test ./internal/cli -run '^$' -bench "${BENCH_PATTERN}" -benchmem -count 1 >"${OUTPUT_FILE}"
cat "${OUTPUT_FILE}"

extract_ns_op() {
  local benchmark_name="$1"
  local value
  value="$(awk -v name="${benchmark_name}" '
    $1 ~ ("^" name "-") {
      for (i = 1; i <= NF; i++) {
        if ($(i + 1) == "ns/op") {
          printf "%.0f\n", $(i)
          exit
        }
      }
    }
  ' "${OUTPUT_FILE}")"
  if [[ -z "${value}" ]]; then
    echo "error: failed to parse ns/op for ${benchmark_name}" >&2
    exit 1
  fi
  echo "${value}"
}

assert_threshold() {
  local benchmark_name="$1"
  local threshold_ns="$2"
  local ns_op
  ns_op="$(extract_ns_op "${benchmark_name}")"
  if (( ns_op > threshold_ns )); then
    echo "error: ${benchmark_name} exceeded threshold: ${ns_op}ns > ${threshold_ns}ns" >&2
    exit 1
  fi
  echo "ok: ${benchmark_name} ${ns_op}ns <= ${threshold_ns}ns"
}

assert_threshold "BenchmarkExecuteCallCore" "${MAX_EXECUTE_CALL_CORE_NS}"
assert_threshold "BenchmarkRunCallBatchSingleItem" "${MAX_CALL_BATCH_SINGLE_ITEM_NS}"
assert_threshold "BenchmarkHandleRPCCall" "${MAX_HANDLE_RPC_CALL_NS}"
