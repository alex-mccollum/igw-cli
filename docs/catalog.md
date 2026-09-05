# Gateway catalog architecture

The v1 catalog implementation lives in `internal/catalog`. It is currently
being built alongside the existing CLI; the existing `api sync` command still
uses the legacy loader until command cutover.

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
parser version, and two hashes. Atomic file publication coordinates concurrent
writers. The newest complete valid receipt wins, so a slower older refresh
cannot replace a newer one through a shared latest pointer. A corrupt newer
receipt produces a warning and falls back to a valid older snapshot.

The raw hash checks byte integrity. The contract hash normalizes object key
order and JSON whitespace while retaining number precision. It conservatively
includes all document fields; it does not claim that equivalent schema syntax
always has an equal hash. Pins use this contract hash.

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
CLI and workflow contracts. Tests currently use clearly labeled synthetic
OpenAPI fixtures; real Ignition capture and compatibility qualification remain
required by `docs/plans/rebuild-v1.md`.
