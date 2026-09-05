# Commands

## v1 Development Entrypoint

The staged rebuild is available through `cmd/igw-next`; current release commands
remain below. Scope and limitations are in `docs/rebuild-preview.md`.

```bash
go build -o bin/igw-next ./cmd/igw-next
bin/igw-next schema --json
bin/igw-next profile show --json
bin/igw-next spec sync --json
bin/igw-next api list --search gateway --json
bin/igw-next api describe 'GET /data/api/v1/gateway-info' --json
bin/igw-next api request 'GET /data/api/v1/gateway-info' --json
bin/igw-next gateway doctor --json
bin/igw-next api raw --method POST --path /data/api/v1/scan/projects --dry-run --json
bin/igw-next spec export --out gateway-openapi.json --json
bin/igw-next spec inspect gateway-openapi.json --json
bin/igw-next spec inspect gateway-openapi.json --summary --json
bin/igw-next spec import gateway-openapi.json --json
bin/igw-next api list --offline --json
bin/igw-next spec diff before-openapi.json after-openapi.json --json
```

Qualified API references are available without Gateway configuration, credentials,
network access, or a populated cache:

```bash
bin/igw-next spec references list --json
bin/igw-next spec references inspect ignition-8.3.9-defaults --json
bin/igw-next spec references export ignition-8.3.9-defaults --out ./reference-8.3.9 --json
bin/igw-next api list --reference ignition-8.3.9-defaults --search gateway --json
bin/igw-next api describe 'GET /data/api/v1/gateway-info' --reference ./reference-8.3.9 --json
bin/igw-next api capabilities --reference ignition-8.3.9-defaults --json
bin/igw-next api capabilities --reference ignition-8.3.0-core --json
```

Bundled selectors cover 8.3.0 and 8.3.9 with suffixes `-defaults` and `-core`.
For example, `ignition-8.3.9-core` records the minimal OPC UA profile. JSON
metadata includes `moduleCount` for all installed observations and
`activeModuleCount` for those observed active; the core profile has 32 and 1
respectively. `moduleProfile` identifies explicit newer qualification; older
manifests retain their original all-active evidence. Human output shows both
counts and labels historical manifests `legacy all-active`. The 8.3.0 references
report tag-transfer workflows as unavailable. See the [qualification matrix](compatibility-matrix.md).

`REFERENCE` accepts a bundled selector or a local bundle directory; prefix a
relative directory with `./` if it has the same name as a bundled selector.
Export requires a new directory and preserves the complete evidence bundle,
including exact compressed vendor JSON. It never replaces a previous bundle.
`--spec-pin SHA256` checks the reference contract for inspection, export, and API
discovery. `--reference` is available only on `api list`, `api describe`, and
`api capabilities`.
These commands report provenance in `meta.reference`, with no target or target
catalog receipt. They do not populate the Gateway cache or establish authority
for a request. An invalid reference fails explicitly. See `docs/catalog.md` for
the source-of-truth and qualification boundaries. On a shared Linux/WSL
workstation, wrap API discovery in `bash scripts/bounded-run.sh --` because it
parses the captured model; reference listing, inspection, and export only verify
bounded metadata and payload checksums.

`api request` accepts either an exact `METHOD /path` key or an unambiguous
operationId. Use repeatable `--path-param name=value`, `--query key=value`,
and `--header name:value` for parameters and `--body @file.json` for input.
Preview mutations with `--dry-run`; execution requires `--yes`. A preview
may fetch the API document but never sends the proposed request.

For simple path parameters, pass the literal string, boolean, integer, or number
with `--path-param name=value`; the CLI handles percent encoding. Encoded slashes
remain inside the value, and percent escapes are decoded once for validation.
Empty path values fail before dispatch. The same exact numeric spellings and
limits described below for query values apply to path values. The complete
selected schema applies, and an operation's declaration overrides an inherited
declaration of the same parameter. Vendor `servers` never changes the selected
target or the operation-relative path used for validation.

Path arrays, objects, content-based parameters, and label/matrix styles currently
fail schema-assisted validation as `unsupported_serialization`; their contracts
remain inspectable with `api describe`. A path template with multiple expressions
in one segment must have an unambiguous binding. `api raw` remains the explicit
escape hatch for encodings that are not yet supported.

