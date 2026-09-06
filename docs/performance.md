# Performance qualification

The v1 cutover replaces legacy call/RPC microbenchmarks with command-schema,
typed HTTP request, actual captured-catalog, loaded-operation lookup, conditional
catalog revalidation, and streamed-artifact benchmarks.
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

## JSON contract engine

The JSON engine removes the full OpenAPI model, YAML rendering, duplicate
operation metadata decoding, and repeated reference verification. Document and
contract hashing avoid extra encoded-document copies. A paired process run with
Go 1.27.1 under the unchanged 8 GiB/two-CPU guard observed:

| Reference | Startup before / after | Peak RSS before / after |
| --- | --- | --- |
| ignition-8.3.0-core | 0.69 / 0.31 s | 148.5 / 71.5 MiB |
| ignition-8.3.0-defaults | 1.12 / 0.49 s | 248.6 / 108.7 MiB |
| ignition-8.3.9-core | 0.76 / 0.33 s | 161.0 / 77.8 MiB |
| ignition-8.3.9-defaults | 1.63 / 0.80 s | 399.2 / 158.9 MiB |

Each row uses five interleaved samples per binary and unchanged reference bytes.
All operation inventories and raw/document/contract identities match. The
[measurement record](qualification/catalog-json-engine.json) retains binary
hashes, samples, and inventory checksums. These are local startup observations,
not portable latency guarantees or fresh live-Gateway qualification. Earlier
measurements below describe the superseded model backend.

## Parser 20 optimization

Profiling the cutover parser attributed about 44% of sampled allocations to the
YAML scanner's token insertion. The private parser representation now uses a
block root with JSON-encoded values, so the scanner can consume each root member
without queuing the entire document as a possible complex key. Indentation still
avoids quadratic same-line node indexing. Vendor bytes, returned definitions,
raw/document/contract hashes, and reference provenance stay unchanged.

Full document validation uses the already decoded exact JSON value with the
same embedded OpenAPI 3.0/3.1 metaschemas and compiler as the upstream validator.
Model construction and reference validation still run. This avoids redundant
JSON views and, for adapted schemas, a second parser document. Differential
validation and typed-node tests cover both versions, precise numbers, Unicode,
references, and invalid documents. Numeric work is bounded before compilation.
No parser validation flags are relaxed and no dependency version changes.

The final Go 1.27.1 checks observed a 1.440 s full-reference open with
772,654,328 B/op: about 28% less elapsed time and 50% fewer cumulative allocated
bytes than the cutover baseline. Go 1.25.7 observed 1.856 s and 823,487,000 B/op.
These are separate local three-iteration runs. The allocation regression ceiling
is now 1 GiB, leaving headroom on both toolchains while detecting a return to the
previous cost. This ceiling measures allocation, not resident memory.

Five actual process samples took 1,548.377–1,604.946 ms for reference inspection
(median 1,566.673 ms), and 13.533–14.954 ms for command schemas. Three additional
`/usr/bin/time` runs of reference inspection observed 398,300–407,356 KiB peak
RSS (389–398 MiB) and 1.60–1.69 s elapsed time. Each was a new CLI process using
the complete retained reference, with output discarded. OS filesystem caches
were not flushed; this is process startup, not cold-disk latency.

The loaded-operation benchmark opens and validates the actual reference outside
its timer, then resolves the exact Gateway-info operation with independent
definition bytes. Setup and release are excluded. A 1,000-iteration Go 1.27.1
run observed 2,111 ns/op and 4,112 B/op. This isolates lookup cost; ordinary
separate CLI invocations still pay catalog-open cost. The 32 MiB streamed
artifact benchmark remains below 1 MiB allocation on both toolchains.

[The parser-20 receipt](qualification/catalog-parser20.json) binds production
inputs and the executable to retained full-unit, smoke, catalog-race, benchmark,
and process logs. The four current-parser capture expectations advance to parser
20; their other fields and all original capture/live receipts stay unchanged.
No Gateway or host lifecycle operation ran during these checks. All validation
used the same 8 GiB/two-CPU limits; none needed a larger allocation.

The checked-in thresholds are regression ceilings with headroom for shared CI,
not portable performance guarantees. Final completed-source acceptance remains
part of the full v1 goal.

## Unchanged catalog revalidation

The refresh service now reuses the catalog already validated by `Store.Load`
when the selected Gateway confirms its conditional request with HTTP 304, or
returns exactly identical document bytes. It still verifies each write invocation
and publishes a new immutable receipt. Catalog ownership transfers only after
publication succeeds, preserving the fallback model and metadata on error.
Different bytes require a full parse even when their contract hash is equal.
Unsolicited 304 responses and validators from imported catalogs cannot establish
fresh verification. Parser and contract-policy identities do not change.

`BenchmarkCatalogRevalidation` uses the complete retained 687-operation catalog,
an isolated on-disk store, and a loopback HTTP fixture. Each measured iteration
loads and validates the snapshot, makes one conditional verification request,
publishes the receipt, and closes the returned catalog. Initial reference open
and store setup are outside the timer. This is a service-path measurement, not
a live-Gateway or separate-process measurement.

The three-iteration Go 1.27.1 baseline observed 2.422 s and 1,238,138,594 B/op.
The final gate observed 1.293 s and 646,838,037 B/op, about 47% less time and
48% fewer allocated bytes. Go 1.25.7 observed 1.626 s and 718,034,120 B/op.
Both toolchains pass the new 1 GiB allocation ceiling, which would reject the
baseline's duplicate parsing cost. The same gate retains the independent
full-reference, loaded-lookup, request, schema, and 32 MiB artifact checks.

[The revalidation receipt](qualification/catalog-revalidation.json) retains
source/binary identities, before/after measurements, regression results, full
unit/smoke checks, focused race checks, and minimum-Go checks. The ownership
tests also exercise precise referenced request constraints after transfer and
after failed publication. No historical Gateway qualification is renewed.

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
