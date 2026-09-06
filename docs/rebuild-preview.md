# v1 implementation status

The rebuilt command tree now runs through the single `cmd/igw` entrypoint.
`cmd/igw-next` and the legacy CLI/RPC implementation have been removed. The
source remains under qualification; cutover is not a published or release-ready
v1. The complete gates remain in `docs/plans/rebuild-v1.md`, and migration
guidance is in `docs/migration-v1.md`.

Build and inspect the command tree:

```bash
bash scripts/bounded-run.sh -- go build -o bin/igw ./cmd/igw
bin/igw --help
bin/igw schema --json
bin/igw completion bash
```

Examples for this entrypoint are maintained in
`docs/commands.md`. The command tree currently provides:

- `profile list`, `show`, `set`, `use`, `remove`, `migrate`, and `rollback`, with
  explicit local previews/writes, private storage, revision checks, and preserved
  configuration precedence. See `docs/profiles.md` for migration and recovery.
- `spec sync`, `inspect`, `import`, `export`, and `diff` for full documents.
- `spec references list`, `inspect`, and `export` for qualified offline bundles.
- `api list`, `describe`, and `request` using a selected target's catalog.
- `api capabilities` for the advertised or missing routes required by tag
  workflows and verified Gateway restart.
- `api list`, `describe`, and `capabilities --reference NAME_OR_DIRECTORY` for
  explicit offline discovery without target configuration or credentials.
- `api raw` for explicit requests without schema validation.
- `api batch` for bounded sequential JSON/text requests with ordered per-item
  results, zero-operation previews, and explicit continuation after ordinary
  failures.
- `gateway doctor`, which only reads Gateway information; `restart-tasks`; and
  `restart`, with a read-only preview and observed restart completion.
- `resource types`, `describe`, `list`, `get`, and named-resource
  `create`/`update`/`delete` with signatures, previews, and state verification.
- `project list`, `get`, `inspect`, `export`, and verified ZIP `import`, with a
  reviewed content digest required for replacement.
- `tag export` and `import`, with explicit collision policies, import-report
  checks, catalog prerequisites, and property readback for supported JSON imports.
- `backup export`, filtered `logs list`, and `logs download` with bounded,
  atomic artifacts.
- `diagnostics bundle status`, `collect`, and `download`, with bounded polling
  and state/size checks before publishing the latest bundle.
- `schema`, help, version metadata, and generated shell completions.

`--json` emits one result using `version: "igw/v1"`, including parse errors.
The result has `ok`, `outcome`, `data`, `error`, and `meta`, plus artifact metadata
when a file is saved. API JSON is structured data; large integer precision is
preserved. Errors use `kind`, `message`, `exitCode`, and optional `details`.
Transport URLs, response error bodies, credentials, and invalid instance values
are omitted from errors. Non-UTF-8 responses without `--out` use base64.

Request preparation and execution live in `internal/execute`, separate from
parsing and rendering. Previews report method, path, input names, body size and
digest, catalog evidence, and validation coverage. They never send the proposed
request or create a download. A schema-assisted write refreshes its contract
within the invocation and requires `--yes`. Mutations do not follow redirects
or automatically retry. A transfer failure after dispatch is conservatively
reported as `uncertain`. A successful generic mutation is `accepted`, with
`verification: "not_performed"`; it does not imply that the resulting Gateway
state was checked. Named resource workflows share a fresh catalog across
read/write/readback and require the Gateway's success flag plus observed state.
Update/delete execution requires the reviewed `--if-signature`; preview can
read and report it. Verified workflows report `completed` and `verified`, while
ambiguous outcomes stay `uncertain`. This qualifies stored configuration, not
the resource's operational health. See the command guide for body semantics,
collection behavior, and verification limits.