`--header name:value` supplies HTTP field text. Header names are case-insensitive;
repeated flags retain their value order. Leading/trailing HTTP spaces and tabs
are removed from field values. Unicode whitespace, commas, percent escapes,
and internal tabs remain literal; the CLI does not URL-decode header values.
An empty value supplies an empty field, while omitting the flag supplies none.
Invalid field names or control bytes fail with exit 2 before discovery or
dispatch, including during previews. Errors do not echo supplied field text.
The same checks apply to `--content-type`; a whitespace-only media type fails.
Authentication, routing, and body-framing headers remain managed by the CLI.

For a declared simple header parameter, schema checks use the normalized HTTP
text. Supply one field for a primitive; repeated primitive fields fail. Arrays
combine repeated fields and comma-separated members into one ordered value,
with complete item, length, and uniqueness checks. HTTP spaces/tabs around
members are removed; percent escapes and quotes remain literal. Empty list
members are ambiguous and fail validation. No header-specific quoting grammar
is inferred. Exact numeric and boolean rules match path/query values.

When the parameter declares `content`, supply one field containing its declared
JSON or UTF-8 `text/plain` representation. JSON can express objects, empty lists,
empty members, and null unambiguously; nested constraints and exact numbers are
checked. Duplicate JSON keys, trailing JSON, invalid Unicode, and unsupported
encodings fail before dispatch. Each declared header value is limited to 32 MiB
across its field lines. An operation's declaration overrides an inherited
header case-insensitively; conflicting declarations at the same level fail.
Absent optional custom headers need no decoder.

OpenAPI's `Accept`, `Content-Type`, and `Authorization` parameter declarations
are ignored as the specification requires. Body media checks still apply.
The managed API token is checked for required presence only; its actual value
never enters schema validation, and the Gateway checks authentication. Declared
headers generated by HTTP (such as `Host` or an omitted `User-Agent`) fail as
`unsupported_serialization` when their effective values cannot be established.
Simple object headers and other unsupported encodings remain inspectable with
`api describe`; `api raw` is the explicit escape hatch.

`api describe` includes `bodyInputs` alongside the original vendor contract.
Each entry reports the declared media type, required-body flag, schema presence,
supported encoding, validation coverage, and streaming support. `selected_media`
means that an actual content type is needed to resolve support for a media range.
Malformed vendor media types remain visible as `unsupported`.

For named query parameters, supply one value for a primitive, or repeat the
same key for each item in an exploded form array. A comma inside an array item
remains part of that item. For example, this previews two session IDs:

```bash
bin/igw-next api request 'DELETE /data/perspective/api/v1/sessions' --query sessionId=SESSION_1 --query sessionId=SESSION_2 --dry-run --json
```

The Gateway's complete parameter schema validates each primitive or whole
array, including enums, numeric bounds, item constraints, array size, and
uniqueness. Repeating a primitive parameter fails before dispatch. Strings
retain whitespace, commas, reserved characters, and Unicode; the CLI performs
URL encoding. Supplied empty strings are validated as values, with no implicit
null, default, or omission substitution. Booleans use `true` or `false`.
Integers use JSON decimal integer syntax; numbers also accept JSON decimal
fractions and exponents. Numeric validation preserves precision, with a limit
of 4096 characters and an exponent between -4096 and 4096 to bound computation.
The original wire text is never rounded or rewritten by validation.

This covers the named primitive and exploded primitive-array query shapes in
the qualified Gateway catalogs. Other query styles, nested values, and schema
types without an unambiguous text representation require additional encoding
support; use `api raw` explicitly for those cases. Header/path encoding and
multipart input have separate contracts and remaining implementation work.

JSON request bodies are decoded without rounding numbers and validated against
the selected request schema. The CLI sends the original bytes, including
whitespace and numeric spelling; the preview digest covers those same bytes.
Absent bodies, `null`, empty strings, `false`, and zero remain distinct.
Integral JSON numbers such as `1.0` and `10e-1` satisfy an integer schema.
Duplicate object keys, multiple JSON values, invalid UTF-8, unpaired Unicode
escapes, and nesting beyond 256 levels are refused. JSON schema validation is
bounded to 32 MiB and uses the same numeric text/exponent limits as query values.
The most specific declared media type applies. A `+json` suffix selects JSON
decoding but does not make that type interchangeable with `application/json`.
This validation does not rewrite defaults or remove properties; Gateway-side
validation and permission checks still apply.

