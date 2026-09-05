# igw v1 rebuild execution plan

Status: active. Accepted scope and goal: 2026-09-05.

## Goal and definition of done

Build a reliable Ignition 8.3+ CLI that lets a human or shell agent discover a
Gateway's supported API, inspect its input contract, preview an explicit change,
execute it deliberately, and verify the result using the CLI's own guidance.

Completion requires all of the following, backed by recorded verification:

- A typed execution/workflow core shared by task commands and generic requests.
- A single command definition source for parsing, help, completion, and typed
  machine schemas; one versioned JSON result/error contract.
- Target-bound credentials, bounded cancellation/retries, and atomic streamed
  artifacts. Previews and passive diagnostic checks never send mutating
  requests; explicit collection requires confirmation.
- Complete Gateway-specific OpenAPI catalogs with provenance, immutable
  snapshots, validation, freshness, import/export, diffs, and digest pinning.
- Resource configuration, project/tag import/export, and operational workflows
  with supported preconditions and honest completion/verification outcomes.
- Reproducible disposable-Gateway spec capture, compatibility tests, and
  scheduled reference-update automation. Mock tests are not real-Gateway proof.
- Passing unit/race/build/docs/release-artifact checks and real-Gateway workflow
  acceptance evidence before v1 is described as release-ready.
- Updated command, migration, architecture, and contributor documentation.

Keep exit codes 0/2/6/7, IGNITION_GATEWAY_URL, IGNITION_API_TOKEN, flags > env >
config precedence, explicit --yes for mutations, and release artifact naming.
The first release excludes MCP, persistent RPC, full desired-state deployment,
and fleet orchestration. Pushes, tags, and publication require a separate user
request; build and commit verified slices locally.

## Architecture decisions

Retain Go and standard-library HTTP/filesystem primitives. Add Cobra for a
single command tree and libopenapi plus its validator behind a catalog boundary.
Qualify these libraries against captured Ignition specifications. Workflow
services take typed inputs rather than re-entering a CLI argument parser.

CatalogSnapshot stores provenance and indexed Operation records. Operation
identity is method plus path; operationId is an alias only when unambiguous.
PreparedRequest binds the resolved target, encoded inputs, preview, and
preconditions. Result/Problem are independent of rendering. Keep credentials
and filesystem writes out of catalog parsing.

Expose profile, resource, project, tag, gateway, logs, diagnostics, backup, api,
spec, schema, and completion command groups. Human output is concise; --json
always selects structured output, including parse errors. API JSON stays JSON,
not a string containing JSON. Preserve partial batch outcomes. Help and command
schemas never require connectivity. Generic API requests remain an explicit
escape hatch when a schema cannot represent a supported Gateway operation.

Previews may read current state but never send the proposed mutation. Reviewed
replacement/deletion uses server-supported signatures from the observed state.
Never imply atomicity, successful application, authorization, or validation
where the Gateway does not provide evidence. Diagnostics remain read-only;
scans are explicit maintenance operations. Do not replay uncertain writes.

## API source of truth

The target Gateway's /openapi.json is authoritative for its documented wire
contract; /openapi is the documentation UI. Actual responses and server
validation remain authoritative for state and permissions. Reviewed workflow
policy supplies effects, retry eligibility, and completion checks.

- Partition snapshots by profile and normalized effective URL, including proxy
  base paths. Never implicitly use an unrelated working-directory spec.
- Record source URL, fetch/validation times, Gateway version, available module
  metadata, raw/document/contract SHA-256, and parser/policy versions.
- Preserve raw vendor documents. Keep narrow reviewed corrections separately,
  scoped to affected versions/hashes. Report schema gaps explicitly.
- Disable automatic external-reference retrieval. Spec server URLs cannot
  redirect credentialed traffic away from the selected Gateway.
- Validate before atomic promotion; retain last valid snapshots on refresh
  failure. Coordinate concurrent writers and use unique temporary files.
