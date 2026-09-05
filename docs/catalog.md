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
CLI and workflow contracts.

## Real Gateway qualification

The test suite includes the exact compressed document captured from an official
Ignition 8.3.9 container with its default modules. It contains 687 operations
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

Policy `ignition-openapi/1` matches the observed generator identity and those
exact structural shapes. Each snapshot binds the policy to its original raw
SHA-256. It does not grant trust based on the document's title or license URL;
the resulting model must still pass OAS validation and reference checks.
Descriptions and exports retain the vendor definitions. `spec inspect` exposes
every adjustment with its operation and JSON pointer; snapshot metadata reports
counts by rule. Any additional defect remains an error. This adapter implements
the [OAS parameter and response rules](https://spec.openapis.org/oas/v3.1.0.html).

Recursive arrays, such as required security-level children, accept finite trees
and are checked against actual input values. The parser receives a formatted
private copy: its node index otherwise performs quadratic work on large compact
JSON. Eager compilation of unrelated request/response schemas is disabled.
Local discovery of the captured expanded document took about two seconds with
roughly 600 MiB peak RSS on the development machine; memory needs optimization
before release qualification.

Document/model validation is not proof that every request schema is usable.
For example, 8.3.9 schedule creation repeats `$id` in `config` and `backupConfig`,
which fails JSON Schema compilation. This produces a `catalog_schema` error,
not a claim that the user's payload is invalid. Explicit raw requests remain
available. A separate reviewed solution and real mutation tests are required
before those resource workflows are qualified.

Two fresh instances also produced different example timestamps in scan-lock
responses. The current conservative contract hash includes examples, so a pin
can change after restart even when input constraints do not. Separating stable
wire-contract identity from documentation/example drift is still required.
A later comparison of the guarded capture with the committed fixture found
56 reordered `oneOf` arrays and four reordered `enum` arrays, in addition to
the two example timestamps. Stable pinning must account for these set-like
schema arrays without changing ordered data arrays or reference semantics.

## Contributor capture

`cmd/igw-capture` creates a fresh, uniquely labeled container from a pre-pulled
official image digest, publishes HTTP only on loopback, disables quickstart and
Gateway Network, and removes its own container and anonymous volumes before
parsing the document. A fixed container name is an exclusive engine-wide slot;
the unique label and exact ID prevent cleanup of another session's container.
It uses random temporary admin credentials, copied privately with the image's
numeric user ownership. Passwords never enter Docker arguments or environment
values. It neither connects to nor reconfigures an existing Gateway.

After `StatusPing` reports RUNNING, the tool authenticates to the disposable
Gateway's built-in IdP and waits for three identical OpenAPI responses. This
private browser-login adapter is confined to test infrastructure; the released
CLI continues to use API tokens. The 8.3.9 run confirmed that `/openapi.json`
requires authentication and that `/data/api/v1/gateway-info` reports the actual
runtime version.

Run the short lifecycle probe before qualifying a new image. It verifies the
engine's configured and kernel-applied limits, rejects a second capture, waits
for a shortened in-container lifetime to expire, and verifies removal. Compile
first so builds and Gateway startup do not overlap. Ordinary test runs skip
this live test unless the image environment variable is explicitly supplied.

```sh
docker pull inductiveautomation/ignition@sha256:28bd6b320157ec8dbbe465d0cd7c9f0ababfda4ff01c0bab982c0522a7b4eba2
mkdir -p bin
bash scripts/bounded-run.sh -- go test -c -o bin/testgateway.test ./internal/testgateway
bash scripts/bounded-run.sh -- env \
  IGW_CAPTURE_TEST_IMAGE=inductiveautomation/ignition@sha256:28bd6b320157ec8dbbe465d0cd7c9f0ababfda4ff01c0bab982c0522a7b4eba2 \
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

The invocation timeout is at most eight minutes and covers startup and HTTP
acquisition; parser work is size/depth bounded but is not yet interruptible.
The validation scope still bounds parser memory and runtime, and the container
has already been removed at that stage. A failed parser check retains the raw
document with an unvalidated receipt. A receipt without a qualified model must
not be bundled as a qualified reference.

Ignition 8.3.0 was also captured with the OPC UA whitelist. That document needs
additional review for optional path parameters and a missing cancellation-ID
schema; it is not yet a qualified fixture. The complete version/module matrix,
observed module inventory, authenticated workflow tests, scheduled update
automation, and independently distributed reference bundles remain tracked in
the [execution plan](plans/rebuild-v1.md).
