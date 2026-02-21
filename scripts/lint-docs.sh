#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

DOC_FILES=("README.md")
while IFS= read -r file; do
  DOC_FILES+=("$file")
done < <(find docs -type f -name '*.md' | sort)

if [[ ${#DOC_FILES[@]} -eq 0 ]]; then
  echo "error: no docs files found"
  exit 1
fi

check_absent_pattern() {
  local pattern="$1"
  local message="$2"
  if rg -n --fixed-strings "$pattern" "${DOC_FILES[@]}" >/dev/null; then
    echo "error: $message"
    rg -n --fixed-strings "$pattern" "${DOC_FILES[@]}"
    exit 1
  fi
}

check_absent_regex() {
  local pattern="$1"
  local message="$2"
  if rg -n "$pattern" "${DOC_FILES[@]}" >/dev/null; then
    echo "error: $message"
    rg -n "$pattern" "${DOC_FILES[@]}"
    exit 1
  fi
}

# Placeholder metadata and repo scaffolding.
check_absent_pattern "yourusername" "placeholder repo owner detected in docs"
check_absent_pattern "originalowner" "placeholder upstream owner detected in docs"
check_absent_pattern "Your Name" "placeholder author value detected in docs"
check_absent_pattern "your.email@example.com" "placeholder author email detected in docs"

# Hardcoded local absolute paths should not appear in shared docs.
check_absent_regex '(^|[^A-Za-z0-9_])(\/Users\/|\/home\/)[^[:space:])"]+' "hardcoded local absolute path detected in docs"
check_absent_regex '([A-Za-z]:\\Users\\)[^[:space:])"]+' "hardcoded Windows user path detected in docs"

# Catch common default-port drift for gateway examples.
check_absent_pattern "localhost:9088" "unexpected localhost default port detected (expected 8088 unless explicitly explained)"
check_absent_pattern "127.0.0.1:9088" "unexpected loopback default port detected (expected 8088 unless explicitly explained)"

# Keep test-layout references honest if those paths are mentioned.
if rg -n 'tests/(unit|integration)' "${DOC_FILES[@]}" >/dev/null; then
  if [[ ! -d tests/unit || ! -d tests/integration ]]; then
    echo "error: docs reference tests/unit or tests/integration, but those directories are missing"
    rg -n 'tests/(unit|integration)' "${DOC_FILES[@]}"
    exit 1
  fi
fi

# Ensure referenced docs pages exist.
missing_refs=0
while IFS= read -r ref; do
  if [[ ! -f "$ref" ]]; then
    if [[ $missing_refs -eq 0 ]]; then
      echo "error: docs reference missing docs/*.md files:"
    fi
    echo "  - $ref"
    missing_refs=1
  fi
done < <(rg -o --no-filename 'docs/[A-Za-z0-9._/-]+\.md' "${DOC_FILES[@]}" | sort -u)
if [[ $missing_refs -ne 0 ]]; then
  exit 1
fi

echo "ok: docs lint checks passed"