- Reads/discovery have a 24-hour refresh interval with explicit stale metadata.
  Schema-assisted writes revalidate within the invocation. An explicit
  --allow-stale-spec can use a validated snapshot associated with that target;
  bundled references never silently authorize writes.
- Pins fail on contract mismatch. Use HTTP validators when available and hashes
  otherwise. Offline inspection uses identified snapshots/reference bundles.
- Capture clean reference Gateways using pinned official image digests/module
  sets. Start with minimum supported and latest qualified 8.3 versions, core and
  expanded first-party configurations. Scheduled capture classifies changes,
  tests workflows, and produces reviewable snapshot updates with checksums.
- Keep customer snapshots local. Ship qualified reference catalogs and make
  additional snapshots independently distributable from CLI releases.

## Milestones

1. **Evidence and safety foundation:** establish the test environment, record
   baseline results, add regressions and repair transport/artifact correctness,
   and establish real specification capture. Preserve existing user edits.
2. **New architecture:** introduce command tree, typed results/execution,
   credentials, artifacts, and snapshot storage alongside the old entrypoint.
3. **Discovery and requests:** deliver full schema inspection/encoding,
   validation, genuine previews, freshness/pins, and bounded batch results.
4. **Workflows:** resources first, then projects/tags and operational commands;
   each supported workflow needs captured-contract and Gateway verification.
5. **Cutover:** replace the entrypoint, remove superseded parsing/RPC code,
   provide explicit reversible profile migration, update docs, and qualify
   v1.0.0 artifacts without publishing them.

Each commit must be independently explainable and validated. Reuse useful
existing tests, transport behavior, and release tooling; do not carry forward
tests whose asserted behavior contradicts the accepted new contract.

## Acceptance scenarios

- Different catalogs on two profiles; URL overrides; Gateway/module upgrades.
- Cold/offline discovery, corrupt snapshots, concurrent sync, stale fallback,
  failed fresh-contract writes, and digest mismatches.
- Missing/duplicate operation IDs, references, required parameters, correct
  path/query encoding, schema gaps, multipart and binary bodies.
- No mutation during preview/doctor; signature conflicts; uncertain writes are
  never automatically replayed; accepted versus completed is distinguished.
- Tokens cannot cross origins through absolute URLs, redirects, or spec data.
- Deadlines/cancellation include requests, retries, discovery, and polling.
- JSON parsing/validation/HTTP errors share the same contract; preserve large
  numbers; stream and atomically save --json --out downloads; reject truncation.
- Batch input/runtime failures preserve completed per-item outcomes.
- Resource changes, project/tag round trips where supported, backup export,
  diagnostics collection, and restart verification on disposable Gateways.
- Cold startup, cached operation lookup, and large transfers use real captured
  catalogs/artifacts in performance checks.

## Progress and evidence

- [x] Repository and primary IA documentation assessed; design choices accepted.
- [x] Active goal created with the full outcome and explicit acceptance gates.
- [x] Baseline Go tests/build/race verification.
- [x] Transport credential isolation and atomic-artifact regressions repaired.
- [x] First disposable Gateway capture and real-schema qualification (8.3.9).
- [ ] Complete minimum/latest version and module qualification matrix.
- [x] New typed command/execution architecture introduced alongside legacy CLI.
- [ ] Complete catalog lifecycle and discovery.
- [ ] Validated task workflows and end-to-end verification.
- [ ] Migration, cutover, and release-artifact qualification.

Initial environment evidence: no Go executable, no configured Gateway or local
OpenAPI snapshot. Docker's WSL shim reports integration unavailable; docker.exe
also reports the Docker Desktop Linux engine pipe absent. Real-Gateway checks
remain pending while independent local implementation proceeds. The initial
command-doc consistency check passed; full docs lint could not finish without
Go. Seventeen pre-existing script executable-bit changes belong to the user
and must not be staged as part of this work.

