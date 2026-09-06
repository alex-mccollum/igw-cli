# Gateway qualification matrix

Qualification applies to a recorded image, observed module inventory, contract
policy, and set of exercised workflows. A version label alone does not establish
support for every API or module combination. The target Gateway's current
catalog determines which APIs the CLI can use.

## Workflow-first candidate

Clean candidate `511c5fe` passed 48 native process checks on pinned 8.3.0 and
55 on pinned 8.3.9, both using `core-opcua`, on 2026-09-06 UTC. These cover the
six release journeys, with explicit absent tag APIs on 8.3.0. Diagnostics starts
from an empty status without `fileSize` on both versions. Additional singleton
checks preserve uncertain config creation and rejected metadata-only creation;
complete translations creation is not qualified. Every live stage cleaned up.

See the [candidate record](qualification/workflow-v1/README.md) for exact source,
executable, image, module, artifact, and limitation evidence. The historical
matrix below retains its original identities and broader reference scopes.

## Qualified version and module cells

| Gateway | Profile | Observed modules | Operations | Tag transfer workflows |
| --- | --- | --- | ---: | --- |
| 8.3.0 | image-defaults | 32 active | 672 | APIs absent; import, preview, and export refuse before dispatch |
| 8.3.9 | image-defaults | 32 active | 687 | Advertised; all 39 project/tag checks passed |
| 8.3.0 | core-opcua | 1 active, 31 inactive | 446 | APIs absent; all 27 applicable project/tag checks passed |
| 8.3.9 | core-opcua | 1 active, 31 inactive | 454 | Advertised; all 39 project/tag checks passed |

All four cells passed all 18 pipeline stages under workflow qualification
policy 2 on 2026-09-05. Each includes 10 lifecycle, 27 resource, and 23
operational checks in addition to the applicable project/tag checks.

The minimum reference covers catalog validation, containment, basic schedule
resources, disabled projects, and backup/log/diagnostics workflows. It lists tag
round trips separately as unavailable. The 8.3.9 candidate covers those same
five scopes plus tag round trips. Negative tests exercise expected
failures; they do not imply a missing workflow passed a round trip.

