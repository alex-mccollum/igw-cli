# Reference updates

The target Gateway's current `/openapi.json` remains the documented contract for
that target. Qualified references provide a dated, independently available API
source when a Gateway or registry is unreachable. The CLI never silently treats
an offline reference as a live Gateway verification. See `docs/catalog.md` for
authority, freshness, pinning, and schema qualification rules.

## Run the qualification pipeline

The contributor coordinator uses Python 3's standard library to call the
existing bounded runner and Go capture/qualification tools. It adds no runtime
dependency to the distributed CLI. Run it directly; it creates separate verified
resource scopes for each stage and must not be nested in another bounded job.

```sh
python3 scripts/update-reference.py --tag 8.3 --out bin/new-reference-run
python3 scripts/update-reference.py --tag 8.3.9 --out bin/new-patch-run --skip-pull
python3 scripts/update-reference.py --tag 8.3.0 --module-profile core-opcua --out bin/new-core-run
```

Use `--docker PATH` when the Docker executable has a different location. The
engine must already be running. `--baseline FILE` selects the previous qualified
JSON or JSON.gz document; the default is the retained 8.3.9 reference.
`--skip-pull` requires the resolved immutable image to be present locally.
`--require-clean` verifies the source commit and clean Git status before building
and before final qualification. A separate clean worktree allows this check
without disturbing edits in another checkout. All output directories are new;
failed and previous runs are preserved. Run logs, intermediate metadata, and
temporary build/test files stay beneath the private output directory.

`--module-profile` selects `image-defaults` (the default) or `core-opcua`.
The latter enables `com.inductiveautomation.opcua` and requires all other
installed first-party modules to be inactive and disabled. The same selection
reaches the lifecycle probe, capture, and every workflow Gateway. Ambient
`IGW_TEST_MODULE_PROFILE` values cannot change the coordinator's selection.
Workflow tests validate the stable inventory before provisioning API credentials
or exercising mutations.

The pipeline performs these steps serially:

1. Verify the resource guard, required Docker capabilities, and an empty
   qualification slot. Engine logs contain only capability fields.
2. Record source/toolchain identity and build the contributor and one acceptance
   binary with trimmed source paths, targeting Linux amd64.
3. Resolve the official tag once, retaining exact registry manifests. Use only
   that immutable image throughout the remaining stages.
4. Pull the selected platform when requested, then verify container admission,
   limits, lifetime termination, and cleanup with the lifecycle probe.
5. Capture the exact API document and stable observed module inventory.
6. Run resource, project/tag, and operational acceptance tests from the same
   binary, each in a fresh disposable Gateway.
7. Assemble a checksummed reference using all receipts and the prior baseline.

`run.json` uses `igw/reference-update/v1` and records dated stage statuses,
failures, image identity, source commit/dirty state, Go toolchain, coordinator checksum, test
binary checksum, and the final catalog comparison. Only successful Go
qualification and bundle readback produce `status: "qualified"`. A changed
contract still requires compatibility review. Workflow qualification covers
the recorded scopes, not every operation in the API document.

Qualification policy `igw-reference-workflows/2` records tag capability evidence
from the captured catalog. With both tag routes advertised, all existing tag
round-trip, preview, overwrite, and partial-failure checks remain mandatory.
When both routes are absent, the transfer suite still runs every project check
and verifies that tag import, preview, and export fail with `capability`/2 before
any operation request. Transfer receipts use version 3 and include actual
operation-request counts, error kinds, exit codes, and capabilities. Resource
and operational receipts retain version 2.

The manifest's `qualification.scopes` lists exercised workflows; absent tag
support is recorded separately as `unavailableScopes: ["tags/memory-json"]`.
An unavailable workflow is never counted as a passing round trip. The assembler
cross-checks capability evidence against the original captured document and
requires the exact applicable check set. Partially advertised tag APIs require
additional qualification coverage and fail this policy explicitly. Historical qualification summaries keep their original scope; old development
portable bundles must be regenerated in runtime format `igw/reference/v2`.

New manifests separately record module policy `igw-module-profile/1`, its
profile name, and explicit enabled-module identifiers in `moduleProfile`.
The complete inventory is retained, including disabled modules. Qualification
rejects missing selected modules, unexpected active modules, faults, quarantine,
pending upgrades, or different whitelist values across capture and any receipt.
These cross-checks run in the contributor assembler. Runtime readers verify the
compact manifest and compressed document; they do not replay the qualification
packet. The two-file runtime reference points by URI and SHA-256 to the detailed
`evidence/qualification.json` manifest. Preserve the full contributor artifact
when promoting a new reference. Listing reads only compact manifests.

