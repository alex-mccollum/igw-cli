# Local validation safety

Shared Linux/WSL workstations must run repository builds, tests, and large
captured-schema parsing through the bounded runner:

```bash
bash scripts/bounded-run.sh --check
bash scripts/bounded-run.sh -- go test ./...
bash scripts/bounded-run.sh -- go build ./cmd/igw
bash scripts/bounded-run.sh -- bash scripts/lint-docs.sh
```

Run each command to completion before starting the next. The runner requires
Linux cgroup v2, a working user systemd manager, delegated memory/CPU/PID
controllers, and at least 10 GiB available Linux memory (the job budget plus
2 GiB headroom). It refuses to execute the command if the required controls
are unavailable or not applied.

The command and its Linux descendants share an 8 GiB hard memory limit, a
6 GiB reclaim threshold, zero swap allowance, two CPUs of quota, and 256
tasks (threads count). A memory-limit failure kills the scope as a group.
The default runtime limit is ten minutes with five seconds for termination;
`--timeout SECONDS` can lower it. A per-user lock rejects overlapping guarded
jobs across checkouts. No persistent systemd units or host settings are changed.

Go uses `GOMAXPROCS=2`, `GOMEMLIMIT=3GiB`, and an appended `GOFLAGS=-p=1` to
reduce contention. `GOMEMLIMIT` is a soft runtime target; the cgroup provides
the hard limit, including native allocations and race-detector overhead.
The per-process Go target leaves room within the shared job budget for that
overhead and child processes. These are usage ceilings, not upfront memory
reservations. Changing this job budget does not require a WSL restart.
Arguments, standard streams, current directory, and the caller's environment
are preserved except for those Go settings. Normal command exit codes are
preserved; admission, timeout, and resource failures return nonzero.

Do not automatically retry a failed job, increase limits, or bypass the guard.
Inspect the failure and reduce the workload or move expensive checks to an
isolated CI runner. A timed-out or interrupted check has not passed.

Run `bash scripts/test-bounded-run.sh` directly to verify the guard itself.
It launches small guarded shell commands to check kernel limits, admission,
argument preservation, exit codes, and timeout cleanup. It does not start
Docker, build Go code, or deliberately exhaust memory. Do not wrap this
self-check in another bounded runner: its overlap test needs to acquire the
validation lock independently.

## Boundaries

This runner contains cooperative Linux child processes. It is not a security
sandbox or a limit on Windows applications, other users, or Docker's daemon.
Linux `MemAvailable` does not measure Windows host memory pressure. Jobs that
delegate work to another service need that service's own controls.

Do not overlap builds/tests with image pulls or Gateway captures. Live Gateway
capture is paused following the incident below, pending separate verification
of capture admission, cleanup, and container memory/swap/CPU/PID limits. A
killed capture process can leave its container running because the daemon is
outside the scope. Container cleanup must verify the exact disposable
container ID and ownership label; broad cleanup/prune commands are unsuitable.

Repository automation must not start, stop, restart, terminate, unregister, or
repair WSL distributions or Docker Desktop. An unavailable engine blocks live
checks. Read existing logs and report the problem; host recovery requires a
separate, explicit instruction. Do not modify Windows services, registry,
`.wslconfig`, or Docker Desktop settings during validation.

## 2026-09-05 incident

The user reported `Wsl/0x80040155` for `wsl.exe --terminate docker-desktop`.
Read-only examination of retained local logs established this sequence (UTC):

- 11:51:10: the Ubuntu journal records a poweroff request and orderly shutdown.
- 11:51:13: Docker Desktop loses its WSL bridge.
- 11:51:22: Docker's WSL proxy cleanup reports insufficient memory,
  `Wsl/0x8007000e`.
- 11:52:32: Docker Desktop's recovery log records the exact
  `--terminate docker-desktop` / `0x80040155` failure reported by the user.

The retained Linux kernel journal contained no OOM-kill or panic event for
that boot. These logs establish the recovery failure, not what initiated the
shutdown. Microsoft identifies `0x80040155` as an unregistered interface and
`0x8007000e` as out of memory in its
[HRESULT reference](https://learn.microsoft.com/en-us/windows/win32/com/com-error-codes-1).
Neither code alone proves that a particular repository command caused it.

An unbounded `go test -race ./...` had been launched alongside other validation
work; its completion result was lost across the restart. It is not recorded
as passing. The large OpenAPI model had previously consumed about 605 MiB in
a non-race inspection. This warranted limiting validation independently of
whether it caused the host failure. Heavy tests and container startup were
paused during the investigation. The new guard reduces workload risk; it
cannot guarantee that an unrelated Windows/WSL/Desktop fault will not recur.

Resource-control behavior follows systemd's
[resource controls](https://github.com/systemd/systemd/blob/main/man/systemd.resource-control.xml)
and [scope lifecycle](https://github.com/systemd/systemd/blob/main/man/systemd.scope.xml).