2026-09-05: installed the official Go 1.27.1 toolchain in a private build cache
after verifying its published SHA-256. All package tests, all package race
tests, the CLI build, command-doc consistency, and docs lint pass. Origin
isolation regressions cover foreign absolute URLs, redirects, caller redirect
hooks, and default ports. The smoke script builds successfully but stops at
missing Gateway configuration; this is not real-Gateway verification.

Artifact storage now streams into unique private temporary files, publishes
complete transfers atomically, protects existing files unless --overwrite is
explicit, and reports SHA-256/bytes with --json --out. Regressions cover HTTP
failure, broken streams, size limits, invalid inputs, missing confirmation,
concurrent destination creation, and successful JSON/binary exports. Docker
Desktop start reports already running, but status and engine connection still
fail; no disposable Gateway has run yet.

The new internal/catalog package now preserves full vendor documents and
request schemas, validates OpenAPI 3.0/3.1, rejects external references/dialects,
and supports scoped immutable snapshots, conditional refresh, pins, and explicit
stale-write policy. Synthetic fixture tests and race checks cover references,
large numbers, parameter/body validation, concurrent publication, cancellation,
corruption recovery, and target isolation. It remains separate from the legacy
CLI pending command/execution integration. Selected dependencies require Go
1.25.7+; docs and go.mod now declare that minimum.

The development entrypoint cmd/igw-next now uses one Cobra command tree,
internal/execute preparation/execution, and the igw/v1 result contract. It
supports profile inspection, full catalog discovery/import/export/diff, typed
flag schemas, schema-assisted requests, explicit raw requests, real previews,
read-only doctor, and generated completions. End-to-end HTTP fixture tests
verify no mutation during preview/validation, fresh writes, proxy/path encoding,
JSON precision, downloads, and non-retried uncertain mutations. Scope and
remaining implementation limits are recorded in docs/rebuild-preview.md.

Legacy safety follow-up: doctor no longer dispatches write checks, and legacy
dryRun query forwarding is rejected across call/batch/RPC execution. This
prevents the older entrypoint from presenting mutations as diagnostics or
previews while the new entrypoint is being qualified. Real previews are
available through the development CLI.

Docker's Linux engine is now reachable through docker.exe (29.1.2); the earlier
engine blocker is resolved. A Go contributor capture tool now starts only
uniquely labeled, digest-pinned official containers, commissions a fresh Gateway
with privately copied random credentials, authenticates through the built-in
IdP, captures a stable `/openapi.json`, records the real Gateway version, and
removes its own container/volumes. Captures ran against 8.3.9 with image defaults
and 8.3.0 with an OPC UA whitelist. Existing user containers/volumes were not
modified.

The 8.3.9 fixture is retained as exact gzip-compressed vendor bytes with capture
provenance. It exposes 687 operations/587 paths. Real evidence drove fixes for
SCIM properties named `$ref`, recursive required child arrays, duplicate JSON
keys before normalization, and quadratic indexing of compact JSON. A reviewed
adapter records 343 ignored path `allowReserved: false` annotations and seven
undocumented response placeholders without altering original exports. These
are model-compatibility adjustments, not proof of every workflow.

Newly observed work before workflow qualification: duplicate schema `$id`
values in configuration request bodies cause compilation failures; the CLI now
classifies these as `catalog_schema` rather than invalid user input. Minimum
8.3.0 needs additional parameter-contract review. Live source examples contain
startup timestamps, so stable wire-contract hashing must be separated from
documentation drift. Expanded-document inspection takes about two seconds and
600 MiB peak RSS locally; lazy per-operation models and cancellation during
parsing need investigation. Module inventory, full matrix capture, scheduled
updates, API-token provisioning, and real mutation/verification tests remain
unfinished. The active goal is still the full v1 outcome.

