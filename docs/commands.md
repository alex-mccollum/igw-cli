# Commands

## Command discovery

This file is the canonical command example reference for the v1 command tree
in `cmd/igw`. The source is still completing the gates in
`docs/rebuild-preview.md`; cutover does not imply a published v1 release.
Examples assume `igw` is on PATH; use `bin/igw` after a local build.

```bash
bash scripts/bounded-run.sh -- go build -o bin/igw ./cmd/igw
igw version
igw exit-codes --json
igw schema --json
igw schema resource update --json
igw profile show --json
igw spec sync --json
igw api list --search gateway --json
igw api describe 'GET /data/api/v1/gateway-info' --json
igw api request 'GET /data/api/v1/gateway-info' --json
igw gateway doctor --json
igw gateway restart-tasks --json
igw gateway restart --dry-run --json
igw gateway restart --yes --timeout 3m --json
igw api raw --method POST --path /data/api/v1/scan/projects --dry-run --json
igw spec export --out gateway-openapi.json --json
igw spec inspect gateway-openapi.json --json
igw spec inspect gateway-openapi.json --summary --json
igw spec import gateway-openapi.json --json
igw api list --offline --json
igw spec diff before-openapi.json after-openapi.json --json
```

Configure a profile using explicit stored values. These commands are local and
can run without a Gateway connection:

```bash
igw profile set dev --url http://localhost:8088 --use --dry-run --json
igw profile set dev --url http://localhost:8088 --use --yes --json
igw profile set dev --token-stdin --yes --json < private-token.txt
igw profile list --json
igw profile show --profile dev --json
igw profile set dev --clear-token --if-revision "$CONFIG_REVISION" --dry-run --json
igw profile set dev --clear-token --if-revision "$CONFIG_REVISION" --yes --json
igw profile use --default --yes --json
igw profile remove dev --yes --json
```

Use `--url` to store a target; runtime `--gateway-url`, `--profile`, and
environment values are not saved. Token input is bounded to 16 KiB and never
printed. `--if-revision` uses `data.revision` from `profile list`, or
`data.before.revision` from an edit preview. Each successful write produces a
new revision. Every edit requires exactly one of `--dry-run` or `--yes`.
An active profile must be deselected before removal.

Existing users explicitly migrate the legacy settings before editing them:

```bash
igw profile migrate --dry-run --json
igw profile migrate --yes --json
igw profile list --json
igw profile rollback --if-revision "$CONFIG_REVISION" --dry-run --json
igw profile rollback --if-revision "$CONFIG_REVISION" --yes --json
```

Migration preserves `config.json` and activates a separate `config.v1.json`.
Rollback requires the unchanged legacy file and archives the current v1 bytes
before returning to it. Migration reads current legacy settings and has no v1
revision precondition. See `docs/profiles.md` for exact precedence, strict file
validation, storage permissions, concurrent writer behavior, and recovery limits.

`gateway restart` is a full Gateway restart and interrupts its running services.
Use a direct URL for the intended Gateway node. A preview reads node identity,
process ID, uptime, and pending tasks, then prepares `POST
/data/api/v1/restart-tasks/restart?confirm=true` without sending it. An actual
restart requires `--yes` and refreshes the catalog before its baseline reads.
`api capabilities` checks the four required routes against the selected catalog;
advertised routes do not establish permission or qualify a Gateway's responses.

The workflow sends one POST and polls only reads under the total `--timeout`
budget (30 seconds by default). Use `--interval` to set polling between 100ms
and 1m; the default is 1s. `--offline` is unavailable because baseline and
verification require current state. `--yes` and `--dry-run` are mutually
exclusive. Malformed or missing baseline identity/process/task fields prevent
the restart, including when the vendor response schema omits required fields.

Completion requires the same observed `redundancy.localId`, either a changed
`overview.processId` or a decreased `overview.uptime`, and no pending restart
tasks. Uptime is reported in the API's units, which the captured document does
not specify; the CLI does not infer seconds or elapsed wall time. `processId`
is not assumed to be a JVM PID: both qualified images reported the
`ignition-gateway` wrapper PID, which stayed unchanged while Java restarted.
`localId` is read before and after each overview/task observation. JSON evidence
records
`before`, `last`, `polls`, `acknowledged`, `proof`, and `correlation`.
`correlation: "selected_target"` limits the claim to the addressed target.
`meta.verification: "restart_observed"` means these checks passed. A reachable
API or an empty task list alone is insufficient.

