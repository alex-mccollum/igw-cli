# v1 development CLI

The rebuild has an executable development entrypoint in `cmd/igw-next`. The
released `cmd/igw` remains available while workflows and real Gateway
compatibility are qualified. This is an implementation preview, not a v1
release. The accepted completion gates are in `docs/plans/rebuild-v1.md`.

Build and inspect the command tree:

```bash
go build -o bin/igw-next ./cmd/igw-next
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
state was checked. Task-specific completion verification is still pending.

The default invocation deadline is 30 seconds across discovery and execution.
In-memory response bodies default to a 16 MiB limit. `--out` streams directly
to atomic artifact storage; existing files require `--overwrite`. Request
bodies currently have a 32 MiB limit. Larger streamed uploads, multipart/form
encoding, parameter serialization beyond explicit path/query/header values,
resource signatures/diffs, bounded batch, task workflows, profile migration,
reference bundles, and container qualification remain on the rebuild roadmap.

Catalog storage is under the platform user cache directory at
`igw/catalog-v1`. This cache does not use the legacy CWD OpenAPI file. Local
imports are marked as references for offline inspection; they do not count as
fresh Gateway verification. `spec diff` lists changed operation definitions
and reports shared/path contract changes conservatively; it does not certify
backward compatibility. See `docs/catalog.md` for storage and authority details.
