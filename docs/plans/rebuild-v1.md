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
- [x] Complete initial minimum/latest version and module qualification matrix.
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

Saved image resolutions now have an offline loader and full-chain validation.
It verifies regular-file/size limits, the supported receipt version and source,
image/tag/platform/timestamp provenance, both exact digests, descriptor size and
media type, and the platform link before exposing the image config digest.
Publication reuses the same validation. Tests reject forged metadata, corrupt
manifests, incomplete directories, non-regular files, and canceled reads; full
unit checks, focused race checks, all builds, and docs checks passed. This gives
the update packager a reusable check against the image ID observed by Docker.

The contributor now assembles independently distributable `igw/reference/v1`
bundles with `igw-capture qualify`. Its versioned policy requires matching image
configuration, test-binary checksum, current parser/contract, observed module
inventory, owned cleanup, and the complete named positive/negative workflow
checks. It reparses exact capture/baseline documents, compares them through the
catalog core, preserves exact compressed vendor data and original acceptance
receipts, and publishes a checksummed manifest last into a new directory.
The offline reader verifies fixed payload paths, checksums, gzip limits, and
vendor identities. Qualified references cannot establish a live target's
contract or authorize writes; checksums do not authenticate a publisher.

Fresh acceptance on the same 8.3.9 image passed all four runs using binary
`63aaa19c9a60f3872419f3cadbd75102e21f440b0863f72d32fb7f67c560a800`:
10 lifecycle checks in 10.25 seconds, 27 resource checks in 135.40 seconds,
38 project/tag checks in 171.56 seconds, and 23 operational checks in 115.90
seconds. All three workflow inventories match the capture's 32-module hash,
and all cleanup checks passed. The first reference is retained in
`internal/reference/bundles/ignition-8.3.9-defaults` (about 792 KiB), with the
same 687-operation contract and explicit workflow qualification scopes.
The local assembly result is `bin/reference-qualification.json`. Full unit
checks, focused reference/builder/contributor race tests, all builds, and docs
checks passed. Tests reject mismatched/partial receipts, absent containment,
false completion of a partial tag import, corrupt or substituted payloads,
path traversal, incomplete publication, and replacement of a previous bundle.
Scheduled execution, runtime reference-selection commands, and the complete
minimum/latest/module matrix remain required for the full v1 goal.

The development binary now embeds the complete qualified 8.3.9 reference.
`spec references list`, `inspect`, and `export` verify and preserve bundles;
`api list` and `describe --reference` select either an embedded name or a local
directory. Both storage paths use the same checksum, gzip, current-parser, and
identity checks. Exports retain exact manifest/payload bytes, require a new
directory, and check the reviewed identity before publication. Explicit pins
are enforced for inspection, export, and discovery.

Reference results use `meta.reference` with image/module/qualification evidence
and no target receipt or freshness assertion. The qualification parser and
current inspection parser are distinguished. Tests forbid configuration reads,
environment credential access, network requests, and target cache writes; they
also reject the flag on all request paths. The shared JSON helper now returns
an existing typed usage error without importing result rendering, preserving
exit-code behavior and keeping the reference/result dependency acyclic.

Full unit tests, focused reference/builder/CLI race tests, all three builds,
and docs checks passed (`bin/reference-runtime-final-gates.log`). Regressions
cover original-byte export, changed identity, checksum corruption, no clobber,
output failure, cancellation, pins, source selection, and human provenance.
Standalone binary verification from an empty working directory passed listing,
inspection, export, embedded/local discovery, and generated flag schemas;
evidence is in `bin/reference-offline.yys37X/` and
`bin/reference-runtime-binary-check.log`. The real Gateway-info description
remains a compact 12,672-byte JSON result including reference provenance.
Legacy smoke remains unavailable without a configured default Gateway; the
fresh disposable-Gateway workflow qualification above is separate evidence.
Scheduled qualification, additional version/module profiles, and the remaining
workflow/cutover/release gates still keep the full v1 goal active.

The reference updater now has a serialized contributor coordinator and a
weekly/manual Actions workflow. It verifies admission/engine controls, records
source and Go toolchain identity, builds one test binary with trimmed paths,
resolves the moving tag once, and uses that immutable image for lifecycle,
capture, and all three workflow runs. The existing Go qualifier assembles the
candidate only after every receipt passes. Per-stage logs and a versioned run
receipt survive failures; no host recovery, stage retry, promotion, or publication
occurs. A clean-source option checks both commit and worktree before building
and before final qualification. Ambient live-test opt-ins are cleared and all
temporary build/test data remain under the private run directory.

Coordinator unit tests pass all stage-failure boundaries, serialized execution,
pinned image/binary reuse, unsafe engine/container admission, changed sources,
no-clobber output, and owned-process cancellation. Actionlint 1.7.12 validated
the CI and new workflow; docs checks passed. These are orchestration checks,
not live Gateway qualification of the coordinator. A clean-checkout real run is
next. The remote workflow remains disabled until a dedicated constrained runner
is provisioned and `IGW_REFERENCE_RUNNER_ENABLED=true`; no remote activation or
push was performed. The workflow produces 30-day review artifacts and does not
replace durable Git/binary references. Setup, schedule limitations, manual
fallback, provenance, and review instructions are in `docs/reference-updates.md`.

The complete real coordinator run passed at 2026-09-05T17:06:55Z from clean
commit `4734207735bae92b9d4a7b89ee08f668b9429288`. All 18 stages passed in 755.43
seconds, including cold trimmed-path builds, fresh moving-tag resolution, pull,
lifecycle, capture, three workflow suites, unchanged-source checks, and final
bundle qualification. Source and toolchain provenance identify Go 1.27.1 on
Linux amd64 with CGO enabled. All four acceptance receipts identify binary
`02a8334ff4b6a0e3785ff735fc6f4993c4b555e0003fea045f146145b1e4b71f`.
The lifecycle/resource/project-tag/operational suites passed 10/27/38/23 checks;
resource, transfer, and operational runs took 133.51, 172.55, and 112.98 seconds.
All cleanup checks passed, and an independent post-run Docker query found no
qualification container. The isolated source worktree remained clean.