The qualified disposable Gateways returned identical `localId` values across
separate containers. A matching value is a consistency check, not proof of
unique physical node identity. There is no restart job ID or atomic node
precondition. Separate reads cannot
guarantee affinity through a load balancer, establish exclusive causality, or
prove every module's health. An observed node change stops verification.
After a disconnect or server error on the POST, the CLI may continue read-only
verification; it never replays the restart. Successful observation can therefore
have `acknowledged: false`. Auth failures, invalid observations, cancellation,
or a deadline after dispatch produce `uncertain` with the normal error exit
code (auth 6, other failures 7). Inspect current state before considering another
restart. Preview and local usage errors remain non-mutating. Both core image
versions passed live restart qualification; see `docs/compatibility-matrix.md`.

Qualified API references are available without Gateway configuration, credentials,
network access, or a populated cache:

```bash
igw spec references list --json
igw spec references inspect ignition-8.3.9-defaults --json
igw spec references export ignition-8.3.9-defaults --out ./reference-8.3.9 --json
igw api list --reference ignition-8.3.9-defaults --search gateway --json
igw api describe 'GET /data/api/v1/gateway-info' --reference ./reference-8.3.9 --json
igw api capabilities --reference ignition-8.3.9-defaults --json
igw api capabilities --reference ignition-8.3.0-core --json
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

Path arrays/objects using the parameter's `schema`/`style` strategy, and
label/matrix styles, currently fail
schema-assisted validation as `unsupported_serialization`; their contracts
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

`--query key=value` separates on the first `=`. Names and values are literal:
whitespace is preserved, percent escapes are not decoded, and the CLI performs
URL encoding. An empty name fails with exit 2 before discovery or dispatch;
an empty value remains present. Malformed encoded queries in legacy request
paths fail rather than silently discarding part of the input. Errors do not
echo the supplied query text.

For named query parameters, supply one value for a primitive, or repeat the
same key for each item in an exploded form array. A comma inside an array item
remains part of that item. For example, this previews two session IDs:

```bash
igw api request 'DELETE /data/perspective/api/v1/sessions' --query sessionId=SESSION_1 --query sessionId=SESSION_2 --dry-run --json
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

Where a query or path parameter declares `content`, supply one value in its
declared JSON or UTF-8 `text/plain` representation. For example, the argument
`--query 'options={"enabled":false}'` supplies a JSON object only if that
parameter declares JSON content; `--path-param 'value={"id":9007199254740993}'`
uses the same rule for a path parameter. The CLI handles URL encoding and
validates the exact decoded value without rewriting the supplied text. This
does not reinterpret ordinary string parameters as JSON.

Content parameters support nested objects, arrays, references, explicit JSON
null when allowed, and exact numbers. JSON `""` is distinct from omitted input;
empty text query values are present and must satisfy their schema. Empty path
segments remain invalid. Repeated content query parameters, duplicate JSON
keys, malformed Unicode, and trailing JSON fail before dispatch. Each content
value is limited to 32 MiB before decoding, with the JSON numeric/nesting limits
described below. Unsupported media fail when supplied; an absent optional
content query parameter needs no decoder. An operation's parameter declaration
overrides the same inherited name/location. Named content keys cannot overlap
the Gateway's exploded filter property names.

The retained Gateway captures qualify named primitive and exploded primitive-
array query shapes. Their documents contain no query/path `content` parameters;
the added encoding support is verified against HTTP fixtures, not a new live
Gateway claim. Other query styles and nested `schema` serialization still need
explicit support; `api raw` remains available. Multipart input has a separate
contract and remaining implementation work.

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
`not_requested`. Multipart schema decoding and additional schema encodings
remain unfinished.

For an operation that declares `application/x-www-form-urlencoded`, use
`--urlencoded 'name=value'` on `api request` or `api raw`. Repeat it to provide
more fields or multiple values under the same name. The name must be nonempty;
`--urlencoded 'name='` supplies an empty string. Everything after the first `=`
is literal UTF-8 text: the CLI escapes it once, preserves repeated-value order,
and sorts field names. A literal `%2F` becomes `%252F`, `+` becomes `%2B`, and
spaces become `+`. The preview digest covers the exact transmitted bytes.
The limit is 4096 fields and 32 MiB after encoding. This option owns the content
type and cannot be combined with `--body`, `--upload`, `--content-type`, or
multipart options. `--form-field` continues to construct multipart text parts.