`text/plain` bodies are validated as exact UTF-8 strings, including whitespace,
enums, patterns, and character-length constraints. The optional charset can be
`utf-8` or `us-ascii`; ASCII requires ASCII bytes. Other charset/encoding support
must be implemented before schema-assisted requests accept it. Unsupported
schema encodings return `unsupported_input` with exit 2 before dispatch. Bodies
without a declared request-body contract, missing required bodies, and missing
or invalid content types also fail before dispatch; `api raw` remains explicit.

`--body ''`, an empty `@file`, empty stdin, or an empty `--upload` file explicitly
supplies a zero-byte body. Omit the input option to omit the body. A required
body requires an explicit input, and empty text still must satisfy its schema
(for example, `minLength: 1` rejects it). A JSON empty string is `--body '""'`;
a zero-byte JSON input fails decoding. An explicit content type alone does not
supply a body. The supplied content type is retained on the HTTP request.

Generic previews include `bodyPresent` as well as `bodyBytes`. Explicit empty
inputs have `bodyPresent: true` and the SHA-256 of zero bytes; omitted inputs
have `bodyPresent: false` and no body hash. With an omitted optional body, only
the applicable presence and parameter checks run.

Generic request results report actual coverage in `meta.validation`; previews
also retain `data.validation`. `declared_schema` means the supported declared
schema checks passed. `declared_transport` means the body received media-type
and presence checks, with no validation of its contents. Raw requests report
`not_requested`. Multipart schema decoding, URL-encoded form construction, and
additional schema encodings remain unfinished.

For an opaque binary body, use `--upload FILE --content-type MEDIA_TYPE`.
The input must be a regular file; the CLI creates a private disk snapshot so
preview metadata and transmission use the same bytes within an invocation.
`--max-upload-bytes` defaults to 1 GiB. The snapshot is removed on completion.
`--body` and `--upload` are mutually exclusive. Schema-assisted streaming is
available when the selected media type has no body schema or uses a recognized
unconstrained binary schema: an empty schema for `application/octet-stream`, or
`type: string` with `format: binary`. Media ranges use the same most-specific
selection as bounded bodies. Its coverage is `declared_transport`; archive,
tag, or other file contents still require server or workflow validation.
Additional binary value constraints are refused until supported. Use bounded
`--body` for a supported schema decoder, or `api raw` explicitly.

```bash
bin/igw-next api request 'POST /data/api/v1/projects/import/{name}' --path-param name=Example --upload project.zip --content-type application/zip --dry-run --json
```

For multipart uploads, use repeatable `--form-field name=value` and
`--form-file name=path`. Text values are literal UTF-8, so `@file`, `-`, commas,
and equals signs inside a value are not interpreted as input selectors. Empty
text values and repeated names are supported. Shorthand fields are emitted
first in their supplied order, followed by files in their supplied order.

Use `--multipart @parts.json` when part order, transmitted filenames, or media
types need explicit control. The manifest is an ordered JSON array:

```json
[
  {"name": "note", "text": "Reviewed input"},
  {"name": "files", "file": "./payload.bin", "filename": "payload.bin", "contentType": "application/octet-stream"},
  {"name": "note", "text": ""}
]
```

Set `operation` to the operation key selected from your Gateway. Use the part
names and file formats required by that endpoint's documentation; the names
above illustrate a manifest, not a vendor endpoint contract.

```bash
bin/igw-next api request "$operation" --multipart @parts.json --dry-run --json
```

`--multipart` also accepts inline JSON or `-` for stdin. The manifest is limited
to 1 MiB and 256 parts. Offline `schema --json` includes its JSON `inputSchema`
on the flag definition. Each part needs `name` and exactly one of `text` or
`file`. Optional `filename` applies only to files; its default is the local
basename. Relative file paths resolve from the working directory. Text defaults
to `text/plain; charset=utf-8`; files default to `application/octet-stream`.
`contentType` can set an explicit concrete media type. Files are sent as raw
bytes; text is not coerced, base64-encoded, or parsed as another format.