Workstation recovery follow-up: the incident and protections are recorded in
`docs/development-safety.md`. After the user approved an 8 GiB validation
budget, the complete race suite passed with a measured 3.26 GiB cgroup peak.
The real catalog fixture, compatibility rules, and a CLI regression that blocks
uncompilable schemas before any mutation are committed. Both CLI builds and
docs checks passed; the legacy smoke script still lacks a configured Gateway
token and is not real workflow evidence.

The capture helper now uses an exclusive engine-wide container name with a
unique ownership label, verifies configured and kernel-applied memory/swap/
CPU/PID limits, requires the guarded validation scope, and bounds individual
Docker calls. It wraps the official entrypoint in an in-container deadline and
removes the container before schema parsing. Unit/race tests cover admission,
foreign-ID/owner refusal, canceled cleanup, and failed resource checks. A live
lifecycle probe on the pinned 8.3.9 image verified exclusive admission, applied
limits, lifetime termination (exit 124 after 6.13 seconds for a five-second
probe), and removal. No WSL or Docker Desktop restart was used.

The guarded 8.3.9 capture then completed against the same pinned image at
2026-09-05T13:26:00Z, authenticated successfully, retained all 687 operations
and the same 350 reviewed adjustments, and removed its container before parsing.
Its raw SHA-256 is
`94e537dab80f0c9ba93b8cfe8b9c93754fa6e7e6b2127e6dbbc03500f25e51b3`;
the local receipt and exact bytes are in `bin/capture-8.3.9-bounded/`.
The earlier committed fixture remains the reference. Comparing its JSON with
the new capture found 56 reordered `oneOf` arrays, four reordered `enum` arrays,
and the two example timestamps. This expands the stable-hashing work beyond
ignoring documentation fields; reference semantics and ordered input arrays
must remain intact. This is live capture/lifecycle evidence, not yet
authenticated CLI workflow acceptance.

Stable identity follow-up: the second 8.3.9 capture is now retained alongside
the first. Versioned `igw-contract/1` projection gives both the same contract
hash while separate raw and document hashes expose all original differences.
It preserves constraints, exact number spellings, duplicate alternatives,
ordered instance/tuple arrays, extensions, and reference targets. Version 2
receipts retain provenance; version 1 receipts are checksum-verified and
requalified without changing historical bytes or Gateway verification times.
Legacy pins require explicit review and replacement. The development CLI adds
compact `spec inspect --summary` and separates document drift from contract
changes in `spec diff`. Regression tests cover fresh-write pins across benign
drift, rejection of changed constraints, reference positions, and corrupt or
unrecognized identity receipts.
Full unit and race suites, all three contributor/CLI builds, command-doc
consistency, and docs lint passed with the bounded runner. The race run's
observed cgroup peak was 2.62 GiB. Resource schema qualification, task workflows,
and the remaining full v1 gates are still pending.

Resource schema follow-up: policy `ignition-openapi/2` removes duplicated `$id`
keywords only from identical, reference-free primary/backup settings variants
in the observed resource POST/PUT shape. It retains assertions and refuses the
whole operation if any variant has references, anchors, dialect changes, nested
identifiers, or mismatched definitions. Both real captures qualify 288 such
occurrences (638 total adjustments); their original bytes and all identities
remain unchanged. Tag-provider schemas still require separate review. Real
resource mutations and completion verification remain the next workflow gate.
The full unit suite, catalog/CLI/capture race tests, all three builds, and docs
checks passed. The observed race cgroup peak was 3.93 GiB within the 8 GiB cap.
The read-only legacy smoke script builds but exits 2 at doctor with an empty
Gateway URL/token; it remains unavailable as live acceptance evidence. No
WSL or Docker Desktop lifecycle/configuration commands were used in this slice.

Authenticated API qualification: the contributor test harness now provisions a
dedicated security level and temporary API token inside its fresh container.
Security singleton changes carry observed signatures, retain administrator
permissions, and refuse public/unexpected policies. The full header credential
is `name:key`; the generator's key component alone is insufficient. Runtime
tokens remain opaque and unchanged by the CLI. README and doctor/HTTP hints no
longer claim that HTTP 403 proves authentication succeeded.