Schema-assisted form requests decode the wire bytes and validate the complete
object, including required properties, references, root assertions, and array
constraints. Supported bindings are directly declared properties of an object:

- Primitive properties use their exact string, integer, number, or Boolean
  spelling. No value is implicitly converted to null.
- Object properties default to one JSON value, supplied as literal JSON text.
  An explicit JSON `encoding.contentType` also supports arrays and other JSON
  values; their complete schema and the existing JSON limits apply.
- Explicit `form` style with exploded primitive arrays uses repeated fields.
  Commas inside a value stay literal. Explicit `style`, `explode`, or
  `allowReserved: false` selects the style strategy and ignores `contentType`.
- Other supplied bindings, implicit array encodings, packed arrays, exploded
  objects, reserved expansion, binary content encodings, and unsupported media fail
  locally. Absent optional fields require no decoder. Unknown field names are
  refused because their serialization cannot be established from the schema.

These rules follow the clarification in
[OpenAPI 3.0.4](https://spec.openapis.org/oas/v3.0.4.html#encoding-object) and
[OpenAPI 3.1.1](https://spec.openapis.org/oas/v3.1.1.html#encoding-object).
The retained Gateway captures have no URL-encoded form declarations; this
support has synthetic-contract and HTTP-fixture evidence. The seven multipart
routes in the 8.3.9 default capture are a separate existing body contract.
To supply a pre-encoded form or an explicit empty form, use
`--body @form.txt --content-type application/x-www-form-urlencoded` or
`--body '' --content-type application/x-www-form-urlencoded`. These bytes go
through the same schema decoder. The decoder also accepts `charset=utf-8` or
`charset=us-ascii`; ASCII requires ASCII field names and values after decoding.

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
igw api request 'POST /data/api/v1/projects/import/{name}' --path-param name=Example --upload project.zip --content-type application/zip --dry-run --json
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
igw api request "$operation" --multipart @parts.json --dry-run --json
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

The pinned 8.3.9 core-profile qualification verified repeated `files` parts with
explicit filenames on the translations bulk-datafile route. For an existing
core translations resource, obtain its current signature with `api request`
against `GET /data/api/v1/resources/singleton/ignition/translations`, using
`--query collection=core`. Set `signature` to the returned resource signature,
then prepare a files-only manifest such as:

```json
[
  {"name": "files", "file": "./payload.bin", "filename": "payload.bin", "contentType": "application/octet-stream"}
]
```

Save it as `files.json` and review the request:

```bash
igw api request 'PUT /data/api/v1/resources/datafile/ignition/translations' --query collection=core --query "signature=$signature" --multipart @files.json --dry-run --json
```

Replace `--dry-run` with `--yes` to execute the reviewed inputs. Read back each
file using the corresponding single-datafile GET route to verify its bytes.
`files` is a tested working name, not a vendor-declared exclusive name; this
recipe does not establish behavior for every bulk-datafile route. The tested
8.3.0 catalog lacks the bulk route. See the
[body-input qualification](compatibility-matrix.md#additional-request-body-qualification)
for exact versions and evidence.

For independent small requests, `api batch` accepts a JSON array through
`--input` as literal JSON, `@file`, or `-` for stdin. A manifest can mix reads
and writes against the selected Gateway:

```json
[
  {"id": "gateway", "operation": "GET /data/api/v1/gateway-info"},
  {"id": "encrypt", "operation": "POST /data/api/v1/encryption/encrypt", "bodyText": "Example", "contentType": "text/plain"}
]
```

Save it as `batch.json`, preview every item, then execute deliberately:

```bash
igw api batch --input @batch.json --dry-run --json
igw api batch --input @batch.json --yes --json
```

Each item requires a unique `id` (1..64 ASCII letters, digits, dots, underscores,
or hyphens) and an `operation` key or unambiguous alias. Optional `pathParams`
maps names to strings; `query` and `headers` map names to arrays of strings.
Repeated values retain order, `[""]` sends an empty value, and `[]` omits that
key. `body` contains the literal JSON value to transmit, preserving exact numbers
and JSON `null`; `bodyText` supplies literal UTF-8, including an empty string.
Choose at most one. Text is not a file selector. `contentType` defaults to
`application/json` when a body is supplied; specify `text/plain` for plain text.
The offline command schema includes the complete manifest shape.

The complete manifest is limited to 1 MiB and 100 items. A separate 1 MiB input
budget counts values and every repeated query/header name before URL encoding;
a compact array cannot bypass it. Malformed JSON,
duplicate keys or IDs, unknown fields, and invalid input types fail before
Gateway access. Each response is capped at 256 KiB. Use single-request commands
for raw HTTP, file uploads, multipart input, and streamed artifacts. Batch items
cannot override the invocation's target, credentials, confirmation, or catalog
policy. A batch containing an advertised mutation requires `--yes` before any
operation runs; `--yes` and `--dry-run` are mutually exclusive. Read-only batches
need no confirmation. Offline batches require `--dry-run` and a cached catalog.

Items run in order using one catalog and one discovery/execution deadline.
Confirmed batches revalidate that catalog within the invocation. Previews send
zero operation requests. Operation-specific validation occurs when each item
is reached, so a later invalid item preserves earlier results. By default, the
first failure stops execution. `--continue-on-error` attempts subsequent
independent items after ordinary validation or HTTP failures. Uncertain writes,
authentication failures, and cancellation always stop. Batches provide no
rollback, cross-item value substitution, automatic mutation replay, or guarantee
that a generic accepted mutation reached its desired state.

One `igw/v1` result contains ordered `data.items`, each with `id` and its full
`result`, plus `succeeded`, `failed`, and `notRun` counts. Unattempted items have
`ok: false`, `outcome: "not_run"`, and no fabricated error or data. Successful
previews report `preview`; a successful batch containing accepted operations
reports `accepted`. Mixed success/failure reports `partial`, and any uncertain
write reports `uncertain`. The exit code is the first ordinary failure's code,
unless a terminal authentication/cancellation/uncertainty failure takes
precedence. Human output lists every ID and outcome, including on failure;
use `--json` to retain full response data and evidence. Review per-item results
before retrying any part of a failed batch.

Named resource workflows discover their routes from the selected Gateway's
catalog and verify successful changes with an independent read:

```bash
igw resource types --offline --json
igw resource describe ignition/schedule --json
igw resource list ignition/schedule --limit 50 --offset 0 --json
igw resource list ignition/schedule --filter 'name[eq]=Example' --json
igw resource get ignition/schedule Example --collection core --json
igw resource create ignition/schedule Example --body @schedule-fields.json --dry-run --json
igw resource create ignition/schedule Example --body @schedule-fields.json --yes --json
igw resource update ignition/schedule Example --body '{"description":"Day shift"}' --dry-run --json
igw resource update ignition/schedule Example --body '{"description":"Day shift"}' --if-signature REVIEWED_SIGNATURE --yes --json
igw resource delete ignition/schedule Example --dry-run --json
igw resource delete ignition/schedule Example --if-signature REVIEWED_SIGNATURE --yes --json
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
Human previews show the target, changed fields, request digest, and reviewed
signature. Apply that signature with `--if-signature` for update/delete after
reviewing the proposed body. Human failures also retain workflow state and
available verification checks; JSON keeps the complete `igw/v1` result.

Successful workflows report `outcome: "completed"` and
`meta.verification: "verified"` after response/state checks. This verifies stored
configuration, not operational health of the configured connection or device.
Verification compares submitted values, permits additional Gateway defaults,
and checks that omitted top-level writable fields remain unchanged. It does not
prove removal of unspecified nested properties.
If acknowledgement, signature, or readback cannot establish the outcome,
the result stays failed or uncertain. After a mutation attempt, `data.checks`
reports acknowledgement, matching change count, and readback validity. When
available, it also reports signature and field comparisons, with sorted
`mismatchedFields` names. Comparison fields are omitted when unavailable or
inapplicable. These diagnostics contain no configuration values and do not
authorize an automatic retry. Redacted secrets or vendor-normalized
values that cannot be compared can prevent verification; inspect current state
before retrying. Forced reference changes still require explicit generic API
requests.

Singletons use the same commands with `TYPE` alone. `resource types` reports
`singleton: true` when the catalog advertises that type's singleton read route;
each requested mutation must also exist in the current catalog. For example:

```bash
igw resource get ignition/translations --json
igw resource update ignition/translations --body '{"description":"Shared translations"}' --dry-run --json
igw resource update ignition/translations --body '{"description":"Shared translations"}' --if-signature REVIEWED_SIGNATURE --yes --json
igw resource delete ignition/translations --dry-run --json
igw resource delete ignition/translations --if-signature REVIEWED_SIGNATURE --yes --json
igw resource create ignition/translations --body @translation-fields.json --dry-run --json
igw resource create ignition/translations --body @translation-fields.json --yes --json
```

Omitting `NAME` selects the singleton route explicitly. Supplying `NAME` selects
the named-resource route; neither mode falls back to the other. Singleton
identity is type plus collection. The CLI omits name fields from mutations and
uses the signature-only delete path. Singleton reads explicitly disable
`defaultIfUndefined`, so a default configuration cannot count as an existing
stored definition. Update/delete keep the same reviewed-signature requirement,
single-mutation behavior, and independent readback checks as named resources.
Create requires observed absence. Deletion verifies that the stored definition
is absent; the Gateway may still use built-in defaults at runtime.

The fixture checks cover all 17 singleton types in each retained default
8.3.0/8.3.9 contract, plus actual CLI HTTP fixtures. A disposable-Gateway harness
exercises translations update, stale review, deletion, recreation, and duplicate
creation refusal. Its first 8.3.0 run verified update, stale-review refusal, and
deletion, but recreation returned HTTP 200 with an unverified outcome. The
[original failed attempt](../internal/testgateway/testdata/singleton/attempts/README.md)
is retained. Full singleton qualification remains pending.
The subsequent diagnostic run on 8.3.0 matched acknowledgement, signature,
description, and enabled state, but omitted `config` from both readbacks even
though the schema advertises it. Such a result remains `uncertain`/exit 7;
metadata success cannot establish translation configuration state. The harness
keeps that exact configuration request and separately tests metadata-only
creation. This observed limitation does not establish behavior for other types
or Gateway versions.

Project workflows transfer a complete project ZIP, inspect its file manifest,
and verify the imported contents through a fresh export:

```bash
igw project list --limit 50 --offset 0 --json
igw project get Example --json
igw project export Example --out project.zip --json
igw project inspect project.zip --json
igw project import Copy --in project.zip --dry-run --json
igw project import Copy --in project.zip --yes --json
igw project import Copy --in project.zip --overwrite --dry-run --json
igw project import Copy --in project.zip --overwrite --if-project-sha256 REVIEWED_DIGEST --yes --json
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
igw api capabilities --json
igw tag export --provider default --path Example --out tags.json --json
igw tag import --provider default --in tags.json --dry-run --json
igw tag import --provider default --in tags.json --yes --json
igw tag import --provider default --path Destination --in tags.json --collision-policy Overwrite --dry-run --json
igw tag import --provider default --path Destination --in tags.json --collision-policy Overwrite --yes --json
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
igw backup export --out gateway.gwbk --json
igw logs list --min-level WARN --since 1h
igw logs list --limit 50 --min-level WARN --since 1h --json
igw logs list --logger Gateway --search timeout --since 1h --json
igw logs list --logger Gateway --since 2026-09-05T08:00:00-07:00 --until 2026-09-05T09:00:00-07:00 --json
igw logs list --since 2026-09-05T08:00:00-07:00 --until 2026-09-05T09:00:00-07:00 --limit 50
igw logs download --out system-logs.idb --json
igw diagnostics bundle status --json
igw diagnostics bundle collect --out diagnostics.zip --dry-run --json
igw diagnostics bundle collect --out diagnostics.zip --yes --timeout 2m --json
igw diagnostics bundle download --out diagnostics-latest.zip --json
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
Human output renders epoch-millisecond event timestamps in UTC, followed by
severity, logger, message, and available stack context. Context and unfamiliar
fields are preserved; an unrecognized response shape falls back to complete
JSON. Terminal controls in formatted fields are escaped. `--json` preserves the
original structured response, including pagination metadata and event fields.
The human footer shows page counts and the next offset when available. Keep
the same filters and a fixed time window while paging; offset pagination is
not an immutable snapshot. An empty page is distinct from a failed request.

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
