#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "${SCRIPT_DIR}/.."
source "${IGW_PERF_THRESHOLDS_FILE:-${SCRIPT_DIR}/perf-thresholds.env}"

OUTPUT_FILE="$(mktemp)"
trap 'rm -f "$OUTPUT_FILE"' EXIT

go test ./internal/cli -run '^$' \
  -bench '^(BenchmarkCommandSchema|BenchmarkTypedRequest|BenchmarkCapturedCatalog|BenchmarkLoadedOperationLookup|BenchmarkCatalogRevalidation|BenchmarkStreamedArtifact32MiB)$' \
  -benchtime=3x -benchmem -count=1 >"$OUTPUT_FILE"
cat "$OUTPUT_FILE"

assert_metric() {
  local name="$1" metric="$2" limit="$3" observed
  if [[ ! "$limit" =~ ^[0-9]+$ || "$limit" == 0 ]]; then
    echo "error: threshold for $name $metric must be a positive integer" >&2
    exit 2
  fi
  observed="$(awk -v name="$name" -v metric="$metric" '
    $1 ~ ("^" name "-") {
      for (i = 1; i < NF; i++) if ($(i+1) == metric) { printf "%.0f\n", $i; exit }
    }' "$OUTPUT_FILE")"
  if [[ -z "$observed" ]]; then
    echo "error: missing benchmark metric $name $metric" >&2
    exit 1
  fi
  if (( observed > limit )); then
    echo "error: $name $metric exceeded budget: $observed > $limit" >&2
    exit 1
  fi
  echo "ok: $name $metric $observed <= $limit"
}

assert_metric BenchmarkCommandSchema ns/op "${IGW_PERF_MAX_COMMAND_SCHEMA_NS:-$MAX_COMMAND_SCHEMA_NS}"
assert_metric BenchmarkTypedRequest ns/op "${IGW_PERF_MAX_TYPED_REQUEST_NS:-$MAX_TYPED_REQUEST_NS}"
assert_metric BenchmarkCapturedCatalog ns/op "${IGW_PERF_MAX_CAPTURED_CATALOG_NS:-$MAX_CAPTURED_CATALOG_NS}"
assert_metric BenchmarkCapturedCatalog B/op "${IGW_PERF_MAX_CAPTURED_CATALOG_BYTES:-$MAX_CAPTURED_CATALOG_BYTES}"
assert_metric BenchmarkLoadedOperationLookup ns/op "${IGW_PERF_MAX_LOADED_LOOKUP_NS:-$MAX_LOADED_LOOKUP_NS}"
assert_metric BenchmarkLoadedOperationLookup B/op "${IGW_PERF_MAX_LOADED_LOOKUP_BYTES:-$MAX_LOADED_LOOKUP_BYTES}"
assert_metric BenchmarkCatalogRevalidation ns/op "${IGW_PERF_MAX_CATALOG_REVALIDATION_NS:-$MAX_CATALOG_REVALIDATION_NS}"
assert_metric BenchmarkCatalogRevalidation B/op "${IGW_PERF_MAX_CATALOG_REVALIDATION_BYTES:-$MAX_CATALOG_REVALIDATION_BYTES}"
assert_metric BenchmarkStreamedArtifact32MiB ns/op "${IGW_PERF_MAX_STREAMED_ARTIFACT_NS:-$MAX_STREAMED_ARTIFACT_NS}"
assert_metric BenchmarkStreamedArtifact32MiB B/op "${IGW_PERF_MAX_STREAMED_ARTIFACT_BYTES:-$MAX_STREAMED_ARTIFACT_BYTES}"