Original coordinator and full reference evidence are retained in
`internal/referencebuild/testdata/ignition-8.3.9-update/`; the local run and stage
logs remain in `bin/reference-update-20260905/`. The observed 8.3.9 image,
32-module inventory hash, and 687-operation contract match the prior reference.
New raw SHA-256 is `2e9f5fc8cf03b4d84d7aebd450efcb43a8773c9df9f53e9dcfca460a40b2c152`;
the document SHA-256 is `acddff10b785ab4a10487ab32169f5af7792a1b8e7bb2004fe1fd1fd42527b11`.
The retained candidate passed current CLI checksum inspection and API discovery,
and docs checks passed. The existing runtime reference was preserved. This
proves the local coordinator end to end; remote schedule activation, the broader
version/module matrix, remaining workflows, and v1 cutover/release gates remain
unfinished. No push, tag, publication, host recovery, or memory-setting change
was performed.

The minimum-version review now recognizes the exact 8.3.0 Gateway-relative
EULA identity and two parameter defects, with parser version 9 and compatibility
policy `ignition-openapi/3`. Sixteen entity-section/SCIM parameters incorrectly
mark a selected path placeholder optional. Their private model requires those
placeholders and preserves every supplied value constraint; it never infers an
alternate route. The script-cancellation ID has no schema. Discovery preserves
that original omission and exposes the gap, while schema-assisted execution
and preview return `catalog_schema`/2 before sending an operation. Explicit raw
DELETE still requires `--yes`. Tests cover preserved raw descriptions, retained
value constraints, rejected unreviewed shapes, missing placeholders, unrelated
operation availability, corrected subsequent schemas, and no mutation during
either rejection path.