Original clean-source run receipts and complete candidates are retained under
`internal/referencebuild/testdata/`. Each reference manifest binds exact vendor
bytes, registry manifests, image configuration, module inventory, parser,
contract identity, test executable, workflow scopes, and receipt checksums.
See [reference updates](reference-updates.md#recorded-real-run) for dated results
and [catalog authority](catalog.md) for freshness and pinning semantics.

The CLI embeds exact reviewed bundles for all four cells. List selectors with
`spec references list`: use `ignition-8.3.0-defaults`, `ignition-8.3.0-core`,
`ignition-8.3.9-defaults`, or `ignition-8.3.9-core`. The original 8.3.9 default
bundle remains unchanged, with its historical policy-1 evidence; the fresh
policy-2 default qualification is retained separately under contributor
testdata. Reopening historical evidence never renews its live verification
timestamp. References support offline inspection and export; live requests
continue to use the selected Gateway's own catalog.

## Additional list-filter qualification

Separate list-filter qualification passed on both `core-opcua` cells on
2026-09-05 using clean source `c8974b7` and the same test executable. Each run
passed 24 CLI checks after its lifecycle probe: resource/project/log selection,
exact special characters, combined filters, pagination, nonmatches, generic
request parity, preview without dispatch, and invalid/duplicate-key refusal.
The 8.3.9 and 8.3.0 suites took 79.10 and 74.13 seconds respectively. Every
container was removed and independent container queries were empty.

Original OpenAPI documents, receipts, process outcomes, and clean-source
observations are retained in `internal/testgateway/testdata/query-filters/`.
Both catalogs have the same contract identities as their core references above.
This additional evidence records parser 13; it does not rewrite the parser or
workflow policy of an earlier reference, qualify every filter field/operator,
or complete general parameter/multipart support.

## Additional request-body qualification

The body-input suite passed on both pinned `core-opcua` cells on 2026-09-05/06
UTC from clean source `03566a8`, using one executable and parser 19. Each run
first passed all 10 lifecycle checks with that same executable and image.

| Gateway | CLI checks | Body-suite duration | Bulk datafiles |
| --- | ---: | ---: | --- |
| 8.3.0 | 31 | 100.94 s | Absent route refused before dispatch |
| 8.3.9 | 62 | 148.53 s | Translations files uploaded and independently verified |

Both versions accepted literal/file/stdin UTF-8 text, explicit empty text,
binary bodies, streamed binary uploads, and empty uploads at the encryption
endpoint. Previews sent zero operations; consumed-byte hashes matched the
prepared inputs. Missing, invalid, and unsupported inputs failed locally.
Responses passed structural flattened-JWE checks, without decryption or
cryptographic validation.

On 8.3.9, repeated `files` parts with explicit filenames preserved binary,
Unicode-text, and empty files. A stale signature returned HTTP 500/exit 7 and
left the files unchanged. Opaque overwrite and deletion were independently
verified; the original translations configuration remained unchanged. `files`
is an observed working name, not an exclusive field-name contract. The optional
singleton creation/deletion branch was not exercised because it already existed.

All processes and receipts passed, independent container queries were empty,
and the source checkout remained clean. Original captures, previews, payload
digests, and provenance are retained with an integrity manifest under
`internal/testgateway/testdata/body-inputs/`. Contract hashes match the existing
core references. These results do not relabel earlier bundles or qualify every
datafile type, schema-bearing form, binary constraint, or module profile.

## Additional batch qualification

Both pinned `core-opcua` cells passed the batch suite on 2026-09-06 UTC from
clean source `2ddc7f4`, using the same executable and parser 19. Each first
passed 10 lifecycle checks, then 13 CLI checks: catalog capture and 12 batch
invocations with 29 ordered item results and 15 operation requests. The 8.3.9
and 8.3.0 suites took 60.80 and 59.25 seconds respectively.

Qualification covers malformed-input and confirmation refusal, online/offline
previews without dispatch, unconfirmed reads, mixed reads/encryption requests,
and preserved responses after later validation, HTTP 404, and unknown-operation
failures. Default stopping and explicit continuation behaved as documented.
Each confirmed batch fetched one fresh catalog for all items. Encryption
responses received structural JWE checks only. All processes and receipts
passed, independent container queries were empty, and the source stayed clean.

Original evidence and integrity checks are retained under
`internal/testgateway/testdata/batch/`. This scope does not cover arbitrary
operation combinations, live forced disconnects/revocation, or the subsequent
repeated-name budget fix. Transport/authentication failure handling and the
budget guard have separate fixture tests. The receipts keep their actual source
and timestamps; final current-source qualification remains part of the full v1
gate.

## Additional Gateway restart qualification

Both pinned core images passed on 2026-09-06 UTC from clean source `8084cd0`,
using the same executable and parser 19. Each first passed 10 lifecycle checks,
then 9 CLI checks. The 8.3.0 and 8.3.9 suites took 88.29 and 90.19 seconds,
with 62 and 61 restart polls respectively. Each sent one confirmed POST, used
one fresh catalog for the restart, and completed through `uptime_reset`.

The advertised `processId` identified the `ignition-gateway` wrapper. It stayed
unchanged while an independently observed Java child was replaced. Container
and wrapper start identities, Docker restart count, and applied resource limits
remained unchanged. Independent cleanup queries were empty. Both runs had
empty pending-task lists; module health and nonempty task completion were not
qualified. Exact captures and original provenance are retained in
`internal/testgateway/testdata/restart/`.

Separate containers returned the same `localId`, so matching that field is a
consistency check rather than proof of globally unique node identity. The CLI
requires direct node addressing for verification; its clarified correlation
label is `selected_target`. Original receipts retain `observed_node`, their
actual source, and their original timestamps. No OpenAPI schema claim is used
to infer uniqueness, JVM PID semantics, uptime units, or request causality.

## Observed module profiles

The core runs use the explicit `com.inductiveautomation.opcua` whitelist and
retain all 32 installed module observations. The selected OPC UA module is
active/enabled; the other 31 are inactive/disabled. Both complete pipelines ran
from clean commit `28d7334`, with the same acceptance executable. Original
receipts and qualified references are retained in
`internal/referencebuild/testdata/ignition-8.3.0-core/` and
`internal/referencebuild/testdata/ignition-8.3.9-core/`. Separate preceding
capture-only fixtures remain in `internal/moduleprofile/testdata/` with their
original observation times.

The coordinator's `--module-profile core-opcua` passes the same selection to
capture and every workflow suite. Module policy `igw-module-profile/1` verifies
the resulting inventory and refuses unexpected module states or substituted
profiles. IA documents fully qualified identifiers and the
module whitelist in its [container guide](https://www.docs.inductiveautomation.com/docs/8.3/platform/docker-image#module-identifiers-table)
and [environment reference](https://docs.inductiveautomation.com/docs/8.3/appendix/reference-pages/platform-environment-variables).
The whitelist value expresses intent; observed modules establish the test cell.

This matrix does not qualify third-party modules, every first-party workflow,
arbitrary cross-version migrations, or production operational health. Remote
scheduled qualification still needs a provisioned runner and an observed run.
The [execution plan](plans/rebuild-v1.md) tracks remaining request, workflow,
migration, performance, and release gates for the full v1 goal.
