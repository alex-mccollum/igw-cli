# Local qualification evidence

`catalog-revalidation.json` records the subsequent unchanged-catalog refresh
optimization, based on a precommit worktree at `e306eec`. The before/after
benchmark measures disk load, current-parser validation, conditional HTTP
verification against a fixture, receipt publication, and model cleanup. The
receipt identifies full unit/33 smoke checks, focused race checks, and both Go
toolchains' performance gates. Its initial baseline transcript includes a test
fixture correction; the separate regression transcript then reproduces only
the four expected missing-reuse cases. Production code changed after that
regression. This adds no live-Gateway or host-lifecycle evidence.

`catalog-parser20.json` records the subsequent catalog optimization. It binds
production/reference inputs and the native executable to exact retained logs
for the full suite, 33 smoke checks, catalog race checks, both Go toolchains'
performance gates, process timings, and measured peak RSS. The source was a
precommit worktree based on `8efb9ee`. Its transcript notes distinguish the
corrected loaded-lookup timing from the initial sample, and cumulative profile
allocation from per-operation allocation and peak memory. See
[performance methodology and results](../performance.md).

The four current-parser capture expectations advance to parser 20 without any
other field changing. Original vendor captures, reference manifests, and live
receipts remain historical evidence. All local checks used the existing guard
limits, without containers or host configuration changes.

## Single-entrypoint cutover

`v1-cutover.json` records the six-platform artifact audit and the source-input
digest for the single-entrypoint cutover. `v1-cutover-checks.txt` retains the
unaltered final check log; its SHA-256 is recorded in the receipt. The directory
disables Git newline conversion to preserve that identity.
The exact check transcript also preserves the benchmark's padded CPU field,
so whitespace lint is disabled for that transcript alone.

These checks ran on a precommit worktree based on `b08b625`, using Go 1.27.1 and
the bounded runner. The 244 Go/module/reference inputs match commit `62c9de9`.
That comparison does not change the build's recorded base commit or its `-dirty`
version suffix. Documentation and this evidence were recorded after the build.
Local archives remain in `bin/cutover-artifacts-v1`; no release was published.

The dry-run passed full unit and command/docs checks, all six builds, packaged
Linux version checks, and 33 isolated process checks. The same guarded job then
passed focused command race tests, 33 native process checks, the captured-catalog
and streamed-artifact benchmark gate, and real-process latency samples.

An independent archive audit checked all six platform headers, binary hashes,
required documentation, twelve checksums and aliases, and private staging
cleanup. Deliberately inserted stale ZIP members and a stale payload directory
were excluded from rebuilt archives; the existing payload directory was
preserved. The receipt records these checks separately from the dry-run log.

Only Linux amd64 executables ran locally. Compilation and header checks are not
native Windows/macOS behavior evidence. No real Gateway was contacted in these
checks, and no historical Gateway receipt was relabeled. The full rebuild's
remaining input, workflow, performance, and final-source qualification gates
stay active.