Gateway restart now has a typed baseline/write/readback workflow. It checks
the four advertised routes, reads node identity around each process/task
observation, dispatches one confirmed restart, and requires a process change or
uptime reset with matching reported identity and no pending tasks. It reports
`restart_observed` verification, with explicit acknowledgement and observational
correlation evidence. Neither HTTP availability nor an empty task list proves
restart by itself. Both pinned core images passed live qualification with
independent replacement-JVM observations and unchanged container limits.
The wrapper PID stayed unchanged, and separate containers shared a `localId`:
use a direct node URL and do not interpret that ID as globally unique. Retained
receipts keep their original correlation label; current wording limits the
claim to `selected_target`. See `docs/compatibility-matrix.md`.

The default invocation deadline is 30 seconds across discovery and execution.
In-memory response bodies default to a 16 MiB limit. `--out` streams directly
to atomic artifact storage; existing files require `--overwrite`. Request
bodies using `--body` have a 32 MiB limit. `--upload` snapshots a regular file
to private disk storage and streams it once, with a default 1 GiB limit.
Schema-assisted JSON bodies retain exact numbers and transmitted bytes;
duplicate keys, malformed Unicode, excessive nesting, and excessive numeric
work are refused before dispatch. Named query primitives and repeated exploded
arrays also receive complete value validation without changing wire text.
See `docs/commands.md` for the accepted input spellings and limits.
Schema-assisted streaming requires a declared opaque or unconstrained binary
body and reports `declared_transport` validation. Plain text has exact UTF-8
schema validation; unsupported body decoders are refused before dispatch.
Explicit empty text/files retain their presence and still receive the applicable
schema checks; previews distinguish them from omitted bodies with `bodyPresent`.
Simple path primitives retain exact values and complete constraints, including
operation overrides; validation uses the selected operation independently of
vendor server URLs. Structured path/header encodings remain in the active plan.
`api describe.bodyInputs` exposes these support boundaries, and generic request
metadata reports the checks actually performed. Multipart construction supports
literal text fields, streamed files, and ordered JSON part manifests with
transport-only coverage for schema-less declarations. URL-encoded input now has
bounded literal construction and object-schema validation for the supported
primitive/JSON/explicit-array bindings documented in `docs/commands.md`.
Multipart schema decoding, other form bindings, parameter serialization beyond
explicit path/query/header values,
singleton-resource live qualification, broader tag format/policy verification,
remote update-schedule activation, and final qualification
of the completed implementation remain on the rebuild roadmap.

Catalog storage is under the platform user cache directory at
`igw/catalog-v1`. This cache does not use the legacy CWD OpenAPI file. Local
imports are marked as references for offline inspection; they do not count as
fresh Gateway verification. Version 2 receipts distinguish original bytes,
canonical document identity, and the stable contract used for pins. Old receipts
are verified and requalified locally with a warning; old pins need explicit
replacement. `spec inspect FILE --summary --json` returns compact identities and
counts. `spec diff` distinguishes document changes from contract changes under
the reported policy; it does not certify backward compatibility. See
`docs/catalog.md` for storage and authority details.

The development binary embeds qualified 8.3.0 and 8.3.9 references with both
default modules and the minimal OPC UA profile. `spec references list` shows
each reference's exact image, module counts, contract identity,
assembly time, and qualification scope. Inspection reports individual module
versions and original acceptance evidence checksums. Export preserves a complete
standalone bundle; the same API discovery commands can use that directory on an
offline machine. Reference results use `meta.reference` and never claim live
target freshness. `parserVersion` identifies the recorded qualification;
`inspectionParserVersion` appears only after API discovery reparses the document
using the current parser. References are explicit and are never substituted for
a Gateway during request execution. The local reference updater has passed
default-module and minimal-profile acceptance on 8.3.0 and 8.3.9; the minimum
references explicitly record missing tag APIs. The initial four-cell matrix
has evidence under the current qualification policy, with each receipt retaining
its original source/parser identity. Remote schedule activation and final
current-source acceptance remain unfinished. See `docs/reference-updates.md` for retained
evidence and `docs/compatibility-matrix.md` for coverage boundaries.