A real 8.3.9 run through `nextcli.App` confirmed authenticated catalog acquisition,
basic-schedule preview with independently observed absence, creation/readback,
description-only update preserving omitted configuration, stale-signature
rejection with unchanged-state verification, and deletion/readback. The stale
signature returned HTTP 500 rather than 409/412. The Gateway contract hash
matched the two committed captures despite different original document bytes.
API token material stays in process memory; acceptance receipts identify the
image, observed version, test binary, catalog, checks, and successful cleanup.
The final run passed all 13 assertions in 101.78 seconds, including denied
anonymous and bare-key access; its receipt is committed at
`internal/testgateway/testdata/ignition-8.3.9-basic-schedule.json`. Full unit and
race suites, all three binary builds, command-doc checks, and docs lint passed
under the 8 GiB guard. Legacy smoke rebuilt successfully but still exited 2 at
doctor because the default Gateway URL/token are unset. No owned test container
remained after acceptance.

The next boundary selected after generic API acceptance was a typed workflow
invocation owning one fresh target catalog across read/prepare/write/verify,
plus resource type discovery, list/get, and create/update/delete commands. Resource workflows
must check the response's `success` field, supported signatures, and observed
state; generic HTTP acceptance alone is insufficient. They must preserve
uncertain outcomes and avoid replay, including the observed HTTP 500 conflict
case. The following slices implement that resource boundary; project/tag
workflows, the minimum-version matrix, migration/cutover, and the other full
v1 gates remain unfinished.

Workflow invocation foundation: `execute.Scope` now owns one catalog, target,
credential, and cancellation context across serial steps. Write scopes refresh
before state reads and reuse that validated snapshot for preparation, mutation,
and readback. Steps cannot change target/policies, issue raw requests, or upgrade
a read scope to write access. Closing cancels an in-flight request and releases
the catalog. Specific unit/race checks cover catalog fetch counts across repeated
workflows, request validation, denied escalation, and cancellation; existing
generic-request CLI tests continue to pass.

Named-resource implementation now exposes type discovery, describe/list/get,
and create/update/delete over the shared scope. Change bodies contain writable
fields only; the service owns identity, collection, array wrapping, and observed
signatures. Update/delete execution requires a reviewed `--if-signature`, then
checks it against current state before sending that same precondition. Preview
reports changed root fields and the validated request digest without values.
Completion requires a success acknowledgement, a matching returned/readback
signature for create/update, supplied-field comparison, preservation of omitted
root fields on update, or observed absence after delete. Generic HTTP 500 does
not become an invented concurrency conflict; uncertain transfers are never
replayed. Singleton and forced reference changes remain generic operations.

The first extended live run passed the existing API contract and resource type
discovery, then failed list validation before dispatch and removed its container.
Offline reproduction identified validator 0.14.0 incorrectly assigning limit
and offset to the absent optional exploded filter. A narrow private validation
view now excludes only provably absent filters while retaining scalar checks
and refusing unqualified supplied/ambiguous filter input. Parser version 6
and captured-catalog regressions record this correction without rewriting vendor
bytes or changing contract identity.

The second extended live run passed all 27 assertions in 137.07 seconds on
pinned 8.3.9, including paginated listing and dedicated resource workflow
completion. Its committed receipt is
`internal/testgateway/testdata/ignition-8.3.9-resource-workflows.json`; the
test-binary hash was checked and no owned container remained. This is a
qualified basic-schedule workflow, not full resource/module/version coverage.
Remaining workflow work includes singleton resources, filter serialization,
project/tag round trips, and operational actions, alongside the existing batch,
catalog distribution/update, minimum-version, migration, and cutover gates.
Full unit and race suites, all three binary builds, command-doc checks, and
docs lint passed after the resource implementation and parser correction. All
local checks used the 8 GiB guard. The separate legacy smoke script remains
dependent on a configured default Gateway; its read-only run rebuilt the binary
and exited 2 at doctor with the unset default URL/token. Real-Gateway acceptance
is supplied by the disposable test receipt above.