Every nonzero stage result stops the pipeline. There is no retry, image fallback,
automatic module substitution, host recovery, or budget increase. An outer
watchdog bounds guard startup as well as the workload. Cancellation targets only
the coordinator's own process group; the bounded runner owns scope cleanup and
containers retain their separate lifetime limits. Inspect any remaining owned
container before another run. The contributor never repairs WSL or Docker
Desktop. Local safety requirements are in `docs/development-safety.md`.

## Scheduled execution

`.github/workflows/reference-update.yml` defines Tuesday 09:23 UTC qualification
of the `8.3` channel and manual dispatch for an explicit patch. It accepts only
default-branch execution, uses read-only repository permissions, pins action
implementations by commit, disables persisted checkout credentials, and serializes
runs without canceling one in progress. It does not execute pull-request code,
commit updates, push, tag, publish, or change the CLI's bundled default.

Each scheduled or manually dispatched tag is qualified with both module
profiles. The matrix uses `max-parallel: 1` and `fail-fast: true`: jobs run
serially and a failure cancels queued profile jobs. Output directories and
artifact names include the profile, so evidence cannot overwrite another cell.
The outer workflow still keeps `cancel-in-progress: false` to avoid interrupting
an active qualification when a later run is queued. These controls follow
[GitHub's matrix documentation](https://docs.github.com/en/actions/how-tos/write-workflows/choose-what-workflows-do/run-job-variations).

Activation requires a provisioned runner with labels `self-hosted`, `linux`,
`X64`, and `igw-reference`, plus repository variable
`IGW_REFERENCE_RUNNER_ENABLED=true`. Use a dedicated host with an up-to-date
Actions runner, Python 3, Git, a working user systemd manager, cgroup v2 controller
delegation, and a running Linux Docker engine. Allow at least 16 GiB RAM and
enough disk for the image, Go caches, temporary builds, and retained evidence;
the exact admission check still requires 10 GiB Linux memory available. The
process scope remains limited to 8 GiB with zero swap, two CPUs, and 256 tasks.
Each disposable Gateway separately receives 2 GiB, zero swap, two CPUs, and
256 PIDs. The coordinator never provisions or changes these host settings.

The workflow requires a clean checkout and retains allowlisted candidate and
failure evidence for 30 days. It excludes compiled tools and temporary test
files, which can include synthetic Gateway backups and diagnostics. Candidate
artifacts are for review; the retained Git bundle and binary embedding provide
durable availability after those artifacts expire. See the upstream
[artifact retention documentation](https://github.com/actions/upload-artifact#retention-period).

GitHub schedules can be delayed or dropped under load; public-repository
schedules can also be disabled after 60 days without repository activity.
Scheduled and manually dispatched workflows must exist on the default branch.
These limitations are documented under
[workflow events](https://docs.github.com/en/actions/reference/workflows-and-actions/events-that-trigger-workflows#schedule).
Check the date and outcome of the latest run; a missing, skipped, or failed run
is not freshness evidence. Use manual dispatch or the same local coordinator
when a scheduled run is missed. The repository variable/runner must be enabled
and a real hosted run observed before claiming the remote schedule is active.

## Review and retain an update

Inspect `run.json`, every required stage, the reference manifest, and the catalog
comparison. Confirm the image/version, observed module profile, current parser,
clean source provenance, complete workflow checks, and cleanup. Retain prior
qualified bundles when a new image fails. A raw or document-only change with an
equal policy hash still retains its new original bytes and provenance; a
contract change requires focused compatibility assessment and real checks.

After review, add only `reference.json` and `openapi.json.gz` to a new directory
under `internal/reference/bundles/`, or distribute those two files independently
through an authorized release channel. Retain the full contributor audit packet
at a durable source and set its evidence URI accordingly; changing the locator
must preserve the evidence checksum and original qualification identity. Update the baseline intentionally. Preserve prior bundles;
never replace an old manifest with newly generated evidence. Shipping bytes and
promoting a new default are separate reviewed release actions.

The coordinator supports both reviewed module profiles. The initial minimum/
latest matrix has complete local qualification for both profiles; selecting an
additional tag alone does not prove that release is supported. Read the progress
evidence in
[current qualification status](qualification/README.md) before making compatibility or schedule-activation
claims.

Coordinator failure/ordering tests run without Docker or Go compilation:

```sh
python3 -m unittest discover -s scripts -p 'test_update_reference.py'
```

These tests verify orchestration, not real Gateway behavior. Real pipeline
receipts provide the separate acceptance evidence.

## Historical qualification

The four version/profile cells were qualified on 2026-09-05 using the earlier
parser. Their original pipeline runs and audit packets remain in
[Git history](https://github.com/alex-mccollum/igw-cli/blob/65e643d/docs/reference-updates.md#recorded-real-run).
Current-format built-ins retain those dates and qualified identities separately
from present parsing. Consult [current status](qualification/README.md) before
claiming that a new source revision passed live qualification.