Multipart options cannot be combined with `--body`, `--upload`, or
`--content-type`; the CLI owns the boundary and outer content type. The JSON
manifest cannot be mixed with shorthand fields/files. `--max-upload-bytes`
limits the complete encoded body, including MIME framing. The CLI keeps the
final private snapshot and at most one temporary source-file snapshot at a time;
temporary disk use can approach twice the configured upload limit. All owned
snapshots are removed after the invocation.

Previews include ordered `parts` with names, transmitted filenames, content
types, sizes, and SHA-256 hashes. They omit local paths and field values. The
outer body hash includes a generated boundary and can differ across invocations;
compare part hashes when reviewing the same inputs again. Within an invocation,
the preview and transmission use one immutable encoded snapshot.

Schema-assisted multipart construction currently requires a selected media type
without a body schema and reports `declared_transport`. The captured 8.3.9 bulk
datafile routes declare multipart without part schemas; 8.3.0 lacks those bulk
routes. No part names or value constraints are inferred from those gaps.
Schema-bearing multipart requests are refused until their decoder is supported.
Explicit `api raw` supports the same multipart options without catalog claims.

Named resource workflows discover their routes from the selected Gateway's
catalog and verify successful changes with an independent read:

```bash
bin/igw-next resource types --offline --json
bin/igw-next resource describe ignition/schedule --json
bin/igw-next resource list ignition/schedule --limit 50 --offset 0 --json
bin/igw-next resource list ignition/schedule --filter 'name[eq]=Example' --json
bin/igw-next resource get ignition/schedule Example --collection core --json
bin/igw-next resource create ignition/schedule Example --body @schedule-fields.json --dry-run --json
bin/igw-next resource create ignition/schedule Example --body @schedule-fields.json --yes --json
bin/igw-next resource update ignition/schedule Example --body '{"description":"Day shift"}' --dry-run --json
bin/igw-next resource update ignition/schedule Example --body '{"description":"Day shift"}' --if-signature REVIEWED_SIGNATURE --yes --json
bin/igw-next resource delete ignition/schedule Example --dry-run --json
bin/igw-next resource delete ignition/schedule Example --if-signature REVIEWED_SIGNATURE --yes --json
```

`resource list`, `project list`, and `logs list` accept repeatable
`--filter 'field[operator]=value'`. Quote the complete expression for your shell.
For example, combine `--filter 'name[sw]=Line'` with
`--filter 'enabled[eq]=true'`. The selected Gateway's schema validates operator
spelling and value constraints. Each field/operator key can appear once; values
remain exact text, including `+`, `&`, `=`, Unicode, and large numeric strings.
The existing pagination and search options remain separate parameters.

Generic requests use the same validation with
`--query 'name[eq]=Example'`. An exploded filter uses its property keys directly;
`--query 'filter={...}'` is not that wire format. Invalid or ambiguous filters
fail before an operation request. Other object serializations and nested filter
values still need explicit encoding support; `api raw` remains available.

Replace `REVIEWED_SIGNATURE` with the `data.signature` from `get` or the
`data.beforeSignature` from the corresponding preview. A changed signature
stops execution before a mutation; the observed signature is also sent to the
Gateway to cover changes between the read and write. Create requires observed
absence and uses the Gateway's create endpoint. None of these commands replay
failed writes or force deletion of referenced resources.

`--body` is one object containing `description`, `enabled`, `config`, and/or
`backupConfig`. Name, collection, and signature come from command arguments;
the CLI builds the API's array wrapper. Omitted top-level update fields remain
unchanged. Supply a complete `config` or `backupConfig` when changing it; this is
not a recursive merge patch. A basic `schedule-fields.json` can contain:

```json
{"description":"Day shift","enabled":true,"config":{"profile":{"type":"basic schedule"},"settings":{"allDays":true,"allDayTime":"08:00-16:00"}}}
```

Get and changes default to collection `core`; list uses the Gateway's active
collection and returns one page with the Gateway's pagination metadata. Resource
types can be discovered offline, but state reads and change previews require
connectivity. Previews expose changed field names and a request digest without
configuration values. Explicit `get` and `list` return resource configuration.