Project/tag transfer foundation: opaque uploads now snapshot a regular input
file to private disk storage, compute its size/digest, and stream those exact
bytes without loading the body into memory. The default limit is 1 GiB, with
context checks during copying/reading and deterministic cleanup. Streamed
requests cannot combine inline bytes or enable retries, and content type/body
framing are CLI-managed. Catalog validation accepts streaming only for declared
opaque media without a body schema and reports `declared_transport`; it does
not imply content validation. A 40 MiB CLI transfer test verifies byte identity,
length, credentials, and preview without dispatch. Source-file replacement,
size limits, cancellation, closed streams, and schema/framing bypasses have
specific regressions. Project and tag imports use these opaque wire contracts.

Project/tag workflow follow-up: the development command tree now includes
project list/get/inspect/export/import and tag export/import. Typed services
share one execution scope and invocation deadline across preparation and
readback. Project archives are inspected without extraction, compared by
content manifest, and validated before publishing export files. Replacement
requires a reviewed before-digest and reports the absence of an atomic server
revision precondition. JSON tag verification compares supplied properties and
named children while allowing Gateway defaults. Exact JSON comparison is
shared with resources; ambiguous structures and duplicate keys are refused.
Unknown reports, failed readback, and mismatches cannot establish completion.
Tag report failures inside HTTP 200 return exit 7, preserving known partial
outcomes. XML/CSV and Rename/Ignore currently provide acknowledged acceptance
only, with verification explicitly unavailable.

The real 8.3.9 transfer workflow test passed 38 checks in 174.34 seconds. Its
immutable receipt is
`internal/testgateway/testdata/ignition-8.3.9-project-tag-workflows.json`, for
binary SHA-256
`d6faec44e524b5dd767914409318b46b1e07c3e2c5c7c7d5ba6968ccede0bcc1`.
It qualifies the generic wire contracts plus dedicated project import and
replacement, stale-digest refusal, archive readback, tag JSON import/overwrite,
and preview absence/unchanged-state checks. Duplicate Abort returned a partial
report while the existing memory-tag value stayed unchanged; transport 2xx is
insufficient and Abort does not imply transactional rollback. The Gateway was
removed successfully through exact-owner cleanup. The captured document still
declares a quality-code array although 8.3.9 returns a count envelope; raw
vendor bytes remain unchanged and the discrepancy is documented.

Full unit and race suites, all three builds, command-doc consistency, and docs
lint passed for this slice. Observed validation cgroup peak was 5.06 GiB under
the approved 8 GiB cap. The read-only legacy smoke script rebuilt successfully
and stopped with exit 2 at the unset default Gateway URL/token. The disposable
receipt above supplies real transfer acceptance; no host lifecycle or memory
configuration changes were made. Existing user script-mode edits are preserved.

Remaining v1 work includes Gateway restart verification, bounded batch,
singleton/filter qualification, broader tag/version/module acceptance,
scheduled reference updates and distribution, migration/cutover, and release
artifact qualification. The full goal remains active.

Operational implementation: backups and log databases have bounded atomic
downloads; log queries support pagination, level/logger/search, and absolute
or relative start times. The optional exploded-filter adapter is qualified
for log pagination in parser version 8. Local inspection cancellation now
retains exit 7 instead of being misclassified as usage exit 2 at the CLI root.