The full retained 8.3.0 document now passes the OAS metaschema with 326 reviewed
adjustments: 222 path annotations, seven empty responses, 80 duplicate unused
resource IDs, 16 selected-path requirements, and one undocumented ID. Full
model resolution still fails on `#/$defs/key` and `#/$defs/keyVariant` in the
keyboard-layout resource. The document contains 24 such references across
config/backupConfig schemas in four operations, with nested definitions and no
schema `$id` establishing their [reference base](https://json-schema.org/understanding-json-schema/structuring).
No corresponding references are
present in the retained 8.3.9 document. The CLI rejects the unresolved model;
the historical 8.3.0 receipt remains unvalidated and has not been promoted.
Failed inspection and model-resolution details remain under
`bin/minimum-parameter-*.json` and `bin/minimum-parameter-model-errors.txt`.

Both existing 8.3.9 fixtures passed current parser inspection with unchanged
raw, document, and contract identities, 687 operations, and 638 adjustments.
Only their current-parser qualification files were regenerated; original
capture and reference-bundle receipts remain unchanged. Resolving the separate
keyboard-layout defect, fresh minimum-image containment/capture/workflows, and
the other matrix and v1 delivery gates remain required.

The full unit suite, focused catalog/execution/CLI race checks, both binary
builds, command-doc consistency, and docs lint passed under serialized bounded
validation. Logs are in `bin/legacy-parameters-{unit,race,docs,smoke}.log`.
Legacy smoke built successfully, then stopped at `doctor` with exit 2 because
the default Gateway URL/token configuration is empty; no live smoke success is
claimed. No Gateway containers or host-setting changes were needed for this
slice.

The keyboard-layout reference defect now has a constraint-preserving adapter.
Expanding the two definitions from the retained 8.3.0 config schema produces
exactly the inline config schema in the retained 8.3.9 document; the comparison
is recorded in `bin/keyboard-schema-comparison.json`. Parser version 10 and
policy `ignition-openapi/4` expand eight primary/backup schemas across the four
reviewed operations. The operation's complete reference graph must match the
observed acyclic shape, both configs must match and use at most 64 KiB each, and
no unknown reference, scope, identity, dialect, or `$ref` sibling may be present.
All assertions and source evidence survive. Descriptions explain the correction
once per operation while retaining both adjustment pointers.

The complete historical minimum document now passes model resolution with 446
operations and 334 adjustments. Raw SHA-256 remains
`de174add02ef1557f603edae7c7802ea298bce35301b9675b5b620f4247b2e9c`,
document SHA-256 is
`90ab3938159692f3371fba9f3becde9a899ee28fc339ec906f92fa25152c297d`,
and contract SHA-256 is
`8f6578c201fb31c718c778b2b5593e3f44d09bb86199c17f7a5c1991c8567c08`.
`bin/keyboard-minimum-inspection.json` records the current inspection. This is
parser evidence, not a replacement for the original unvalidated capture receipt
or current containment and live workflow qualification.

Regression tests use an extracted vendor keyboard schema and cover all four
embedding positions, both primary and backup validation, valid nested keys,
invalid key types/accents/system values, missing fields, forbidden properties,
newly supplied value constraints, and atomic rejection of unreviewed graphs.
Both 8.3.9 parser fixtures were requalified with identical hashes, operation
counts, and adjustment counts. Fresh minimum-version Gateway qualification is
the next acceptance gate; the complete v1 goal remains active.

Full unit tests, focused catalog race tests, the development CLI build, and
documentation checks passed under the workstation guard. Unit and race logs
are in `bin/keyboard-{unit,race}.log`. Legacy live smoke still lacks a configured
default Gateway; the fresh guarded coordinator run will provide separate
minimum-version acceptance evidence.

The first fresh default-module minimum pipeline ran from clean commit
`3d390e8f49eb97a8616701dc44e1c184fce06e9b`, using Go 1.27.1 and pinned image
`inductiveautomation/ignition@sha256:9fa22bb89a3004b95c6d7b281f690f9122ec53e4509f762fb23e10d5306eeafb`.
Image config is `sha256:3ccb0dd03f8237048a0cc8554abd29a25429d684be85bbbe0c8bd1c42a95be85`.
The lifecycle probe passed ten checks in 6.151273834 seconds, including no OOM
and independent cleanup. Its test binary SHA-256 is
`479374537118a769d5435cc9f8f478068bf21d5514883aa4df86db2f1272590d`.
Capture observed Gateway `8.3.0 (b2025091510)` and 32 healthy active first-party
modules, inventory hash
`f8205842deae9087a570d3c30f54f8ed1d414f65d429bd4eefd3040493ae256f`.

The coordinator stopped at capture-time OAS validation, before any workflow
suite. EAM marked its boolean `running` path parameter optional; SFC omitted
both chart path parameter schemas. The complete 672-operation document and
failed receipt were retained; cleanup passed and an independent Docker query
found no qualification containers. The run was terminal and its source checkout
clean before that owned worktree was removed. No retry, host recovery, or
resource-limit change occurred. Original run logs remain in
`bin/reference-minimum-20260905/`.

Parser version 11 / policy `ignition-openapi/5` now handles those exact module
parameters. EAM retains its boolean constraint and requires the selected path
placeholder. SFC discovery reports the missing schemas, and reads/previews fail
with `catalog_schema`/2 before sending an operation; no type is inferred from
8.3.9. Updated parameter schemas resume normal validation when supplied. The
new fixture's full catalog and resource/project/log request regressions pass.

The exact compressed document, original failed capture/coordinator/lifecycle
receipts, and separate current qualification are retained under
`internal/catalog/testdata/ignition-8.3.0-defaults/`. Original raw SHA-256 is
`ca661fb27a4f27dc262d53164676e11429d33f74dfabc95e196f3c61b712f765`,
document SHA-256 is
`224a230ee101293e1cd6f05db54dd825420c5dd6cf94bca677066a3114b28cde`,
and contract SHA-256 is
`a87917c8a2ff264ab77def23e9fa619af23cd8027e525b7d42ff2bdfe933b644`.
Current inspection reports 672 operations and 648 adjustments. Existing 8.3.9
fixtures retain their identities and counts. The minimum fixture remains
parser evidence, not a qualified offline reference: a current-binary clean
pipeline and real workflow acceptance remain required.

Full unit tests, focused legacy catalog/CLI race tests, the development CLI
build, complete captured-document checks, and docs checks passed under the
bounded runner. Logs are in `bin/minimum-module-{unit,race}.log` and
`bin/minimum-default-captured-test.log`. Legacy smoke's default Gateway remains
unconfigured; real minimum-version workflow acceptance is still pending.

The second clean minimum pipeline (`40a8502`, parser 11) passed the ten-check
lifecycle probe, captured all 672 operations with 648 reviewed adjustments, and
passed all 27 resource workflow checks. It ran from 17:48:14 to 17:54:09 UTC on
2026-09-05. The transfer suite passed its initial project API round trip, then
failed before sending tag import: the 8.3.0 catalog contains neither tag import
nor tag export. No operational suite or final qualifier ran. Original receipts,
registry evidence, exact compressed OpenAPI bytes, and transfer stdout are
retained in `internal/referencebuild/testdata/ignition-8.3.0-incomplete/`.
All disposable containers were removed and an independent Docker query found
none remaining. No host recovery or automatic retry occurred.

The second raw hash is
`b61c848935cf8717587c089be4566dde3a695f27a79be4271fcd23d90d0386e3`;
its policy-1 contract hash differs from the preceding capture despite identical
image and module inventory. Comparing the original documents found only 56
`oneOf` reorderings, four `enum` reorderings, and two example timestamps. The
projection freezes the entire document when the old keyboard references do not
resolve at the root. The next identity policy must use the reviewed reference
scope and retain explicit verification of historical hashes and bundles. It
must not silently reinterpret policy 1 or hash inferred parameter constraints.

Workflow availability must come from the actual catalog. Minimum qualification
must test clear refusal for missing APIs and record supported scopes, without
counting absent tag round trips as passing. The separately qualified 8.3.9 tag
workflows remain required. This evidence narrows the next two implementation
slices; the full compatibility matrix and v1 delivery goal remain unfinished.

Contract policy `igw-contract/2` and parser version 12 now resolve the identity
defect using the keyboard adapter's shared read-only qualification. Only the
reviewed paired schemas establish additional local reference scopes. The hash
retains the original definition targets, constraints, and unknown references;
it never incorporates inferred parameter schemas or other model placeholders.
Both 672-operation minimum captures now have current contract SHA-256
`30d6a91031f2495477e53c38bfdec7dfebb0157820a661c7e6a387c839b5f800`.
Both 687-operation 8.3.9 captures have current contract SHA-256
`17b4ace179c02c79f4af74485773cb5afc4c35b179f7fe020c4326cb0dacceea`.
Original bytes, raw/document hashes, and adjustment counts remain unchanged.
The repeated minimum capture is now a parser regression fixture alongside
its separately retained incomplete workflow evidence.

Policy 1 remains an explicit historical verifier. Cache loads verify it before
deriving current identity, preserve prior parser/hash/policy history, warn about
pin migration, and leave original receipts and Gateway timestamps untouched.
Historical reference bundles remain readable and exportable with their recorded
policy and pins. API discovery reports the current `inspectionCatalog` identity
separately from immutable qualification metadata. A current-policy hash cannot be
silently substituted for a recorded reference hash, and new reference assembly
still requires current-policy live receipts. No historical acceptance evidence
was promoted or rewritten.

The full unit suite and focused catalog/reference/CLI race checks passed under
serialized bounded validation. All four complete captured-document regressions
passed, including exact historical-hash verification and repeated-capture
stability. Tests also cover unknown reference scopes, preserved new assertions,
tampered/unknown policy receipts, immutable migration history, old-pin refusal,
and reference policy relabeling. Logs are in
`bin/identity-policy-{unit,race,qualification}.log`. Capability-aware workflows,
minimum-version live acceptance, and the remaining v1 gates are still required.

The development CLI and canonical legacy binary built successfully. Command-doc
checks, docs lint, and the added old-policy assembly-refusal regression passed.
Legacy smoke stopped at `doctor` with exit 2 because the default Gateway URL and
token are unconfigured; no live smoke success is claimed. Logs are in
`bin/identity-policy-{docs,smoke}.log`. No containers or host-setting changes were
needed for this identity slice.

Tag workflow capability definitions now own the exact API prerequisites used
by both discovery and typed execution. `api capabilities` reports `advertised`
or `unavailable`, required/missing operation keys, and the selected catalog's
provenance. It supports target-cache offline inspection and explicit qualified
references without assigning them live authority. These first definitions cover
tag export, acknowledgement-only import modes, and verified JSON import;
availability reporting for other workflow families remains to be added.

`execute.Scope.Require` checks prerequisites against the invocation's shared
snapshot without an operation request. Missing operations return `capability`/2
with structured guidance. Verified JSON imports and their previews require both
import and export, preventing a write when the readback route is absent.
XML/CSV and Rename/Ignore still require only import and report acknowledgement
without independent verification. Tag export now uses a typed workflow with the
same requirement check and existing atomic streaming behavior. Generic API
requests retain their explicit method/path operation semantics.

The qualifier still requires the original complete project/tag check set.
It must next record actual supported scopes and verify refusal for unavailable
tag workflows on 8.3.0, while retaining full tag round trips for 8.3.9. No missing
API has been counted as a passing round trip, no undocumented fallback was
introduced, and no new live acceptance is claimed for this slice.

The full unit suite and focused catalog/scope/tag/CLI race checks passed under
the bounded runner. Tests cover exact operation keys, closed/canceled scopes,
all import format/collision-policy prerequisite choices, no dispatch or artifact
on missing prerequisites, preserved opaque acknowledgement and streamed export,
human guidance, target-cache offline discovery, and isolated reference selection.
Both real captured Gateway documents yield the expected tag capabilities. The
built development CLI also successfully inspected capabilities from the bundled
8.3.9 reference. Logs are in `bin/capability-{unit,race,build-docs}.log`, with
compiled-CLI output in `bin/capabilities-reference.json`.

Both binaries built, and command-doc consistency and docs lint passed. Legacy
smoke stopped at `doctor` with exit 2 because the default Gateway remains
unconfigured (`bin/capability-smoke.log`). These local checks ran serially with
the existing memory limits; no container or host-setting change was needed.

Qualification policy `igw-reference-workflows/2` now separates actual workflow
coverage from missing API capability. Shared tag prerequisites live in
`internal/workflow`, below execution and reference reading, so CLI discovery,
typed workflows, and qualification use the same definitions without circular
dependencies. The policy handles the two observed shapes: both tag transfer
routes advertised, or both absent. Partial tag API surfaces explicitly require
additional coverage before qualification.

Transfer receipts use version 3 with capability assessments, error kinds, exit
codes, and observed operation-request counts. Every project check runs in both
shapes. Advertised tag APIs require all 38 historical transfer checks plus
capability discovery. Absent APIs require the project checks, capability
discovery, and three explicit tag import/preview/export refusals. The HTTP
transport counts actual requests independently of result metadata; refusal
requires `capability`/2, zero operation requests, and no download artifact.
Resource and operational receipts remain version 2.

Assembly derives capabilities from the verified captured document and compares
them with the transfer receipt before accepting the exact required check set.
The manifest lists unavailable `tags/memory-json` separately in
`qualification.unavailableScopes`, excluding it from successful workflow scopes.
Opening a new-policy reference compares those claims with the original catalog.
Historical policy-1 bundles retain their original receipts and remain readable;
no old receipt was rewritten or promoted. A fresh clean minimum-version pipeline
remains necessary to establish live acceptance under the new policy.

The full unit suite, focused reference/qualifier/request-observer/tag/CLI race
checks, development and contributor builds, and command/docs checks passed
under serialized bounded validation. The first unit run exposed a missing
synthetic token in the new request-observer fixture; correcting that fixture
passed both its focused regression and the complete rerun. Logs are retained in
`bin/qualification-capability-{unit-final,race,build-docs}.log`. Legacy smoke
built successfully and stopped at `doctor` with exit 2 because the default
Gateway URL and token remain unconfigured
(`bin/qualification-capability-smoke.log`). No new live acceptance is claimed
for this implementation slice; no container or host-setting changes were needed.

The fresh clean minimum pipeline from `ef55762` subsequently passed all 18
stages under contract policy 2 and workflow qualification policy 2. It ran from
18:44:21 to 18:53:49 UTC on 2026-09-05 in 568.08 seconds using Go 1.27.1. The
lifecycle/resource/project-tag/operational receipts contain 10/27/27/23 checks.
All project workflows ran; the three absent-tag checks each observed zero
operation requests, `capability`/2, and no output artifact. Backup, logs, and
diagnostics passed in another fresh Gateway. Every container was removed,
independent Docker inspection found none remaining, and the clean source
worktree was removed after verification. No host recovery or limit change ran.

The original run receipt and complete candidate are retained in
`internal/referencebuild/testdata/ignition-8.3.0-policy2/`. The capture has 672
operations, 648 reviewed adjustments, and the same 32-module inventory/current
contract identity as the previous minimum captures. Its raw hash is
`b8ea0090102ad0018f8fde8f5e1120f0e090b59d54a5f74182a528cf0ef435db`
and document hash is
`899fb46e172399bb2e99cc753ace48ddea3401a5a499cd1e509a86be7e48999a`.
Five workflow scopes are qualified; tag round trips remain explicitly
unavailable. The 8.3.9 comparison requires review and is not a claim of full API
backward compatibility. The new retained-evidence regression verifies original
catalog/image identities and all acceptance receipts offline without renewing
timestamps. No runtime bundled selector or default changed in this slice.

The focused retained-evidence test and compiled-CLI offline capability discovery
passed under the bounded runner. Discovery reports all three tag capabilities
as unavailable and preserves the manifest's explicit unavailable scope. All 11
retained files match the original run byte for byte. Command-doc consistency and
docs lint passed. Logs are in `bin/minimum-policy2-retained.log`,
`bin/minimum-policy2-discovery.json`, and `bin/minimum-policy2-docs.log`.

Next matrix work: qualify 8.3.9 under the current policy and propagate a named
minimal OPC UA profile consistently through capture and every workflow suite.
IA's current documentation recommends the full `com.inductiveautomation.opcua`
identifier for the whitelist. Actual module inventory, including disabled or
quarantined states, must determine whether a requested profile was realized;
the requested setting alone is insufficient evidence. The complete v1 goal,
remote scheduler activation, and remaining implementation/release gates stay
active.

The fresh 8.3.9 policy-2 pipeline from clean `f5993f9` passed all 18 stages from
18:55:58 to 19:04:46 UTC on 2026-09-05 in 528.63 seconds. Its
10/27/39/23 lifecycle/resource/project-tag/operational checks passed, retaining
every historical tag check and adding capability discovery. The test executable
hash is identical to the minimum run: only separate evidence tests and docs
changed between the two source commits. Every container was removed; an
independent query found none remaining. The clean source worktree was removed
after verification, with no host recovery or changes to limits.

The original run and complete candidate are retained in
`internal/referencebuild/testdata/ignition-8.3.9-policy2/`. All six scopes are
qualified. The 687-operation catalog has the same current contract identity
and 32-module inventory as earlier captures, while preserving new raw hash
`8b53ecad684cd32fd387142af58f3512d3c9b39b1c52ac90e37912c0020b87ce`
and document hash
`106f238119d2f0cf6dfa785454e21203a95724289b761e6b7c4766bce917870c`.
The reference comparison reports `unchanged_under_policy`; no operations were
added or removed. Both default-module cells now have current-policy acceptance.
The shared retained-evidence regression covers both capability shapes; the
new `docs/compatibility-matrix.md` makes qualified coverage and remaining module
cells explicit. The runtime embedded reference remains unchanged.

Both retained-evidence cases passed under the bounded runner, as did compiled
offline 8.3.9 capability discovery and command/docs checks. Discovery reports
all tag prerequisites advertised, six qualified scopes, and no unavailable
scope. All 11 new retained files match the original run byte for byte. Logs are
in `bin/defaults-policy2-retained.log`, `bin/full-tags-policy2-discovery.json`,
and `bin/defaults-policy2-docs.log`. No full v1 completion or remote schedule
activation is claimed; module-profile propagation and both smaller-profile
cells are the next qualification slice.

Fresh OPC UA captures on both pinned images passed serialized containment,
parser validation, and cleanup. The 8.3.9 capture at 19:07:54 UTC contains 454
operations; 8.3.0 at 19:09:05 contains 446. Each retains all 32 installed modules,
with OPC UA active/enabled and 31 excluded modules inactive/disabled in the
healthy collection. Original receipts and losslessly compressed documents are
retained in `internal/moduleprofile/testdata/`. These captures establish the
observed profile shape, not live workflow qualification for it.

The shared `internal/moduleprofile` contract now owns selection, complete module
inventory types, and versioned validation. `image-defaults` preserves all-active
requirements; `core-opcua` requires the selected module active and every excluded
module explicitly inactive/disabled. Faults, quarantine, pending upgrades,
missing selected modules, and inconsistent inventory checksums are rejected.
The same selection flows through the coordinator, contributor capture, lifecycle
probe, and all acceptance suites. Suites check the observed profile before
provisioning API credentials or sending test mutations. Every receipt records
its whitelist, which must match the capture during assembly.

New manifests include module policy `igw-module-profile/1` separately from the
unchanged workflow qualification policy. Reference reading binds manifest module
metadata to the complete original captured inventory. Historical references keep
their all-active default interpretation and original bytes; removing the new
field or filtering inactive metadata cannot relabel a core capture as defaults.
No new core workflow qualification or scheduled matrix execution is claimed
until clean complete runs establish those separate results.

The full unit suite and focused module-profile/reference/qualifier/Gateway-test/
CLI race checks passed under serialized bounded validation. Regressions verify
both real inactive-module captures, missing or unexpected active modules,
fault/quarantine/upgrade refusal, whitelist consistency across every receipt,
manifest relabeling and filtering, historical bundle readability, and coordinator
selection propagation with ambient-value isolation. Python coordinator tests
passed, and both retained capture receipts/documents match the original bytes.
Logs are in `bin/module-profile-{focused,regression,unit,race}.log`.

Development/contributor builds and command/docs checks passed. Compiled
contributor checks rejected unknown and conflicting profile flags with exit 2
before creating output or invoking Docker. Legacy smoke built successfully and
stopped at `doctor` with exit 2 because the default Gateway remains unconfigured.
Logs are in `bin/module-profile-{build-docs,cli-flags,smoke}.log`. No host settings
or validation limits changed. A clean complete core-profile run is next.

The initial four-cell qualification matrix is complete. Clean `28d7334` core
pipelines passed all 18 stages on both images: 8.3.9 from 19:27:41 to 19:37:18
UTC (576.77 seconds), and 8.3.0 from 19:38:51 to 19:45:51 UTC (420.24 seconds).
The same test executable produced all four receipts in each run. Checks total
99 and 87 respectively; absent 8.3.0 tag APIs yield three verified pre-dispatch
refusals. Each inventory retains one active OPC UA module and 31 inactive
modules. All containers were removed, independent queries were empty, and
clean source worktrees were removed. No host recovery or limit changes ran.

Exact run receipts and complete references are retained under
`internal/referencebuild/testdata/ignition-8.3.{0,9}-core/`. Their current contract
hashes match the preceding captures, while new raw/document hashes preserve the
fresh observations. Offline regression qualification checks all four cells,
including each inventory, image identity, catalog, lifecycle, workflow receipts,
and capability scopes. The historical whitelist correction preserves old
all-active inventories without restricting previously allowed whitelist values;
new named profiles still require the exact selection and complete inventory.

The scheduled workflow now serializes both profiles per tag with separate
artifact names and failure cancellation of queued work. Workflow structure and
source/permission isolation were checked locally; remote activation/execution
remains unverified. Request encoding, batch outcomes, additional workflow
capabilities, restart verification, performance, migration/cutover, and release
qualification remain part of the active v1 goal.

The development CLI now embeds the three missing reviewed cells alongside the
unchanged original 8.3.9 default reference. All copied payloads and manifests
match their qualified candidates byte for byte. JSON summaries add
`activeModuleCount`, derived from the verified inventory; human output includes
the recorded profile and active/installed counts. Core references preserve
31 inactive observations. Historical manifests keep their original identities
and all-active interpretation without a synthesized profile field.

The full unit suite, focused reference/qualification/CLI race checks, both CLI
builds, command/docs checks, and all eight coordinator tests passed using
serialized bounded validation. Compiled offline listing reports all four
selectors, and minimum core capability discovery reports the expected three
unavailable tag workflows with no target metadata. Logs are in
`bin/reference-matrix-{focused,unit,race,build-docs}.log`. The legacy smoke build
succeeded, then `doctor` exited 2 because the default Gateway URL and token
remain unconfigured (`bin/reference-matrix-smoke.log`). Separate disposable
Gateway qualification above provides the live evidence. No host settings or
validation limits changed.

The next request-contract slice adds repeatable `--filter field[operator]=value`
to resource, project, and log lists. Generic API queries share the same typed
executor and catalog validation. The parser binds exploded filter properties
separately from named scalar parameters, validates the complete object schema,
and preserves exact text on the wire. Required/inherited parameters, overrides,
headers, and request bodies remain validated. Unknown operators, duplicate
properties, malformed query encoding, and ambiguous ownership refuse execution.
Other parameter serialization and multipart inputs remain unfinished.

Parser identity advances to 13 without changing compatibility/contract policies
or hashes. Current-parser fixture expectations advance separately from original
capture and workflow evidence. Historical cross-file checks retain each
capture's recorded parser; new reference assembly rejects historical parser
evidence even if internally consistent. No reference payload or original live
receipt was rewritten.

Focused regressions reproduced the prior filter failures before the fix. The
initial race run exposed shared schema-renderer writes under concurrent
validation; each catalog now serializes its validation calls. A separate live
filter suite was added to verify actual resource/project/log selection and
pre-dispatch refusals on fresh disposable Gateways, retaining exact OpenAPI
bytes, executable provenance, request counts, and cleanup evidence. Compilation
or skipped opt-in tests do not establish live qualification.

The final full unit suite, both CLI builds, command/docs checks, and focused
catalog/CLI filter race regressions passed under serialized bounded validation.
The earlier broad race run also passed the reference and retained-qualification
packages; its catalog failure was the reproduced renderer race repaired above.
Logs are in `bin/query-filter-{race,race-fixed,final-checks}.log`. Legacy smoke
built successfully and stopped at `doctor` with exit 2 because the default
Gateway remains unconfigured (`bin/query-filter-smoke.log`). The new live
harness compiled and its non-live helper tests passed; real filter acceptance
on both pinned images is the next gate. No host settings or limits changed.

The first clean filter run from `ffd9ece` passed the 8.3.9 lifecycle probe but
failed in the test harness before catalog retrieval. Its request counter assumed
a non-nil HTTP transport; the real session uses Go's default nil-transport
convention. Cleanup succeeded and an independent query was empty. The original
failed log/receipt and explicit unqualified assessment are preserved in
`bin/query-filter-qualification-20260905/`. The original receipt incorrectly says
`passed: true`: deferred writing ran before the test runner marked the panic
failed. That flag is not acceptance evidence; the process exited 2 and no checks
completed. The harness now supports default/custom transports and requires
reaching the final assertion before recording success. A live run from the
corrected clean executable remains required. No host failure or recovery ran.
Both transport modes passed the actual-CLI request-observation regression under
the race detector (`bin/query-filter-harness-fix.log`).

Both corrected clean runs passed. From `c8974b7`, 8.3.9 completed its 24 query
checks at 20:55:22 UTC in 79.10 seconds; 8.3.0 completed the same checks at
20:57:28 UTC in 74.13 seconds. The shared executable SHA-256 is
`30d85edb188e8e228afe2d98c562daca3103954e2cbd82952b77cfa7019ac7de`.
Each image passed its lifecycle probe first. Actual resource/project/log
selection, combined filters, exact special characters, pagination, nonmatches,
generic request parity, preview without dispatch, and all six invalid/duplicate
refusals passed. Receipt and process outcomes agree, cleanup succeeded, and
independent container queries were empty. The source remained clean throughout
and its owned worktree was removed without force.

All original files from `bin/query-filter-qualification-20260905-fixed/` are
retained byte for byte in `internal/testgateway/testdata/query-filters/`.
The 8.3.9 raw/document hashes are
`92ad365eed054e63f47e51ecb9d1d8bf89b608894b19a4210f8b98c03a5b720f` and
`a517431f57008bae2db5352c0dcc30d9aa35ff00d977ffd72fb061cdf795545a`;
8.3.0 records
`9de9b6e43a8852d2f3ffd3db0f2e051aa5b6387c3ef40c89f66c42179674dcab` and
`f88441d6d0840f28221001eb7141dfed147ea265a31e89cf1da98b124aca8d52`.
Both contract identities match their previous core captures. The offline
regression separates historical receipt integrity from current parser checks,
covering executable/image/source identity, complete check outcomes, exact
compressed/raw/document/contract hashes, complete module inventories, and
observation/cleanup ordering. This completes the list-filter slice; remaining
parameter serialization, multipart input, batch outcomes, restart verification,
performance, migration/cutover, and release gates keep the v1 goal active.

The retained-evidence/current-parser regression and command/docs checks passed
under the bounded runner (`bin/query-filter-retained-final.log` and
`bin/query-filter-evidence-docs.log`). The common source/build observations and
both image directories match their original successful runs byte for byte.
The only unrelated working-tree changes remain the user's 17 script modes.

Named-query validation is the next input-contract slice. Inspection of the
pinned validator found fixed-size numeric conversion, omitted boolean
constraints, and array validation applied independently to repeated values.
The initial synthetic regression reproduced 19 failures, including correctly
encoded reserved text being rejected, repeated primitive values being accepted,
large/precise numbers being misinterpreted, and missed array constraints.
The failing result is preserved in `bin/parameter-values-before.log`.

Parser 14 now binds named primitives and whole exploded form arrays, validates
their complete schemas with exact numeric values, and uses the same private
validation views as filters. It preserves strings and wire encoding, rejects
ambiguous repeated primitives, and bounds numeric validation work. Explicit
JSON boolean/numeric spelling replaces permissive aliases. Remaining path,
header, other query encodings, structured inputs, and multipart support are
still required; this slice does not redefine complete input support.

Focused tests passed for OpenAPI 3.0/3.1, exact schema bounds and numeric values,
whole-array constraints, inherited/overridden declarations, filter/array
ownership, numeric work limits, and the captured Perspective session-array
contract on all four default captures. Logs are in
`bin/parameter-values-{focused,boundaries,captured}.log`. Current-parser fixture
expectations advance separately; original OpenAPI bytes and live receipts
remain unchanged. The catalog regressions and actual CLI wire tests passed
under the race detector. The full unit suite, both CLI builds, command-doc
consistency, and docs lint also passed under the bounded runner. Logs are in
`bin/parameter-values-{race,cli-race,unit,build-docs}.log`; the first CLI race
attempt exposed a test-handler proxy-path mistake, corrected before its passing
rerun. Legacy smoke built successfully and stopped at `doctor` with exit 2
because no default Gateway is configured (`bin/parameter-values-smoke.log`).
Live qualification recorded under parser 13 remains historical evidence; this
slice has no new live acceptance claim. No host settings or limits changed.

Source inspection also found that the upstream JSON body decoder and subsequent
canonicalization both convert numbers through floating-point values. The next
input slice must preserve exact body values during request-specific schema
validation, including explicit null and directionality constraints, before
adding multipart construction and the remaining structured input encodings.

The exact JSON body regression reproduced 13 failures before implementation:
rounded constants/fractions, collapsed distinct numeric array elements,
rejected finite large numbers, accepted duplicate keys, accepted whitespace as
null, invalid UTF-8 replacement, and unbounded nesting. The failing result is
preserved in `bin/body-values-before.log`. The adapter now bypasses both lossy
upstream conversions, invokes request-specific schema compilation, and bounds
input work while keeping outgoing bytes unchanged. It also selects the most
specific JSON media range and refuses ambiguous declarations and malformed
Unicode. Query/body adapters share the selected-operation view helper.

Focused regressions and all four captured default Gateway contracts passed in
`bin/body-values-{focused,captured}.log`. Coverage includes OpenAPI 3.0/3.1 null
and required-field behavior, exact numeric constraints, duplicate-key and
Unicode refusals, read errors, schema compilation errors, input limits, JSON
suffixes, and media specificity. CLI wire/race and broad results are below.
No new live qualification is claimed. The full input, workflow, performance,
cutover, release-artifact, and current-source acceptance gates remain active.

The initial catalog/CLI race checks passed (`bin/body-values-race.log`),
including exact preview digests, unchanged transmitted bodies, twelve invalid
preview/write refusals, and preservation of required header checks. Additional
Boolean Schema boundaries then reproduced two acceptance errors and a
conditional-schema compilation error. A private parser representation adapter
now preserves Boolean Schema semantics using equivalent object forms after
document validation. It leaves Boolean data/annotations and original document
identities untouched. The final focused body suite passed in
`bin/body-values-boolean-final.log`; earlier failing boundary logs are retained.
The full unit suite passed with parser 15 and its updated capture expectations
(`bin/body-values-unit.log`). Final catalog/CLI race checks also passed after
the Boolean Schema fix (`bin/body-values-race-final.log`), including an added
reference/inspection regression and refusal of Boolean Schemas in OpenAPI 3.0.
Both CLI builds, command-doc consistency, and docs lint passed in
`bin/body-values-build-docs.log`. Legacy smoke built successfully and stopped
at the unconfigured default Gateway's `doctor` with exit 2
(`bin/body-values-smoke.log`). Original live receipts remain unchanged; no new
live acceptance is claimed. No containers, host control commands, host settings,
or limit increases were needed for this slice. The development guide now
reflects all four embedded references and their historical qualification.

The next input slice must also address validation coverage: the upstream
non-JSON body path can return success when no decoder is registered. A
schema-bearing body needs a qualified decoder or an explicit refusal; missing
schemas and opaque transport must not be labeled as complete value validation.
Multipart/form construction and the remaining path/header/structured parameter
encodings should build on that explicit coverage boundary.

The body-media inventory was regenerated from both exact default captures under
the bounded runner (`bin/body-media-inventory-parser15.json`). All seven 8.3.9
multipart media declarations omit schemas. Both versions also advertise plain
text, unconstrained binary forms, wildcard datafiles, and invalid `binary
stream` media names. The baseline regression reproduced 13 false-success cases
(`bin/body-media-before.log`), covering omitted text assertions, unsupported
decoders, undeclared bodies, missing required opaque input, and missing media
types.

Parser 16 now owns body presence/media selection and actual coverage reporting.
It validates plain UTF-8 text with the complete selected schema and refuses
unsupported decoders as `unsupported_input` before dispatch. Opaque and
recognized unconstrained binary inputs report transport-only coverage;
additional binary assertions cannot disappear. The same media selector serves
streaming, including wildcard datafile routes. `api describe.bodyInputs`
separates the vendor's schema declaration from available decoding/streaming,
and generic results expose actual validation in metadata. Existing JSON/body
checks continue through the shared core; an older filter test now supplies
bodies only where its fixture declares them. Initial focused and CLI checks
passed in `bin/body-media-{focused-fixed,integration,binary}.log`. Final captured
contracts and 40 MiB binary streams passed in
`bin/body-media-final-focused.log`. Schema-bearing media ranges now report
`selected_media` before promising a decoder or streaming eligibility; exact
JSON and raw binary selections are covered by the range regression. The final
catalog/CLI race checks and full unit suite passed in
`bin/body-media-{race-final,unit}.log`.
No live acceptance is claimed for this parser. Multipart construction, binary
constraints, explicit empty text, structured inputs, and the remaining full v1
gates retain their original scope.

Both CLI builds, command-doc consistency, and docs lint passed in
`bin/body-media-build-docs.log`. Legacy smoke built successfully, then stopped
at `doctor` with exit 2 because the default Gateway remains unconfigured
(`bin/body-media-smoke.log`). Historical Gateway evidence and raw vendor
documents remain unchanged. No containers, host control commands, host settings,
or validation-limit changes were needed for this slice.

Multipart construction now uses the existing typed upload/execution path.
Shorthand text/file flags and an ordered JSON manifest feed one bounded private
MIME snapshot. Per-part preview identities, concrete media types, source-file
immutability, framing limits, cancellation, and cleanup are explicit. Manifest
parsing rejects duplicate or unknown selectors, null/non-string values, and
lossy Unicode. JSON bodies and manifests share the existing Unicode guard
without changing parser semantics or source identities. Schema-bearing multipart
decoding, URL-encoded forms, and real-Gateway multipart acceptance remain open.
The focused artifact, CLI, and JSON-body regression checks passed in
`bin/multipart-focused.log`, including a 40 MiB file, repeated and empty text,
per-part media/filename overrides, raw requests, pre-dispatch refusals, and
snapshot cleanup. The offline command schema now includes the manifest's JSON
input shape, and the execution regression binds the entire encoded-body hash
and boundary to the prepared upload. Focused artifact/execution/CLI/catalog race
checks and the full unit suite passed in `bin/multipart-{race,unit}.log`.
No live acceptance is claimed for this construction slice.

Both CLI builds, command-doc consistency, and docs lint passed in
`bin/multipart-build-docs.log`. Legacy smoke built successfully, then stopped
at the unconfigured default Gateway's `doctor` with exit 2
(`bin/multipart-smoke.log`). No container or host-service operations ran, and
the resource guard and original OpenAPI/reference evidence remain unchanged.
Explicit empty-body inputs, binary assertions, structured parameter encodings,
multipart schema decoding, and the full workflow/cutover/release gates remain
within the active goal.

Explicit empty-body handling is the next input-contract slice. The baseline
in `bin/body-presence-before.log` reproduced rejected valid empty strings/files,
skipped optional-body assertions, accepted empty JSON/unsupported encodings,
and dropped HTTP media types. Parser 17 now separates omitted inputs from
explicit zero-byte representations, validates empty strings through the same
request-specific compiler, and preserves known zero-length file framing.
Previews report `bodyPresent` and the zero-byte digest. The content-type option
alone does not supply a body, and explicit media metadata reaches the transport.

The initial focused run passed the new regressions and exposed older fixtures
using an empty reader to mean omission (`bin/body-presence-focused.log`). Those
fixtures now construct omitted inputs explicitly; zero-byte opaque input moves
to the accepted transport-only case. Historical vendor documents and live
receipts remain unchanged while current parser expectations advance to 17.
Final catalog/transport/CLI/execution race checks passed in
`bin/body-presence-race.log`, including empty raw requests and preserved default
media types. The full unit suite and both CLI builds passed, followed by command
documentation consistency and docs lint
(`bin/body-presence-{unit,build-docs}.log`). Legacy smoke built successfully,
then stopped at the unconfigured default Gateway's `doctor` with exit 2
(`bin/body-presence-smoke.log`). No live acceptance is claimed for the changed
input boundary. No containers, host-service operations, host settings, or
validation-limit changes were needed for this slice.

## References

- [IA Gateway API documentation](https://www.docs.inductiveautomation.com/docs/8.3/platform/gateway/openapi)
- [IA resource metadata](https://www.sdk-docs.inductiveautomation.com/docs/8.3/programming-for-the-gateway/storing-data-using-resource-collections/resourcetypemeta/)
- [IA endpoint considerations](https://docs.inductiveautomation.com/docs/8.3/appendix/scripting-functions/system-config)
- [Official Ignition container](https://docs.inductiveautomation.com/docs/8.3/platform/docker-image)
- [Durable plans and acceptance criteria](https://developers.openai.com/cookbook/examples/codex/iterating-development-workflows-with-codex#agents-and-plans)