Successful workflows report `outcome: "completed"` and
`meta.verification: "verified"` after response/state checks. This verifies stored
configuration, not operational health of the configured connection or device.
Verification compares submitted values, permits additional Gateway defaults,
and checks that omitted top-level writable fields remain unchanged. It does not
prove removal of unspecified nested properties.
If acknowledgement, signature, or readback cannot establish the outcome,
the result stays failed or uncertain. Redacted secrets or vendor-normalized
values that cannot be compared can prevent verification; inspect current state
before retrying. Singleton resources and forced reference changes still use
explicit generic API requests.

Project workflows transfer a complete project ZIP, inspect its file manifest,
and verify the imported contents through a fresh export:

```bash
bin/igw-next project list --limit 50 --offset 0 --json
bin/igw-next project get Example --json
bin/igw-next project export Example --out project.zip --json
bin/igw-next project inspect project.zip --json
bin/igw-next project import Copy --in project.zip --dry-run --json
bin/igw-next project import Copy --in project.zip --yes --json
bin/igw-next project import Copy --in project.zip --overwrite --dry-run --json
bin/igw-next project import Copy --in project.zip --overwrite --if-project-sha256 REVIEWED_DIGEST --yes --json
```

`project inspect` is local and requires no Gateway configuration. Exports are
validated privately before publication; an existing output file requires
`--overwrite`. Import defaults to requiring destination absence. Replacing an
existing Gateway project requires `--overwrite` and the `data.beforeSha256`
from a reviewed preview as `REVIEWED_DIGEST`. The project API has no atomic
revision precondition: this digest detects earlier changes but cannot prevent
a concurrent edit between the check and import. The result reports that limit.

Project fingerprints ignore ZIP timestamps and entry order. JSON files up to
4 MiB use key-order and number normalization; other files compare byte digests.
Only standard ZIPs with unique relative paths and an object-valued
`project.json` are accepted, with at most 10,000 entries, an 8 MiB central
directory, and 1 GiB total expanded content. ZIP64, prefixed archives, and
symbolic links are unsupported. Nothing is extracted or executed locally.
Post-import export must match the complete file manifest before the command
reports `completed` and `verified`. This checks project files, not runtime
health. Project exports do not contain Gateway resources or tag providers.

Tag imports default to the `Abort` collision policy and verify supported JSON
inputs through independent tag exports:

```bash
bin/igw-next api capabilities --json
bin/igw-next tag export --provider default --path Example --out tags.json --json
bin/igw-next tag import --provider default --in tags.json --dry-run --json
bin/igw-next tag import --provider default --in tags.json --yes --json
bin/igw-next tag import --provider default --path Destination --in tags.json --collision-policy Overwrite --dry-run --json
bin/igw-next tag import --provider default --path Destination --in tags.json --collision-policy Overwrite --yes --json
```

`api capabilities` currently assesses tag workflow prerequisites from the
selected catalog, including offline snapshots and explicit references. Each
entry reports its required and missing operation keys with status `advertised`
or `unavailable`. `advertised` means the routes exist in that document; request
validation and Gateway permissions are still checked when the workflow runs.
Discovery itself exits 0 even when a workflow is unavailable; inspect its status.
No version label is used to infer support. The captured 8.3.0 Gateway lacks both
tag transfer routes; the captured 8.3.9 Gateway advertises them.

Tag workflows refuse missing prerequisites with `error.kind: "capability"`,
exit 2, missing-operation details, and catalog provenance before sending an
operation or creating a download. JSON imports with Abort, Overwrite, or
MergeOverwrite require both import and export, including during preview, so a
missing readback route is detected before a write. XML/CSV and Rename/Ignore
retain their explicit acknowledgement-only behavior and require the import
route. This check does not introduce a fallback to an undocumented API.

Provider defaults to `default`; `--path` is relative to that provider. Export
defaults to JSON, recursively including children and UDT definitions; use
`--recursive=false`, `--include-udts=false`, or `--type xml` explicitly.
Export requires `--out` and publishes the completed download atomically.
Import accepts JSON, XML, or CSV; type is inferred from a known extension,
defaults to JSON for an extensionless file, and can be set with `--type`.
JSON verification inputs are limited to 32 MiB; opaque uploads default to 1 GiB.

