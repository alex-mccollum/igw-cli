# Local cutover qualification

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
