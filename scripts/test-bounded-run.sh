#!/usr/bin/env bash
# Low-load integration checks: no Go build, Docker invocation, or OOM stress.
set -euo pipefail
root=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)
runner="$root/scripts/bounded-run.sh"
scratch=$(mktemp -d)
trap 'rm -rf -- "$scratch"' EXIT

expect_status() {
  local expected=$1 actual=0
  shift
  "$@" >"$scratch/stdout" 2>"$scratch/stderr" || actual=$?
  if [[ $actual != "$expected" ]]; then
    printf 'expected exit %s, got %s\n' "$expected" "$actual" >&2
    cat "$scratch/stderr" >&2
    exit 1
  fi
}

expect_status 0 bash "$runner" --check
expect_status 2 bash "$runner" --inside true
expect_status 2 bash "$runner" --timeout 0 -- true
expect_status 2 bash "$runner" --timeout 601 -- true
expect_status 23 bash "$runner" -- bash -c 'exit 23'

# Literal shell metacharacters, whitespace, environment, and CWD survive.
literal='two words $HOME $(false) `false`'
expect_status 0 bash "$runner" -- bash -c '
  set -e
  [[ $PWD == "$1" && $2 == "two words \$HOME \$(false) \`false\`" ]]
  [[ $GOMAXPROCS == 2 && $GOMEMLIMIT == 768MiB && $GOFLAGS == *-p=1 ]]
  printf "%s" "$2"
' test "$PWD" "$literal"
[[ $(<"$scratch/stdout") == "$literal" ]]

# Nested invocation must fail admission, rather than double the budget.
expect_status 2 bash "$runner" -- bash "$runner" -- true
[[ $(<"$scratch/stderr") == *'another validation job is active'* ]]
expect_status 0 bash "$runner" --check

# The payload handles TERM as success; the supervisor must still report the
# expired scope as failure. A TERM-ignoring descendant must also be removed.
expect_status 125 bash "$runner" --timeout 1 -- bash -c '
  trap "exit 0" TERM
  bash -c '\''trap "" TERM; printf "%s" "$BASHPID" > "$1"; exec sleep 30'\'' child "$1" &
  wait
' test "$scratch/child-pid"
child=$(<"$scratch/child-pid")
if kill -0 "$child" 2>/dev/null; then
  printf 'timed-out scope left child %s alive\n' "$child" >&2
  exit 1
fi
expect_status 0 bash "$runner" --check

# A missing/unusable scope launcher must never execute an unbounded fallback.
mkdir "$scratch/tools"
printf '#!/usr/bin/env bash\nexit 77\n' > "$scratch/tools/systemd-run"
chmod +x "$scratch/tools/systemd-run"
expect_status 77 env PATH="$scratch/tools:$PATH" bash "$runner" -- touch "$scratch/escaped"
[[ ! -e $scratch/escaped ]]
printf 'ok: bounded runner admission, limits, arguments, status, deadline, and cleanup\n'
