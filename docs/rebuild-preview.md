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
- `api list`, `describe`, and `request` using a selected target's catalog.
- `api raw` for explicit requests without schema validation.
- `gateway doctor`, which only reads Gateway information.
- `resource types`, `describe`, `list`, `get`, and named-resource
  `create`/`update`/`delete` with signatures, previews, and state verification.
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
Schema-assisted streaming requires a declared media type without a body schema
and reports `declared_transport` validation. Multipart/form
encoding, parameter serialization beyond explicit path/query/header values,
bounded batch, singleton resources, project/tag workflows, profile migration,
reference bundles, and the complete container qualification matrix remain on
the rebuild roadmap.

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