The initial real 8.3.9 operations contract run passed 13 checks in 91.23
seconds. Backup export produced a ZIP with 1,592 files; log download produced
a SQLite database. Diagnostics status moved from `Invalid` to `Generating`
to `Valid` and stayed `Valid` after download. Its six-file ZIP passed CRC
reads and matched the reported size. This replaces guessed state synonyms and
the legacy assumption that any positive file size means completion.
The private receipt is `bin/operations-contract-8.3.9.json`, for test binary
`f9c5d4239d2da1ef8dfdf9362e6d4ede3d4204077fe7d3ff9860ce4569832e9a`.

The typed diagnostics workflow confirms generation explicitly, refuses an
already-running job, polls within the invocation deadline, and publishes only
after download size and a second ready-state observation agree. It reports
`gateway_latest` correlation because no job ID or server digest exists; it
cannot establish exclusive job ownership or distinguish a same-sized bundle
created concurrently. Tests cover stale sizes, busy state, unknown reports,
transient polling errors, cancellation, publication failure, and preservation
of existing files.

Dedicated operational qualification passed 23 checks in 114.20 seconds on the
same pinned image, recorded in
`internal/testgateway/testdata/ignition-8.3.9-operational-workflows.json` for
binary `3b39df8c17a2df7cc16e725591ac1c4396d876c8269ea939a796112f0d292e11`.
It includes filtered log pagination, backup/log downloads, collection previews
with unchanged state, and size-checked collection/download. Repeated generation
returned `Valid`, and the subsequent archive SHA-256 matched the original.
The first workflow run correctly refused this unqualified acknowledgement;
the implementation now records `generationState` and warns that a new job is
unproven when the Gateway acknowledges an already-ready bundle. The API's
"generate new" description does not establish freshness on every invocation.
Owned-container cleanup passed. Full unit/race suites, all three builds,
command-doc consistency, and docs lint passed. The validation cgroup peaked
at 4.13 GiB within the approved 8 GiB cap. The read-only legacy smoke script
rebuilt but stopped at doctor with exit 2 because the default Gateway URL/token
remain unset. No WSL or Docker Desktop lifecycle/configuration changes were
made, and user-owned script-mode edits remain untouched.

The contributor tool now resolves official `8.3` channel/patch tags through
Docker Hub's registry API without invoking Docker. It preserves exact index
and selected `linux/amd64` manifest bytes, verifies registry and descriptor
digests/size/media types, and emits a dated immutable image receipt. Fixed HTTPS
endpoints, no redirects, private pull-scoped credentials, response/deadline
bounds, and new-only output directories constrain this first update stage.
Tests cover OCI/Docker indexes, ambiguous/foreign platform descriptors, corrupt
or truncated manifests, rate limiting, token/transport redaction, cancellation,
and preservation of previous candidates.

Live resolutions at 2026-09-05T15:46:27Z and 15:46:33Z are retained locally in
`bin/resolve-8.3-20260905/` and `bin/resolve-8.3.0-20260905/`. The `8.3` channel
still points to the qualified 8.3.9 index
`sha256:28bd6b320157ec8dbbe465d0cd7c9f0ababfda4ff01c0bab982c0522a7b4eba2`.
The minimum tag resolves to
`sha256:9fa22bb89a3004b95c6d7b281f690f9122ec53e4509f762fb23e10d5306eeafb`.
These receipts prove registry resolution only. Platform/runtime verification,
scheduled qualification, current-parser workflow receipts, candidate review
packaging, and distributable offline references remain required.
The full unit suite, focused resolver/contributor race tests, all three builds,
command-doc consistency, and docs lint passed for this slice. The read-only
registry checks started no container and changed no host settings.

The capture guard now verifies a local `linux/amd64` image configuration digest,
specifies that platform at creation, and checks the container's image identity
before startup. Capture v3 receipts record the observed image ID/platform and
cleanup; workflow v2 receipts add the same provenance. Historical receipts stay
unchanged. The lifecycle test can now publish an atomic checksum-bound receipt
after all containment and independent removal checks pass.

