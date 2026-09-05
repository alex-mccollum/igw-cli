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
  artifacts. Previews and diagnostics never send mutating requests.
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
  metadata, raw SHA-256, canonical contract hash, and parser/policy versions.
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
- [ ] Disposable Gateway capture and real-schema qualification.
- [ ] New typed command/execution architecture.
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

## References

- [IA Gateway API documentation](https://www.docs.inductiveautomation.com/docs/8.3/platform/gateway/openapi)
- [IA resource metadata](https://www.sdk-docs.inductiveautomation.com/docs/8.3/programming-for-the-gateway/storing-data-using-resource-collections/resourcetypemeta/)
- [IA endpoint considerations](https://docs.inductiveautomation.com/docs/8.3/appendix/scripting-functions/system-config)
- [Official Ignition container](https://docs.inductiveautomation.com/docs/8.3/platform/docker-image)
- [Durable plans and acceptance criteria](https://developers.openai.com/cookbook/examples/codex/iterating-development-workflows-with-codex#agents-and-plans)