The CLI checks import reports even when HTTP status is 200. Failures return
exit 7; known mixed results report `partial`. JSON with `Abort`, `Overwrite`,
or `MergeOverwrite` requires all supplied tag properties and named children to
match a readback before reporting `completed` and `verified`. This allows
Gateway defaults and does not prove removal of unspecified properties or
children, atomic application, or future values of dynamic tags. Redacted,
normalized, inherited, or changing values can prevent verification. XML/CSV
and `Rename`/`Ignore` currently report `accepted` with verification unavailable
when the Gateway reports no failures. Unknown reports and readback mismatches
remain `uncertain`; inspect exported state before retrying. Imports are never
automatically replayed. Previews describe structure and digests without tag
values or project file contents; explicit exports contain the selected data.

Operational commands provide bounded log queries, complete downloads, and
diagnostics collection through one invocation:

```bash
bin/igw-next backup export --out gateway.gwbk --json
bin/igw-next logs list --limit 50 --min-level WARN --since 1h --json
bin/igw-next logs list --logger Gateway --since 2026-09-05T08:00:00-07:00 --until 2026-09-05T09:00:00-07:00 --json
bin/igw-next logs download --out system-logs.idb --json
bin/igw-next diagnostics bundle status --json
bin/igw-next diagnostics bundle collect --out diagnostics.zip --dry-run --json
bin/igw-next diagnostics bundle collect --out diagnostics.zip --yes --timeout 2m --json
bin/igw-next diagnostics bundle download --out diagnostics-latest.zip --json
```

All downloads require `--out`, default to a 1 GiB `--max-bytes` limit, and need
`--overwrite` to replace an existing file. Complete files include byte counts
and SHA-256 receipts. Backup export optionally accepts `--include-peer-local`
for files from a redundant peer. Log download returns the complete internal
SQLite log database on the qualified 8.3.9 Gateway. `logs list` returns one
page, supports `--offset`, `--logger`, and `--search`, and accepts case-insensitive
minimum levels. `--since` accepts a positive duration measured from the local
invocation clock or an RFC3339 timestamp; `--until` requires an RFC3339 timestamp.
Both are sent as epoch milliseconds. A log query does not change logger levels.

Diagnostics status normalizes the qualified Gateway states `Invalid`,
`Generating`, and `Valid` to `empty`, `generating`, and `ready`, retaining the
original value in `gatewayState`. Unknown or incomplete status reports fail
explicitly. A positive file size alone never establishes readiness.

`collect` requires `--yes`, refuses to start while generation is already
running, requests generation once, polls until ready, and downloads privately.
The Gateway can acknowledge a generation request as already `Valid` and return
the existing bundle. `generationState` records the acknowledgement; the CLI
warns when a new generation job is unproven.
`--interval` defaults to one second and accepts 100ms through one minute.
`--timeout` bounds discovery, generation, polling, and download together.
Temporary transport errors and HTTP 429/502/503/504 during polling can be
retried within that deadline; generation requests are never replayed. Timeout
or failed verification after generation reports an uncertain outcome and
preserves an existing destination. Generation can continue on the Gateway.

