# Gateway catalog architecture

The v1 catalog implementation lives in `internal/catalog` and powers the
development CLI in `cmd/igw-next`. The existing released `igw api sync` command
still uses the legacy loader until command cutover.

The target Gateway's `/openapi.json` describes the documented routes provided
by its installed version and modules. IA's `/openapi` endpoint is a UI, so it
is not the first JSON discovery candidate. Runtime state, permissions, custom
validation, and operation effects require separate Gateway evidence. See the
[IA API documentation](https://www.docs.inductiveautomation.com/docs/8.3/platform/gateway/openapi).

The implementation retains the vendor's exact JSON bytes. It builds a complete
OpenAPI 3.0/3.1 model using libopenapi, checks document structure, and provides
request validation using libopenapi-validator. Descriptions retain operation
definitions, inherited path parameters, shared components, and security
requirements. External references are rejected without file or network reads;
import a self-contained document instead. Custom JSON Schema dialects are also
rejected because the validator otherwise enables external schema loading.
Operation identity is `METHOD /path`;
an operationId can be used only when it resolves to exactly one operation.

Snapshots are partitioned by profile and normalized effective Gateway URL,
including a reverse-proxy base path. Credentials are never stored in a snapshot.
Raw documents are immutable SHA-256-addressed blobs; immutable validation
receipts record the target, source, fetch and verification times, HTTP validators,
parser version, and three hashes. Atomic file publication coordinates concurrent
writers. The newest complete valid receipt wins, so a slower older refresh
cannot replace a newer one through a shared latest pointer. A corrupt newer
receipt produces a warning and falls back to a valid older snapshot.

The identities have distinct purposes:

- `rawSha256` checks the exact vendor bytes, including formatting.
- `documentSha256` normalizes object key order and JSON whitespace while
  retaining all fields, array order, and exact JSON number spellings.
- `contractSha256` applies versioned policy `igw-contract/2` before hashing.
  It omits recognized documentation annotations in their OpenAPI/Schema
  contexts and sorts schema `allOf`, `anyOf`, `oneOf`, `enum`, `required`, and
  union `type` arrays. Defaults, constraints, duplicates, unknown keywords,
  extensions, ordered tuples, and arrays inside instance data remain intact.

Pins use the contract hash. The policy name is part of the hash input. Reference
targets and array/annotation ancestors are preserved; unresolved, anchor, and
dynamic references conservatively preserve their containing resource. Local
schema resources respect `$id` boundaries. Policy 2 additionally recognizes the
exact paired keyboard embedding reviewed by the parser adapter below. Its local
references preserve their actual definition targets instead of freezing the
entire document. Generic nested `$defs` and unreviewed references retain the
conservative rules; no parameter placeholder is used for hashing. Original raw
and document identities remain unchanged. This is a reviewed projection, not a
general proof of JSON Schema equivalence or runtime behavior. A changed hash
requires review; it is not automatically a breaking change. The rules follow
the [JSON Schema validation vocabulary](https://json-schema.org/draft/2020-12/json-schema-validation)
and [reference semantics](https://json-schema.org/draft/2020-12/json-schema-core).

New snapshot receipts use version 2. Version 1 receipts remain readable: the
loader verifies their original canonical checksum, derives current identities,
retains `legacyIdentity`, and emits a migration warning. It does not rewrite
history or advance Gateway verification time. Old pins are not accepted as new
contract identities; inspect the document and explicitly replace those pins.
Version 2 receipts with policy 1 also remain readable: the loader verifies the
original policy-1 hash before deriving policy 2. `legacyIdentity` records the
prior parser, hash, and policy, preserving an earlier migration in `previous`
when present. Neither load path rewrites a receipt or advances its fetch or
verification times. All current hashes change because the policy name itself
is hashed, even when the projected contract is otherwise unchanged.

Discovery and reads refresh after 24 hours. A write revalidates during each
invocation, using ETag or Last-Modified when supplied. A failed refresh retains
the last valid snapshot. Reads can report a stale fallback; writes require an
explicit `--allow-stale-spec` and a previously fetched snapshot for the same
target. Explicit refresh requests fail if refresh fails. Offline imports and
bundled references cannot silently authorize writes. Gateway version and module
metadata remain absent until verified; the API document's info.version is not
assumed to be the Gateway version.

Library dependencies are pinned in go.mod/go.sum. The selected parser and
validator require Go 1.25.7. The package boundary keeps vendor models out of the
CLI and workflow contracts.

## Real Gateway qualification

The test suite includes exact compressed documents from two fresh official
Ignition 8.3.9 containers with their default modules. Each contains 687 operations
across 587 paths and is about 12.7 MB uncompressed. Its receipt records the
image digest, observed `ignitionVersion`, capture time, raw/contract hashes,
parser version, and compatibility policy. `info.version` is `1.0.0` in that
document; it is not the Gateway release version.

The vendor document needs a narrowly scoped adapter before OAS validation:

- 343 path parameters contain `allowReserved: false`. That field applies only
  to query parameters. The parser's private model omits this false annotation.
- Seven operations declare an empty `responses` object. The private model uses
  an explicitly undocumented default response without inventing a status code
  or response schema. Operation descriptions expose the missing contract.
- 288 unused `$id` occurrences are repeated across identical primary/backup
  settings variants in resource POST/PUT request bodies. The private model
  omits these identifiers only when every pair matches the observed generator
  shape and every variant is free of references, anchors, nested identifiers,
  and dialect changes. All pairs must qualify before an operation is adapted.

Policy `ignition-openapi/5` matches the observed generator identity and those
exact structural shapes, recognizing both the current IA license URL and the
8.3.0 Gateway-relative `/res/sys/license.html` EULA. Each snapshot binds the
policy to its original raw SHA-256. It does not grant trust based on the
document's title or license URL;
the resulting model must still pass OAS validation and reference checks.
Descriptions and exports retain the vendor definitions. `spec inspect` exposes
every adjustment with its operation and JSON pointer; snapshot metadata reports
counts by rule. Any additional defect remains an error. This adapter implements
the [OAS parameter and response rules](https://spec.openapis.org/oas/v3.1.0.html)
and the [JSON Schema rules for resource identifiers](https://json-schema.org/draft/2020-12/json-schema-core#section-8.2.1).

The retained 8.3.0 document has two additional reviewed parameter defects. For
the exact entity-section and SCIM routes, 16 path parameters are marked
optional. The private model requires values for the explicitly selected path
template and retains all supplied value constraints. It does not infer a
different path with the SCIM version omitted; `api describe` exposes the
original optional annotation and the adjustment. Script cancellation omits
the ID parameter's schema entirely. Discovery retains this gap, while both
`api request` and its preview fail with exit 2 and `catalog_schema` before
sending the operation. `api raw` remains available with explicit `--yes` for
the DELETE request. These adjustments match only the reviewed route, method,
and parameter shapes; unknown defects remain errors. The corresponding fields
are already corrected in the retained 8.3.9 document.

The default-module 8.3.0 capture adds an EAM `running` path parameter marked
optional despite its selected template. Its supplied boolean schema remains
unchanged while the selected placeholder becomes required. The SFC chart GET
omits both `projectName` and `chartPath` schemas. Discovery reports that gap;
schema-assisted reads and previews return `catalog_schema`/2 before sending
the operation. Explicit raw reads remain available. These module-specific
corrections also match only the observed method, route, and parameter shapes;
the corresponding 8.3.9 parameters have complete schemas.

8.3.0 also embeds keyboard-layout `$defs` without rebasing 24 local references.
The private model expands the supplied definitions at eight config/backupConfig
positions in four exact resource operations. Expansion produces the same
inline schema emitted by 8.3.9; it preserves every value constraint. Both
variants must match, stay within 64 KiB each, and contain only the reviewed
acyclic reference graph. Unknown references, siblings on `$ref`, identity or
dialect changes, or extra definition scopes prevent adaptation. Descriptions
retain the original references and explain the correction. This follows
[JSON Schema reference and definition semantics](https://json-schema.org/understanding-json-schema/structuring).

Recursive arrays, such as required security-level children, accept finite trees
and are checked against actual input values. The parser receives a formatted
private copy: its node index otherwise performs quadratic work on large compact
JSON. Eager compilation of unrelated request/response schemas is disabled.
Local discovery of the captured expanded document took about two seconds with
roughly 600 MiB peak RSS on the development machine; memory needs optimization
before release qualification.

Document/model validation is not proof that every request schema is usable.
Captured 8.3.9 basic-schedule create/update/delete schemas now support request
validation. Tests reject invalid primary/backup values and enforce required
signatures. Tag-provider variants
contain references and remain outside the identifier adapter. Uncompilable
schemas produce `catalog_schema`, preserving the distinction from invalid
user input. Explicit raw requests remain available. The adapter keeps all
assertions, including potentially overlapping vendor `oneOf` branches; it
does not infer a profile discriminator or rewrite polymorphic constraints.
Real mutation and outcome checks are still required for workflow qualification.

The two captures differ at 56 reordered `oneOf` arrays, four reordered `enum`
arrays, and two scan-lock example timestamps. Their raw and document identities
differ, but both produce contract hash
`fce0593c41f1d0ae31c0647c34bb10ccebf6f44958c3c55913c887654ff9ccbc`.
Tests retain both captures and require that stability. Separate
`qualification.json` files bind the current parser and policies to each raw
document while preserving the original `capture.json` history.

Use `spec inspect FILE --summary --json` for identities, operation count, and
compatibility totals without thousands of operation definitions. `spec diff`
reports `contractEqual` separately from `documentEqual`; its operation change
list describes document differences. Equal contracts report
`unchanged_under_policy`; different contracts require review. Neither result
certifies Gateway runtime behavior or backward compatibility.

Comparison is owned by `internal/catalog.Compare`; the CLI renders its typed
result. Reference-update tooling can use the same rules without invoking a
CLI subprocess. It compares immutable indexes directly, preserving exact
numbers and avoiding redundant full-document copies.

## Contributor capture

### Qualified reference assembly

`igw-capture qualify` assembles a local `igw/reference/v1` directory from a
capture, saved registry resolution, lifecycle receipt, and the three dedicated
workflow receipts. It hashes the supplied test executable and requires every
receipt to identify that exact binary and image configuration. Workflow
receipts must use the current parser, match the capture's contract and observed
module inventory, come from a disposable loopback Gateway, and retain all
required positive and negative checks. Failed/partial outcomes are necessary
for tests such as stale-signature refusal and partial tag imports.

```sh
bash scripts/bounded-run.sh -- bin/igw-capture qualify \
  --resolution bin/new-release-resolution \
  --capture bin/new-8.3.9-capture \
  --lifecycle bin/new-lifecycle-receipt.json \
  --resources bin/api-acceptance.json \
  --transfers bin/project-tag-workflows.json \
  --operations bin/operational-workflows.json \
  --test-binary bin/testgateway.test \
  --baseline internal/catalog/testdata/ignition-8.3.9-defaults/openapi.json.gz \
  --out bin/new-qualified-reference
```

Assembly performs no network calls or container operations. It reparses the
exact captured and baseline documents, verifies capture identities, and uses
the shared catalog comparison. Its versioned policy currently qualifies ACTIVE
first-party modules that are enabled on startup without pending upgrades.
Other deployment states require separate evidence and a reviewed policy change.

The output preserves the exact vendor document in `openapi.json.gz`, original
capture/workflow/lifecycle receipts, and exact registry manifests. The decoded
resolution receipt is canonically re-encoded from the same validated input.
`reference.json` records identities, image/module metadata, parser version,
comparison, explicit qualification scopes, and each payload's size/SHA-256.
All inputs must qualify before the new output directory is created. Payloads
are published atomically and the manifest is last; incomplete directories have
no usable manifest. Existing bundles are never replaced. The completed bundle
is read back through its public loader before assembly reports success.

`internal/reference.Read` verifies the complete fixed payload list and checksums
offline. `OpenCatalog` also checks gzip integrity, the decompression bound, and
the exact vendor identities after parsing with the current parser. Checksums
establish integrity, not publisher authenticity; use a trusted repository or
release channel. A reference never establishes a live target's contract or
authorizes writes. Parser work retains the bounded-runner requirement.

The first retained bundle is
`internal/reference/bundles/ignition-8.3.9-defaults`. It contains 32 observed
modules, the 687-operation contract, and 98 recorded checks from one binary:
10 lifecycle, 27 resource, 38 project/tag, and 23 operational checks. It is about
792 KiB. Qualification covers the recorded basic-schedule, disabled-project,
memory-JSON-tag, and backup/log/diagnostics workflows; it does not qualify every
request schema or prove general backward compatibility.

The development CLI embeds this complete bundle. `spec references list` exposes
the available selectors; `inspect REFERENCE` checks every payload and reports
the full manifest; `export REFERENCE --out NEW_DIRECTORY` preserves all ten
original files. A selector can be a bundled name or an explicit local directory.
`api list`, `api describe`, and `api capabilities --reference REFERENCE` use the
same current parser
and operation model as Gateway discovery, after checksum and identity checks.
All reference paths work without Gateway configuration, credentials, cache, or
network. They return `meta.reference` with explicit source kind, origin, image,
module inventory hash, qualification scope, and contract identities. Assembly
time is `createdAt`; it is not a current Gateway verification timestamp.
API discovery additionally reports `inspectionParserVersion` and
`inspectionCatalog`, distinguishing current parsing and identity policy from
the recorded qualification. Historical bundles verify against their recorded
supported policy. Their manifest, receipts, and `catalog` identity remain
unchanged; current inspection is not renewed live workflow qualification.
Human output identifies
the reference before listing operations. The `--spec-pin` check applies to
inspection, export, and API discovery using the manifest's recorded contract
hash and policy. A reference pin never becomes a live-target pin implicitly.
Export verifies the reviewed identity
again before publishing and refuses existing directories.

`api capabilities` currently derives tag workflow prerequisites from exact
operation keys in the selected document. It reports `advertised` or `unavailable`
with required and missing operations, without inferring support from version
labels. Typed tag workflows enforce these same definitions against their shared
invocation snapshot before executing. A missing export route prevents a verified
JSON import before any write. This is a catalog prerequisite check, not renewed
workflow qualification or a claim about Gateway permissions and instance data.

New references use qualification policy `igw-reference-workflows/2`. It keeps
all tag checks when both transfer routes are advertised, or requires tested
refusal with zero observed operation requests when both are absent. In the
latter case `qualification.unavailableScopes` records `tags/memory-json` and
that scope is excluded from successful workflow coverage. The manifest includes
the catalog capability evidence; opening the reference checks it against the
original document. Partial tag API surfaces require additional coverage before
qualification. Historical policy-1 bundles keep their original evidence and
remain readable. See `docs/reference-updates.md` for the pipeline contract.

References never populate a target cache and the flag is unavailable on request
commands, including previews. Invalid or unavailable references fail without
Gateway fallback. The binary therefore retains a usable offline source even
when the upstream registry or a Gateway is unavailable. Independently exported
directories retain all provenance; distribution still requires a trusted
repository or release channel. Canonical examples are in `docs/commands.md`.
The update coordinator and scheduled workflow are described in
`docs/reference-updates.md`. Remote schedule activation and additional
version/module profiles require separate verification.

### Resolve and capture an image

Resolve the official `8.3` channel or an explicit `8.3.<patch>` release before
pulling an update candidate:

```sh
bash scripts/bounded-run.sh -- go build -o bin/igw-capture ./cmd/igw-capture
bash scripts/bounded-run.sh -- bin/igw-capture resolve \
  --tag 8.3 --out bin/new-release-resolution
```

This read-only operation contacts Docker Hub's fixed HTTPS authentication and
registry endpoints. It does not invoke Docker, pull layers, start containers,
or require Gateway credentials. Resolution requests an anonymous pull-scoped
token, rejects redirects and duplicate JSON keys, limits each manifest to
4 MiB, and enforces a deadline (30 seconds by default, at most one minute).
Registry errors omit response bodies and credentials. Rate limiting or failed
resolution returns an error without replacing any existing candidate.

The new private output directory contains `resolution.json`, `index.json`, and
`manifest.json`. The two manifest files preserve exact registry bytes, checked
against `Docker-Content-Digest`; the selected manifest must also match its
index descriptor's digest, media type, and size. The receipt records the tag,
resolution time, immutable index image reference, selected platform manifest
digest, and `linux/amd64` platform. Exactly one matching platform descriptor is
required. Direct single-platform manifests, ambiguous platform entries, other
release families, and nightly tags require explicit implementation/qualification
before they can enter this update path. See the registry's
[manifest API](https://distribution.github.io/distribution/spec/api/) and
[token authentication](https://distribution.github.io/distribution/spec/auth/token/).

Resolution is candidate provenance, not OpenAPI or runtime qualification. It
does not prove that the image runs or that its advertised platform matches its
contents. The capture runner must verify the actual platform, pass the lifecycle
probe, and capture the observed Gateway version before accepting new evidence.
Use the returned immutable `image` throughout one qualification run; do not
resolve the moving tag separately for each test. Publish the resolution receipt
alongside capture and workflow receipts so an update can be audited later.

`internal/imageref.Load` revalidates saved resolution directories entirely
offline, including bounded regular files, receipt provenance, exact manifest
digests, and the index-to-platform relationship. `ConfigurationDigest` returns
the selected image config digest only after this validation. Saving a candidate
uses the same checks, so modified receipts cannot publish a different image,
repository, platform, or incomplete chain as a verified resolution.

Live resolution on 2026-09-05 found the `8.3` channel at the same index digest as
the qualified 8.3.9 fixture below; its `linux/amd64` manifest digest is
`sha256:1e6778e8b787baf0b46d9018ac1b77ba58b5c618ea0f3ae543a465fb657d4295`.
The minimum `8.3.0` tag also resolved successfully. These are dated observations;
fresh resolution is required to detect upstream changes. The update coordinator
uses one resolved digest throughout its serial qualification run; reviewed bundle
promotion remains a separate action.

`cmd/igw-capture` creates a fresh, uniquely labeled container from a pre-pulled
official image digest, publishes HTTP only on loopback, disables quickstart and
Gateway Network, and removes its own container and anonymous volumes before
parsing the document. A fixed container name is an exclusive engine-wide slot;
the unique label and exact ID prevent cleanup of another session's container.
It uses random temporary admin credentials, copied privately with the image's
numeric user ownership. Passwords never enter Docker arguments or environment
values. It neither connects to nor reconfigures an existing Gateway.

Reference qualification currently supports `linux/amd64`. Preflight inspects
the local image's OS, architecture, and SHA-256 configuration ID; creation
specifies that platform and verifies the container's image ID before startup.
New capture receipts use version 3 and record `platform`, `imageId`, and the
owned-container `cleanup` result. Resource and operational receipts use version 2;
transfer receipts use version 3 with capability and request-count evidence.
All retain the same image provenance. Historical receipts remain unchanged. Bundle
qualification must match the observed image ID to the selected registry
manifest's config digest; a resolved index alone does not establish the image
configuration that actually ran.

Captures now also observe both `/data/api/v1/modules/healthy` and
`/data/api/v1/modules/quarantined`, with bounded pagination, stable ordering,
explicit total/offset checks, and unique module IDs across both collections.
The `moduleInventory` records IDs, names, module versions, reported states,
startup actions, upgrade flags, and the source collection. It omits exception
details and other diagnostic content. A versioned SHA-256 covers the sorted
module records; the observation timestamp is separate. The requested whitelist
and observed inventory are distinct evidence. Module versions must not be
inferred from the Gateway version.

Three consecutive observations must agree on both module inventory and exact
OpenAPI bytes. This detects startup changes across the observation window; it
is not a server-atomic snapshot. The vendor's `healthy` collection can contain
unloaded or faulted modules, so its name alone does not establish readiness.
Missing, malformed, moving, or incomplete inventory fails capture. Workflow
receipts retain the inventory observed during their own Gateway startup.

After `StatusPing` reports RUNNING, the tool authenticates to the disposable
Gateway's built-in IdP and waits for three identical OpenAPI responses. This
private browser-login adapter is confined to test infrastructure; the released
CLI continues to use API tokens. The 8.3.9 run confirmed that `/openapi.json`
requires authentication and that `/data/api/v1/gateway-info` reports the actual
runtime version.

The 2026-09-05 inventory capture recorded 32 first-party modules, all reporting
`ACTIVE` and `onStartup: enabled`, with no quarantined modules or pending
upgrades. Their versions vary independently: Perspective reported 3.3.9 and
Vision 12.3.9 on Gateway 8.3.9. The complete receipt is
`internal/testgateway/testdata/ignition-8.3.9-module-inventory.json`. It retains
the same 687-operation contract; these module observations apply to that image
and capture, not every 8.3 installation.

Run the short lifecycle probe before qualifying a new image. It verifies the
engine's configured and kernel-applied limits, rejects a second capture, waits
for a shortened in-container lifetime to expire, and verifies removal. Compile
first so builds and Gateway startup do not overlap. Ordinary test runs skip
this live test unless the image environment variable is explicitly supplied.

```sh
docker pull --platform linux/amd64 inductiveautomation/ignition@sha256:28bd6b320157ec8dbbe465d0cd7c9f0ababfda4ff01c0bab982c0522a7b4eba2
mkdir -p bin
bash scripts/bounded-run.sh -- go test -c -o bin/testgateway.test ./internal/testgateway
bash scripts/bounded-run.sh -- env \
  IGW_CAPTURE_TEST_IMAGE=inductiveautomation/ignition@sha256:28bd6b320157ec8dbbe465d0cd7c9f0ababfda4ff01c0bab982c0522a7b4eba2 \
  IGW_LIFECYCLE_EVIDENCE=bin/new-lifecycle-receipt.json \
  bin/testgateway.test -test.run '^TestLiveCaptureLifetime$' -test.v -test.timeout=90s
bash scripts/bounded-run.sh -- go build -o bin/igw-capture ./cmd/igw-capture
bash scripts/bounded-run.sh -- bin/igw-capture \
  --image inductiveautomation/ignition@sha256:28bd6b320157ec8dbbe465d0cd7c9f0ababfda4ff01c0bab982c0522a7b4eba2 \
  --out bin/new-8.3.9-capture
```

The output directory must be new and its parent must exist. Use `--docker` to
select a different executable (`IGW_CAPTURE_TEST_DOCKER` for the probe), or
`--modules com.inductiveautomation.opcua` for
an explicit minimal module whitelist. An empty whitelist uses image defaults.
The tool requires a running Linux Docker engine with cgroup v2 limit support
and a bounded validation scope. Containers have a 2 GiB memory limit, no swap,
two CPUs, 256 tasks, and no restart policy. The image's original entrypoint is
wrapped by `/usr/bin/timeout` with a ten-minute lifetime and fifteen-second
termination grace. Missing limit support or a missing timeout executable fails
the check; there is no unlimited fallback. A stopped leftover container blocks
new captures until its ownership and cleanup are reviewed. See
[local validation safety](development-safety.md).

The optional `IGW_LIFECYCLE_EVIDENCE` path must be new. The probe publishes this
receipt only after all containment checks and independent cleanup verification
pass. It records the image/platform/configuration, test-binary checksum,
timestamps, configured lifetime, observed duration/exit code, and the checks
performed. A lifecycle receipt establishes containment for that image and
binary; it does not establish Gateway startup or API compatibility.

The invocation timeout is at most eight minutes and covers startup and HTTP
acquisition; parser work is size/depth bounded but is not yet interruptible.
The validation scope still bounds parser memory and runtime, and the container
has already been removed at that stage. A failed parser check retains the raw
document with an unvalidated receipt. A receipt without a qualified model must
not be bundled as a qualified reference.

Ignition 8.3.0 was also captured with the OPC UA whitelist. Its optional path
parameters and missing cancellation-ID schema have reviewed adapters, but the
keyboard-layout references also required the definition expansion above. The
full 446-operation model now parses with 334 reported adjustments and unchanged
raw/document/contract identities. The historical capture is not a qualified
reference: current containment, module inventory, and authenticated workflow
evidence are still required for that image. The complete version/module matrix and additional reference profiles
remain tracked in the [execution plan](plans/rebuild-v1.md). The serialized
[reference updater](reference-updates.md) has passed local 8.3.9 acceptance;
remote scheduling still requires a provisioned runner and explicit activation.

A fresh 8.3.0 default-module capture subsequently passed containment and recorded
32 healthy active first-party modules. Its capture-time parser rejected the EAM
and SFC defects above, and the coordinator stopped before workflow tests. The
exact document, original failed receipt, lifecycle evidence, and separate
current-parser qualification are retained as a parser fixture. It now parses
with 672 operations and 648 adjustments. This is reproducible parser evidence;
that original attempt is not a qualified offline reference or live workflow
acceptance. The subsequent clean policy-2 run passed the five covered
default-module workflow scopes on 8.3.0 and retained a complete qualified candidate
under `internal/referencebuild/testdata/ignition-8.3.0-policy2/`. Missing tag
transfer APIs are recorded separately, with verified refusal before dispatch.
See [reference update evidence](reference-updates.md#recorded-real-run).

## Authenticated API acceptance

After the pinned image passes the lifecycle probe, compile and run the separate
API acceptance test. Builds and live Gateway workloads remain separate jobs.

```sh
bash scripts/bounded-run.sh -- go test -c -o bin/testgateway.test ./internal/testgateway
bash scripts/bounded-run.sh -- env \
  IGW_ACCEPTANCE_TEST_IMAGE=inductiveautomation/ignition@sha256:28bd6b320157ec8dbbe465d0cd7c9f0ababfda4ff01c0bab982c0522a7b4eba2 \
  IGW_ACCEPTANCE_EVIDENCE=bin/api-acceptance.json \
  bin/testgateway.test -test.run '^TestLiveAPIResourceContract$' -test.v -test.timeout=7m
```

The test creates a dedicated security level and API key only inside its fresh
container. It adds the level to the existing read/write policies with signature
preconditions while retaining administrator access. Public read/write access
is never enabled. Browser cookies and CSRF protection are used for bootstrap;
CLI requests then use the complete `name:key` token with no cookies. Anonymous
and bare-key requests must still fail. The receipt is published only after
assertions and exact-owner cleanup succeed.

The 8.3.9 API run demonstrated create/read/update/delete of a basic schedule and
absence after preview. A description-only update preserved omitted configuration
and returned a new signature. A stale signature returned HTTP 500, and an
independent read proved the resource remained unchanged. Workflows therefore
cannot recognize concurrency conflicts from HTTP 409/412 alone; they need the
observed resource signature and state. Generic mutations still report
`accepted`, and the test performs verification separately. Dedicated resource
commands own these completion checks and report `completed` only after their
acknowledgement, signature, and readback checks agree.

The extended run passed 27 checks in 137.07 seconds on the same pinned 8.3.9
image, including type discovery, paginated listing, dedicated resource previews,
create/update/delete verification, duplicate-create refusal, and rejection of a
stale reviewed signature. Its receipt is
`internal/testgateway/testdata/ignition-8.3.9-resource-workflows.json`. The result
qualifies the recorded basic-schedule workflow and image; other resource types,
module sets, and versions require their own acceptance evidence.

The generated API key component alone is insufficient for the HTTP header.
Keep the full resource-name/key pair and assign a dedicated security level with
the required Gateway permissions; API keys do not impersonate a user's built-in
Administrator role. See the [vendor's credential example](https://forum.inductiveautomation.com/t/ign-13978-x-ignition-api-token-is-being-sent-lowercase-from-browsers/109911/2)
and [Gateway permission settings](https://docs.inductiveautomation.com/docs/8.3/platform/security/gateway-general-security-settings).

The resource list regression exposed an input-binding defect in validator
0.14.0: an absent optional exploded `filter` object receives scalar `limit`,
`offset`, or `search` values and rejects their names. Parser adapter version 6
introduced a private validation view; version 7 added project listing and
version 8 adds log listing. It omits this filter only when every supplied
query key names another scalar parameter and cannot match the filter's key
pattern. Supplied/ambiguous filters return `unsupported_serialization` until
their binding is qualified; explicit generic raw requests remain available.
Scalar constraints still run. Raw documents, contract identities, and stored model
parameters remain unchanged. Both captured catalogs exercise this behavior.

## Project and tag transfer evidence

The pinned 8.3.9 image passed the generic transfer contract test in 135.07
seconds. Its receipt is
`internal/testgateway/testdata/ignition-8.3.9-project-tag-contract.json`.
The test imported a disabled project into a different name, compared exported
archive contents, checked preview absence and duplicate refusal, then imported
and overwrote a memory tag and independently exported its values. It ran with
the same guarded container ownership, limits, API-token bootstrap, and cleanup
as resource acceptance. Run it with the precompiled acceptance binary using
`-test.run '^TestLiveProjectTagContract$'` and set `IGW_TRANSFER_EVIDENCE` to a
new receipt path. Optional `IGW_TRANSFER_ARTIFACTS` preserves exports in a new
private directory. This receipt qualifies generic transfers, not dedicated
workflow completion checks.

The captured OpenAPI declares tag import results as an array of quality codes.
The real 8.3.9 Gateway instead returned an object with `successCount`,
`failureCount`, and `failures`. A duplicate tag under the Abort collision policy
returned HTTP 200 with nonzero failures. A transport success is therefore
insufficient evidence of an applied import. Preserve the vendor document and
record this discrepancy; reviewed workflow policy must interpret the report
and verify resulting state. Unknown report shapes cannot establish completion.
This is a concrete reason that scheduled source updates must run workflow
acceptance as well as schema parsing and diffing.

The dedicated transfer workflow run passed 38 checks in 174.34 seconds on the
same image; its receipt is
`internal/testgateway/testdata/ignition-8.3.9-project-tag-workflows.json`.
Run `TestLiveProjectTagWorkflows` with the same opt-in environment above to
include these checks. It repeats the generic contract tests and qualifies
project listing, local archive inspection, verified imports, stale replacement
digest refusal, and replacement/readback. It also qualifies JSON memory-tag
import and overwrite, independent proof of preview absence/unchanged values,
and a duplicate Abort result reported as `partial` with exit 7 while the
existing tag value remains unchanged. Abort must not be assumed to roll back
every successful item in an import report. XML/CSV, UDT configurations,
Rename/Ignore verification, other modules, and other Gateway versions require
separate evidence.

## Operational workflow evidence

The pinned 8.3.9 operational run passed 23 checks in 114.20 seconds, with receipt
`internal/testgateway/testdata/ignition-8.3.9-operational-workflows.json`.
It qualifies filtered log pagination, Gateway backup ZIP and SQLite log
downloads, diagnostics preview non-mutation, collection, and repeated download.
The test reads every ZIP entry to check CRCs and compares file hashes with CLI
receipts. Runtime diagnostics verification separately checks reported ready
state and file size before publishing the completed download.

Run the precompiled acceptance binary with
`-test.run '^TestLiveOperationalWorkflows$'`, the same pinned image/bootstrap
environment above, and `IGW_OPERATIONS_EVIDENCE` pointing to a new receipt path.
Optional `IGW_OPERATIONS_ARTIFACTS` retains exports in a new private directory.
These files can contain sensitive diagnostic/configuration data; qualification
uses only its disposable Gateway and commits receipts, not the exported files.

The observed state vocabulary is `Invalid`, `Generating`, and `Valid`. Status
reports retain `Valid` after download. A repeat generation request returned
`Valid` and the same archive bytes: the route's "generate new" description
alone cannot prove freshness. The CLI records the acknowledgement state and
reports `gateway_latest` correlation with `size_matched` verification. Without
a job ID or server digest, it cannot distinguish a concurrent same-sized
bundle or certify that a new job ran. Scheduled compatibility checks must test
repeat generation and artifact behavior as well as the schema's state field.
