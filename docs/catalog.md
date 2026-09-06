# Gateway catalog architecture

The v1 catalog implementation lives in `internal/catalog` and powers the
single CLI in `cmd/igw`. The legacy OpenAPI loader and CWD cache lookup have
been removed; see `docs/migration-v1.md` for command and cache migration.

The target Gateway's `/openapi.json` describes the documented routes provided
by its installed version and modules. IA's `/openapi` endpoint is a UI, so it
is not the first JSON discovery candidate. Runtime state, permissions, custom
validation, and operation effects require separate Gateway evidence. See the
[IA API documentation](https://www.docs.inductiveautomation.com/docs/8.3/platform/gateway/openapi).

The implementation retains the vendor's exact JSON bytes, one decoded JSON
document, and compact route/raw-definition indexes. It validates OpenAPI 3.0/3.1
structure with pinned embedded metaschemas and uses `jsonschema/v6` directly for
selected request schemas. Compiled schemas are reused within an invocation;
there is no YAML conversion or second full OpenAPI object model.

Descriptions retain vendor definitions, inherited path parameters, shared
components, security requirements, and documented gaps. The binding layer owns
path/query/header serialization, exact JSON numbers, body presence, supported
media decoders, and transport-only coverage. OpenAPI 3.0 nullable/exclusive-bound
semantics and request-body read-only requirements are adapted without changing
exports or contract pins. Ignition's document-root component references remain
bound to the registered in-memory document, including inside resource schemas
with relative identifiers.

External references and custom dialects are rejected without file or network
reads; import a self-contained document instead. Unsupported selected schemas
fail closed. Operation identity is `METHOD /path`; an operationId is accepted
only when it resolves to exactly one operation.

Snapshots are partitioned by profile and normalized effective Gateway URL,
including a reverse-proxy base path. Credentials are never stored in a snapshot.
Each target keeps `current.json`, `previous.json`, and at most two SHA-256-addressed
raw documents. A short cross-process lock protects atomic metadata publication
and copying bytes for readers. Network requests and schema parsing run outside
the lock. Publication refuses to replace a later verification with an earlier
one. A corrupt current snapshot produces a warning and falls back to the previous
snapshot. Successful publication removes obsolete blobs; interrupted publication
leaves the last complete snapshot usable. This is a disposable cache, not an
audit history.

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

Snapshot format 3 uses the `igw/catalog-v2` cache namespace. Older development
caches are left untouched and ignored. Run `spec sync` online, or import a raw
OpenAPI document for offline use. Profile configuration and current
`igw-contract/2` pins remain compatible; cache loading has no identity migration
layer and never advances a Gateway verification timestamp.

Discovery and reads refresh after 24 hours. A write revalidates during each
invocation, using ETag or Last-Modified when supplied. A failed refresh retains
the last valid snapshot. Reads can report a stale fallback; writes require an
explicit `--allow-stale-spec` and a previously fetched snapshot for the same
target. Explicit refresh requests fail if refresh fails. Offline imports and
bundled references cannot silently authorize writes. Gateway version and module
metadata remain absent until verified; the API document's info.version is not
assumed to be the Gateway version.

Every store load validates the retained bytes with the current parser. A
conditional [HTTP 304](https://www.rfc-editor.org/rfc/rfc9110.html#section-15.4.5)
can reuse that model only when the request sent its Gateway validator; an
identical fresh response can also reuse it. Different bytes always reparse.
Reuse transfers ownership after the new snapshot metadata is published, so failed
publication leaves the fallback usable. It does not skip write verification,
trust imported validators, or substitute contract equality for byte identity.
See [revalidation measurements](performance.md#unchanged-catalog-revalidation).

Library dependencies are pinned in go.mod/go.sum. Go 1.25.7 remains the
minimum supported toolchain. The catalog boundary exposes no library-specific
models to the CLI or workflow services. See [performance](performance.md) for
the JSON engine replacement measurements.

## Offline references

The runtime reference contains only `reference.json` and `openapi.json.gz`.
The manifest records current catalog identities, image/module provenance,
original qualification scope, and an evidence pointer with its SHA-256. The
contributor output additionally contains `evidence/`: original capture and
workflow receipts, registry manifests, and `qualification.json` with the detailed
comparison and file checksums. All inputs must qualify before output is created.
The runtime manifest is published last; existing output directories are refused.

The four bundled selectors remain `ignition-8.3.0-core`,
`ignition-8.3.0-defaults`, `ignition-8.3.9-core`, and `ignition-8.3.9-defaults`.
`spec references list` reads their manifests only. `inspect` and `export` verify
the compressed payload; export produces an independent two-file reference.
`api list`, `api describe`, and `api capabilities --reference REFERENCE` additionally
parse the document once and verify its current identities. None of these paths
loads Gateway configuration, credentials, target cache, or network clients.

`meta.reference.catalog` uses current policy `igw-contract/2`; pins compare this
identity. `qualification.catalog` and `qualification.parserVersion` preserve
what the original live evidence actually qualified. The summary's `parserVersion`
identifies that historical qualification; `inspectionParserVersion` and
`inspectionCatalog` are populated only after current parsing. `createdAt` retains
the original assembly date and is never a live-target verification timestamp.
Core references retain all 32 installed module records, including inactive ones.

Format conversion does not renew workflow qualification. Converted built-ins
point to their original full manifests in Git at commit `65e643d`. The evidence
URI is a provenance locator, never an instruction for the CLI to fetch files.
For new contributor output, `evidence/qualification.json` is relative to the
original contributor artifact; retain that artifact when promoting a reference.
Checksums establish integrity, not publisher authenticity. Choose a trusted
source. Old development reference formats are rejected with recovery guidance;
export a current built-in or import the raw OpenAPI document instead.

## Compatibility corrections

The vendor document remains unchanged in exports and hashes. Narrow adapters
handle observed generator defects: empty responses, selected path requirements,
resource identifiers, repeated resource-schema IDs, legacy query filters, and
the reviewed keyboard definition embedding. Each reports its scope through
`adjustments` and compatibility metadata. Unsupported declarations fail closed;
the CLI never silently invents a schema for an undocumented argument.

Synthetic regression tests isolate these defects. The four canonical vendor
fixtures cover both supported reference versions and module profiles, including
exact-number bindings, streaming declarations, resource signatures, discriminator
constraints, and unresolved schema refusal. Historical repeat captures and
parser investigations are [archived in Git](https://github.com/alex-mccollum/igw-cli/blob/65e643d/docs/catalog.md).

## Contributor qualification

Use the serial [reference update coordinator](reference-updates.md) to resolve an
official image once, verify containment, capture its document and module inventory,
exercise real workflows, and assemble a candidate. Qualification must identify
the exact current parser and acceptance executable. An old pass never qualifies
a new parser or an untested module combination.

On a shared workstation, every build, test, and large schema parse uses the
[bounded runner](development-safety.md). Before capturing a newly selected image,
the coordinator runs `TestLiveCaptureLifetime`: this opt-in probe verifies kernel
limits, lifetime termination, and cleanup. A prebuilt acceptance executable can
run the same probe with `IGW_CAPTURE_TEST_IMAGE` set to the immutable image and
`IGW_LIFECYCLE_EVIDENCE` set to a new output path, under the bounded runner. Never
start a capture with leftover qualification containers or an unavailable engine.
No repository command may repair WSL/Docker or change host memory settings.

See [current qualification status](qualification/README.md) for what has actually
been checked and [compatibility limits](compatibility-matrix.md) for workflow scope.