The updated guard passed its real 8.3.9 lifecycle probe in 8.52 seconds, with
timeout exit 124 after 6.7344 seconds, no OOM, and verified removal. The receipt
is `internal/testgateway/testdata/ignition-8.3.9-lifecycle.json`, for test binary
`04c96f08dc988cac3bd05a661c8f05d9ba3edf2497e813c9486a624bdcee7e13`.
Its observed image ID
`sha256:f28a0c5a7a85dab32f0f0a04a80bc4dfd00a27de12f899bef979ffb9cce967fd`
matches the config digest in the earlier registry resolution's selected
`linux/amd64` manifest. A fresh full capture then passed at
2026-09-05T15:51:56Z, recorded in `bin/capture-8.3.9-platform-20260905/`:
687 operations, current parser version 8, 638 reviewed adjustments, and the
same contract hash as both qualified reference fixtures. Raw SHA-256 is
`89dd607e3b7738ef865188fa1de1172dc338c7005d2143ffcc6d42b245127ec5`.
Capture cleanup and an independent no-leftover-container check passed.
Full unit/race suites, all three builds, command-doc consistency, and docs lint
passed. The validation scope's observed memory peak was 4.39 GiB within its
8 GiB cap. No host settings or WSL/Docker Desktop lifecycle actions changed.

Document comparison now lives in `internal/catalog.Compare`, shared by
`spec diff` and the forthcoming reference update packager. It reads immutable
catalog indexes directly, avoiding redundant full-document/operation copies,
and preserves the existing JSON comparison fields with sorted non-null lists.
Regression checks distinguish annotations, inherited path constraints, shared
large-integer constraints, and added/removed routes. Full unit/race/build/docs
gates passed. Comparing the two guarded real 8.3.9 captures also passed:
different document hashes and six changed operation documents, no added or
removed routes, and equal contracts under `igw-contract/1`. The local result is
`bin/image-reference-comparison.json`; this is not backward-compatibility proof.

Module capture now observes both documented healthy and quarantined collections,
with bounded pagination, count/offset checks, unique IDs, and three matching
inventory/document observations. New capture and workflow receipts retain the
actual module IDs, versions, states, startup actions, and pending-upgrade flags.
The timestamp is separate from the versioned inventory hash. Collection names
are not interpreted as health assertions; exception details are omitted.

The real 8.3.9 capture passed at 2026-09-05T16:04:53Z, retaining 687 operations
and the qualified contract hash. It observed 32 first-party modules, all ACTIVE,
enabled on startup, and without pending upgrades or quarantined entries.
Module versions have independent major numbers (for example, Perspective 3.3.9
and Vision 12.3.9). The receipt is
`internal/testgateway/testdata/ignition-8.3.9-module-inventory.json`; exact vendor
bytes remain in `bin/capture-8.3.9-modules-20260905/`. Raw SHA-256 is
`2c40e5569b9cdd8acfb2a738f83c45192c3dc83b3a34b6fdfd1445a9e3705375`, and inventory
SHA-256 is `8adf3d3f453ec516a9d29976c94cfb2696c89b85b1f05b35be4c4bc71392d4dd`.
Owned-container cleanup and independent absence verification passed. Full unit
checks, focused contributor/resolver race checks, all builds, and docs checks
passed. Legacy smoke still stops at unset default Gateway configuration.

## References

- [IA Gateway API documentation](https://www.docs.inductiveautomation.com/docs/8.3/platform/gateway/openapi)
- [IA resource metadata](https://www.sdk-docs.inductiveautomation.com/docs/8.3/programming-for-the-gateway/storing-data-using-resource-collections/resourcetypemeta/)
- [IA endpoint considerations](https://docs.inductiveautomation.com/docs/8.3/appendix/scripting-functions/system-config)
- [Official Ignition container](https://docs.inductiveautomation.com/docs/8.3/platform/docker-image)
- [Durable plans and acceptance criteria](https://developers.openai.com/cookbook/examples/codex/iterating-development-workflows-with-codex#agents-and-plans)
