#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
README="$ROOT_DIR/README.md"
COMMANDS_DOC="$ROOT_DIR/docs/commands.md"

if [[ ! -f "$README" ]]; then
  echo "error: missing README.md"
  exit 1
fi

if [[ ! -f "$COMMANDS_DOC" ]]; then
  echo "error: missing docs/commands.md"
  exit 1
fi

if ! grep -q "canonical command example reference" "$COMMANDS_DOC"; then
  echo "error: docs/commands.md must declare canonical command example reference"
  exit 1
fi

if ! grep -q 'docs/commands.md' "$README"; then
  echo "error: README.md must point users to docs/commands.md for full command examples"
  exit 1
fi

README_CMD_COUNT="$(rg -N '^\s*igw(\s|$)' "$README" | wc -l | tr -d ' ')"
MAX_README_CMDS=12
if (( README_CMD_COUNT > MAX_README_CMDS )); then
  echo "error: README.md has $README_CMD_COUNT igw commands; keep onboarding examples at <= $MAX_README_CMDS"
  exit 1
fi

extract_shapes() {
  local file="$1"
  rg -N '^\s*igw(\s|$)' "$file" | awk '
  {
    first = $2
    second = $3
    if (first == "") next
    shape = first
    if (second != "" && second !~ /^--/ && second != "\\") shape = shape " " second
    print shape
  }' | sort -u
}

tmp_readme_shapes="$(mktemp)"
tmp_doc_shapes="$(mktemp)"
trap 'rm -f "$tmp_readme_shapes" "$tmp_doc_shapes"' EXIT

extract_shapes "$README" > "$tmp_readme_shapes"
extract_shapes "$COMMANDS_DOC" > "$tmp_doc_shapes"

missing_shapes="$(comm -23 "$tmp_readme_shapes" "$tmp_doc_shapes" || true)"
if [[ -n "$missing_shapes" ]]; then
  echo "error: README command families missing from docs/commands.md:"
  echo "$missing_shapes" | sed 's/^/  - /'
  exit 1
fi

echo "ok: command docs consistency checks passed"
