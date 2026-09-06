# Current qualification status

Candidate `a1d2b4b` passed the simplification gates on 2026-09-06 UTC.
No release was published and no hosted reference-update activation is claimed.
The [qualification record](simplification.json) binds the source, executables,
images, observed catalogs, measurements, and artifact hashes. Full local run
receipts and logs are preserved under `bin/simplification/final/`.

Subsequent maintenance through `7a30c7e` on 2026-09-06 added verified-blob reuse,
accurate request field pointers, actionable lock errors, original capture dates,
and hosted
updater status reporting. It passed the full offline Go suite, catalog/config/
filesystem-lock race tests, 34 executable smoke checks, 23 reference-tool tests,
command docs/lint, and workflow linting. Logs are in `bin/improvements/`.
The built CLI includes the runtime follow-ups through `6809e5f`; those changes
have not received a new live Gateway qualification or release artifact matrix.
The table and performance measurements below apply to `a1d2b4b` only.

The read-only hosted status check returned `not-installed`: the remote reference
workflow still needs publication, a verified dedicated runner, activation, and
an observed complete run. See [activation steps](../reference-updates.md#updater-status-and-activation).

| Check | Result |
| --- | --- |
| Full offline Go suite | Passed |
| Race detector | Full suite passed; follow-up cache integrity changes passed catalog race tests |
| Minimum Go 1.25.7 | Full suite and CLI build passed |
| Executable smoke checks | 34 passed, including packaged Linux executable |
| Command docs, lint, update coordinator | Passed; eight coordinator tests |
| Performance budgets | Passed |
| Release artifacts | Six built and structurally audited; Linux amd64 executed natively |
| Live version/module matrix | All 20 stages passed across 8.3.0/8.3.9 and core/default profiles |
| Native CLI journeys | 206 process checks passed; 48 per 8.3.0 cell, 55 per 8.3.9 cell |

Every live stage used one fresh owned Gateway under verified limits. Cleanup
receipts and independent name/ownership-label queries passed after every stage.
No WSL/Docker Desktop lifecycle or host memory settings were changed.

Matched release builds show 2.21-3.06x faster reference startup and 53.8-70.9%
lower peak RSS across four references, with identical operation inventories and
catalog identities. Root command discovery is 2,932 bytes by default versus
94,604 bytes recursively. These are local measurements, not latency guarantees.
See [methodology](../performance.md).

Profiles, current `igw-contract/2` pins, workflow commands, explicit `--yes`,
`igw/v1` results, and exit codes remain. Cache and portable reference formats
were intentionally reset before release; raw document import remains available.
Historical built-in qualification identities and dates are preserved separately
from current inspection. Current live checks do not relabel old capture packets.

Prior plans, failed attempts, repeated captures, and superseded qualification
logs are [archived in Git](README.md#historical-evidence) (`65e643d:docs/qualification/README.md`).
Canonical vendor documents and behavior fixtures remain in the active tree.
[Workflow limits](../compatibility-matrix.md) still apply, including unavailable
8.3.0 tag APIs and uncertain singleton configuration creation.

The weekly updater still needs a dedicated enabled runner and an observed
successful hosted run before it can be called active. Embedded references and
retained target snapshots remain available during upstream failures. See
[reference maintenance](../reference-updates.md).

## Historical evidence

Earlier plans, investigations, and original run packets remain in Git at
`65e643d`. References in these docs use `COMMIT:path` so they remain usable in a
full local checkout without depending on a published GitHub URL. For example:

```bash
git show 65e643d:docs/qualification/README.md
git show 65e643d:internal/testgateway/testdata/singleton/attempts/README.md
```

Use `git ls-tree -r --name-only 65e643d -- PATH` to list an archived directory.
A shallow checkout may not contain this history. The local rebuild history had
not been published when checked on 2026-09-06, so its GitHub archive URLs are
not currently available. Bundled manifests retain their original evidence
locators and hashes; their archived full manifests can be read at
`65e643d:internal/reference/bundles/SELECTOR/reference.json` in this checkout.
Portable two-file bundles do not include Git history or the contributor packet.
