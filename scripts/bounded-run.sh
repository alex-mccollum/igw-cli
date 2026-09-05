#!/usr/bin/env bash
# Local Linux/WSL validation only. No host/engine recovery or unbounded fallback.
set -euo pipefail

fail() { printf 'bounded-run: %s\n' "$*" >&2; exit 2; }

# Invoked by systemd-run after moving this process into its own scope. Verify
# kernel controls before executing the payload; accepted properties alone do
# not prove that controllers are delegated and limits were applied.
if [[ ${1:-} == --inside ]]; then
  shift
  read -r entry < /proc/self/cgroup
  [[ $entry == 0::/*/igw-validation-*.scope ]] || fail 'missing validation scope'
  group="/sys/fs/cgroup${entry#0::}"
  for setting in memory.max memory.high memory.swap.max memory.oom.group cpu.max pids.max; do
    [[ -r $group/$setting ]] || fail "cannot verify $setting"
  done
  [[ $(<"$group/memory.max") == 8589934592 ]] || fail 'memory limit not applied'
  [[ $(<"$group/memory.high") == 6442450944 ]] || fail 'memory throttle not applied'
  [[ $(<"$group/memory.swap.max") == 0 ]] || fail 'swap limit not applied'
  [[ $(<"$group/memory.oom.group") == 1 ]] || fail 'group OOM policy not applied'
  [[ $(<"$group/pids.max") == 256 ]] || fail 'task limit not applied'
  read -r quota period < "$group/cpu.max"
  [[ $quota =~ ^[0-9]+$ && $period =~ ^[0-9]+$ ]] || fail 'CPU limit not applied'
  (( period > 0 && quota > 0 && quota <= 2 * period )) || fail 'CPU limit exceeds two cores'

  # The supervisor keeps this lock outside the workload's cgroup, including
  # while a failed workload is being cleaned up.
  flock --nonblock 9 || fail 'validation lock was not inherited'
  export GOMAXPROCS=2 GOMEMLIMIT=3GiB
  export GOFLAGS="${GOFLAGS:+$GOFLAGS }-p=1"
  printf 'bounded-run: verified 8 GiB RAM, no swap, 2 CPUs, 256 tasks; Go packages serialized\n' >&2
  (( $# > 0 )) || fail 'missing command'
  exec "$@"
fi

seconds=600
check=false
while (( $# > 0 )); do
  case $1 in
    --timeout)
      (( $# >= 2 )) || fail '--timeout requires seconds (1..600)'
      seconds=$2; shift 2
      ;;
    --check) check=true; shift ;;
    --) shift; break ;;
    -h|--help)
      printf 'Usage: bash scripts/bounded-run.sh [--timeout SECONDS] -- COMMAND [ARGS...]\n'
      printf '       bash scripts/bounded-run.sh --check\n'
      exit 0
      ;;
    *) fail 'expected -- before command (use --help)' ;;
  esac
done
[[ $seconds =~ ^[1-9][0-9]{0,2}$ ]] && (( seconds <= 600 )) || fail '--timeout must be 1..600 seconds'
if $check; then
  (( $# == 0 )) || fail '--check does not accept a command'
  set -- true
fi
(( $# > 0 )) || fail 'missing command (use --help)'
[[ $(uname -s) == Linux && -f /sys/fs/cgroup/cgroup.controllers ]] || fail 'Linux with cgroup v2 is required'
[[ -d /run/user/$UID && -O /run/user/$UID && ! -L /run/user/$UID ]] || fail 'private user runtime directory unavailable'
for dependency in systemd-run systemctl flock awk realpath; do
  command -v "$dependency" >/dev/null || fail "required command unavailable: $dependency"
done
systemctl --user show-environment >/dev/null 2>&1 || fail 'user systemd manager unavailable; no unbounded fallback'
available=$(awk '/^MemAvailable:/ {print $2}' /proc/meminfo)
[[ $available =~ ^[0-9]+$ ]] && (( available >= 10485760 )) || fail 'at least 10 GiB available Linux memory is required'

script=$(realpath -- "${BASH_SOURCE[0]}")
exec 9>"/run/user/$UID/igw-validation.lock"
flock --nonblock 9 || fail 'another validation job is active; wait for it to finish'
unit="igw-validation-$BASHPID-$RANDOM.scope"
cleanup() {
  # Only this invocation's transient scope, never a host service or engine.
  systemctl --user stop "$unit" >/dev/null 2>&1 || true
  systemctl --user reset-failed "$unit" >/dev/null 2>&1 || true
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM
status=0
systemd-run --user --scope --quiet --expand-environment=no \
  --unit="$unit" --description='igw bounded local validation' \
  --property=MemoryHigh=6G --property=MemoryMax=8G --property=MemorySwapMax=0 \
  --property=CPUQuota=200% --property=TasksMax=256 --property=OOMPolicy=kill \
  --property="RuntimeMaxSec=${seconds}s" --property=TimeoutStopSec=5s \
  -- bash "$script" --inside "$@" || status=$?
# Failed scopes remain queryable until reset, even if a payload handles TERM
# by exiting zero. A deadline or OOM must never turn into a passing check.
result=$(systemctl --user show "$unit" --property=Result --value 2>/dev/null || true)
if [[ -n $result && $result != success ]]; then
  printf 'bounded-run: scope failed (%s)\n' "$result" >&2
  (( status != 0 )) || status=125
fi
exit "$status"