Before publishing, collection and download compare the received byte count
with the reported size and recheck ready state and size. Results explicitly
report `verification: "size_matched"` and `correlation: "gateway_latest"`:
the API exposes no job ID or server content digest, so another client creating
a same-sized bundle cannot be distinguished. Previews read status but do not
start a job or create a file. Bundle generation can consume Gateway resources;
the CLI uses the Gateway's existing configuration and does not enable heap
dumps. See the [vendor's diagnostics documentation](https://docs.inductiveautomation.com/docs/8.3/platform/gateway/web-interface/diagnostics)
for bundle contents and generation settings.

## Current Release Entrypoint

This file is the canonical command example reference.
For script/agent workflow guidance, see `docs/automation.md`.

Defaults and behavior:
- `igw call` defaults `--method` to `GET` when `--path` is provided.
- `igw call --batch` supports JSON array or NDJSON input (`--batch @file|file|-`) with one response envelope per item.
- `igw call --stream` streams successful response bodies directly in non-JSON mode.
- Repeat `--select` to extract a subset JSON object from output (requires `--json`), with dot paths and array indexes (`checks.0.name`).
- `--raw` prints one plain selected value and requires exactly one `--select`.
- `--compact` prints one-line JSON (requires `--json`).
- `--timing` and `--json-stats` expose latency/runtime stats on machine-facing commands.
- `igw tags export` defaults `--provider=default` and `--type=json`.
- `igw tags import` defaults `--provider=default`, infers `--type` from `--in` file extension (`.json`, `.xml`, `.csv`; fallback `json`), and defaults `--collision-policy=Abort`.
- `igw logs download`, `igw diagnostics bundle download`, and `igw backup export` default output filenames whenever `--out` is omitted.
- Mutating commands require `--yes`.
- API discovery defaults to `openapi.json` in the current directory, then `${XDG_CONFIG_HOME:-~/.config}/igw/openapi.json`.
- `igw api stats --prefix-depth N` groups path prefixes by exactly `N` path segments (`0` uses auto grouping).
- If default spec files are missing, `api` and `call --op` auto-sync and cache OpenAPI from the gateway.

Build:

```bash
go build ./cmd/igw
```

Test:

```bash
go test ./...
```

Version:

```bash
igw version
```

Machine contracts:

```bash
igw exit-codes
igw exit-codes --json
igw schema
igw schema --command "config profile"
igw schema --select command.subcommands.0.name --raw
```

API docs discovery:

```bash
igw api list --spec-file /path/to/openapi.json --path-contains gateway
igw api show --spec-file /path/to/openapi.json --path /data/api/v1/gateway-info
igw api show --spec-file /path/to/openapi.json /data/api/v1/gateway-info
igw api search --spec-file /path/to/openapi.json --query scan
igw api tags --spec-file /path/to/openapi.json
igw api stats --spec-file /path/to/openapi.json --json
igw api stats --spec-file /path/to/openapi.json --prefix-depth 2 --json
igw api capability --spec-file /path/to/openapi.json --json file-write
igw api sync --profile dev --json
igw api refresh --profile dev --json --select operationCount --raw
igw api sync --profile dev --openapi-path /openapi.json --json
```

Generic call:

```bash
igw call \
  --gateway-url http://127.0.0.1:8088 \
  --api-key "$IGNITION_API_TOKEN" \
  --method GET \
  --path /data/api/v1/gateway-info

# Method defaults to GET when omitted.
igw call \
  --gateway-url http://127.0.0.1:8088 \
  --api-key "$IGNITION_API_TOKEN" \
  --path /data/api/v1/gateway-info
```

Call by operationId:

```bash
igw call \
  --gateway-url http://127.0.0.1:8088 \
  --api-key "$IGNITION_API_TOKEN" \
  --spec-file /path/to/openapi.json \
  --op gatewayInfo
```

Mutation safety + automation:

```bash
igw call --method POST --path /data/api/v1/scan/projects --yes
igw call --method GET --path /data/api/v1/gateway-info --retry 2 --retry-backoff 250ms
igw call --method GET --path /data/api/v1/gateway-info --out gateway-info.json
igw call --method GET --path /data/api/v1/gateway-info --json --out gateway-info.json --overwrite
igw call --method GET --path /data/api/v1/gateway-info --stream --max-body-bytes 1048576
igw call --batch @batch.ndjson --batch-output ndjson
igw call --batch @batch.json --batch-output json --parallel 4
igw call --method GET --path /data/api/v1/gateway-info --json --select response.status --raw
igw call --method GET --path /data/api/v1/gateway-info --json --select ok --select response.status --compact
igw call --method GET --path /data/api/v1/gateway-info --json --json-stats
```

Downloads with `--out` stream into a private temporary file and publish the
destination only after the complete response succeeds. Existing files require
`--overwrite`. This also applies to backup, logs, diagnostics, tag exports, and
gateway info. With `--json --out`, the response includes `artifact.path`,
`artifact.bytes`, and `artifact.sha256` alongside `bodyFile`; binary content is
kept out of the JSON body. Exceeding `--max-body-bytes` fails with exit code `7`
and does not publish a partial file. A stream sent directly to stdout may
already contain bytes when a transfer fails; check the exit code.

The legacy `call --dry-run` forwarding behavior has been removed: adding a
query parameter did not guarantee a safe preview. It now fails without sending
a request, including through batch/RPC execution. Use the development
entrypoint's genuine `api request --dry-run` or `api raw --dry-run` preview.

Config:

```bash
igw config set --gateway-url http://127.0.0.1:8088
igw config set --auto-gateway
igw config set --api-key-stdin < token.txt
igw config set --gateway-url http://127.0.0.1:8088 --json
igw config show
```

Profiles:

```bash
igw config profile add dev --gateway-url http://127.0.0.1:8088 --api-key-stdin --use
igw config profile add stage --gateway-url http://10.0.1.5:8088 --api-key-stdin
igw config profile add dev --gateway-url http://127.0.0.1:8088 --api-key-stdin --json
igw config profile list
igw config profile use stage
igw config profile use stage --json
```

Profile behavior:
- If there is no active profile yet, the first `config profile add` becomes active automatically.
- If `--profile` is omitted at runtime, the active profile is used when set.

Doctor:

```bash
igw doctor --gateway-url http://127.0.0.1:8088 --api-key "$IGNITION_API_TOKEN"
igw doctor --gateway-url http://127.0.0.1:8088 --api-key "$IGNITION_API_TOKEN" --json --select checks.0.name --raw
igw doctor --gateway-url http://127.0.0.1:8088 --api-key "$IGNITION_API_TOKEN" --json --select ok --select checks.0.name --compact
```

Convenience wrappers:

```bash
igw gateway info --profile dev --json
igw scan projects --profile dev --yes
igw scan config --profile dev --yes
```

Admin wrappers:

```bash
# Logs
igw logs list --profile dev --query limit=5 --json
igw logs download --profile dev --out gateway-logs.zip
# If --out is omitted, defaults to gateway-logs.zip.
igw logs loggers --profile dev --json
igw logs logger set --profile dev --name com.inductiveautomation --level DEBUG --yes --json
igw logs level-reset --profile dev --yes --json

# Diagnostics bundle
igw diagnostics bundle generate --profile dev --yes --json
igw diagnostics bundle status --profile dev --json
igw diagnostics bundle download --profile dev --out diagnostics.zip
# If --out is omitted, defaults to diagnostics.zip.

# Backups
igw backup export --profile dev --out gateway.gwbk
# If --out is omitted, defaults to gateway.gwbk.
igw backup restore --profile dev --in gateway.gwbk --yes --json

# Tags
igw tags export --profile dev --out tags.json
igw tags import --profile dev --in tags.json --yes --json
igw tags import --profile dev --in tags.json --collision-policy Overwrite --yes --json

# Restart
igw restart tasks --profile dev --json
igw restart gateway --profile dev --yes --json

# Wait / poll
igw wait gateway --profile dev --interval 2s --wait-timeout 2m
igw wait diagnostics-bundle --profile dev --interval 2s --wait-timeout 5m --json
igw wait restart-tasks --profile dev --interval 2s --wait-timeout 3m --json --select attempts --raw
```

Shell completion:

```bash
source <(igw completion bash)
```

Persistent RPC mode:

```bash
igw rpc --profile dev
igw rpc --profile dev --workers 4 --queue-size 128
printf '%s\n' \
  '{"id":"h1","op":"hello"}' \
  '{"id":"c1","op":"call","args":{"method":"GET","path":"/data/api/v1/gateway-info","timeout":"30s"}}' \
  '{"id":"x1","op":"cancel","args":{"id":"c1"}}' \
  '{"id":"cap1","op":"capability","args":{"name":"rpcWorkers"}}' \
  '{"id":"s1","op":"shutdown"}' | igw rpc --profile dev
```

Smoke test script:

```bash
IGW_PROFILE=dev ./scripts/smoke.sh
ITERATIONS=25 IGW_PROFILE=dev ./scripts/perf-baseline.sh
./scripts/perf-gate.sh
# Thresholds live in scripts/perf-thresholds.env.
```
