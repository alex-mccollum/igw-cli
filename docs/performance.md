# Performance qualification

The v1 cutover replaces legacy call/RPC microbenchmarks with command-schema,
typed HTTP request, actual captured-catalog, and streamed-artifact benchmarks.
Run `scripts/perf-gate.sh` through the bounded runner. It uses three iterations
of each benchmark, checks every expected metric, and refuses missing results.
It does not contact a Gateway or run concurrent heavy jobs.

The final cutover check on the shared Linux workstation used three benchmark
iterations and observed:

| Benchmark | Time | Total allocated bytes per operation |
| --- | --- | --- |
| Command schema | 1.44 ms | 686,114 |
| Typed HTTP request against a loopback fixture | 0.364 ms | 20,344 |
| Full 8.3.9 default-module reference validation/open | 2.00 s | 1,555,713,656 |
| Stream and publish 32 MiB with hashing | 0.628 s | 109,322 |

These measurements are retained in
[the cutover check log](qualification/v1-cutover-checks.txt), under the 8 GiB,
two-CPU runner with Go 1.27.1, using the precommit cutover worktree based on
`b08b625`. They are local observations, not portable latency guarantees or
live-Gateway timings.
The [qualification receipt](qualification/v1-cutover.json) records the exact
Go/module/reference input digest and toolchain.
Go's B/op is cumulative allocation, not peak resident memory. The catalog case
verifies the full retained reference and builds its model; it is not a synthetic
small schema or a warmed operation lookup.

The same run measured two successful real-process samples per case:
command schemas took 14.862–14.926 ms and full reference-based operation
inspection took 2,199.808–2,210.082 ms. This small sample establishes a starting
point, not a latency distribution. Reported p50/p95 use nearest-rank empirical
quantiles. A separate full CLI race-test run sampled about 2.91 GiB resident
memory; that is an observation, not a measured peak or a portable memory bound.

The checked-in thresholds are regression ceilings with headroom for shared CI.
They are not an assertion that current catalog cost is the best achievable UX.
The captured catalog's allocation and startup cost remain an optimization target
in the full v1 goal. Full source qualification must include actual process
latency, retained input identity, memory observations, and representative large
artifacts. Do not declare that gate complete from the microbenchmarks alone.

The artifact allocation ceiling is 1 MiB for a 32 MiB payload, so buffering the
whole payload fails the gate. Its elapsed-time ceiling includes filesystem sync
and therefore varies with storage. Catalog allocation is separately bounded by
the regression threshold, while the runner supplies the actual hard memory cap.

`scripts/perf-baseline.sh` records real executable latency for local command
schemas and full reference-based operation inspection. `ITERATIONS` is bounded
to 1..100 (default 5). `IGW_PERF_LIVE=1` adds explicit read-only doctor, request,
and batch observations against the selected profile. It records timing and
failures rather than Gateway payloads. Any failed observation makes the command
fail; it does not convert missing samples into a passing baseline.

Legacy RPC measurements are not comparable with the new cases. The old mutating
scan benchmark is removed. No benchmark raises workstation limits, controls WSL
or Docker Desktop, or restarts a Gateway.
