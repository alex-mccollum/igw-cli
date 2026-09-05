# v1 development CLI

The rebuild has an executable development entrypoint in `cmd/igw-next`. The
released `cmd/igw` remains available while workflows and real Gateway
compatibility are qualified. This is an implementation preview, not a v1
release. The accepted completion gates are in `docs/plans/rebuild-v1.md`.

Build and inspect the command tree:

```bash
bash scripts/bounded-run.sh -- go build -o bin/igw-next ./cmd/igw-next
bin/igw-next --help
bin/igw-next schema --json
bin/igw-next completion bash
```

Examples for this entrypoint are maintained in the development section of
`docs/commands.md`. The command tree currently provides:

- `profile list` and `profile show`, using existing configuration precedence and
  environment variables without displaying token values.
- `spec sync`, `inspect`, `import`, `export`, and `diff` for full documents.
- `spec references list`, `inspect`, and `export` for qualified offline bundles.
- `api list`, `describe`, and `request` using a selected target's catalog.
- `api capabilities` for the advertised or missing routes required by tag
  workflows.
- `api list`, `describe`, and `capabilities --reference NAME_OR_DIRECTORY` for
  explicit offline discovery without target configuration or credentials.
- `api raw` for explicit requests without schema validation.
- `gateway doctor`, which only reads Gateway information.
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
`api describe.bodyInputs` exposes these support boundaries, and generic request
metadata reports the checks actually performed. Multipart construction supports
literal text fields, streamed files, and ordered JSON part manifests with
transport-only coverage for schema-less declarations. Multipart schema decoding,
URL-encoded forms, parameter serialization beyond explicit path/query/header values,
bounded batch, singleton resources, broader tag format/policy verification,
profile migration, remote update-schedule activation, and final qualification
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
