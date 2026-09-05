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

`api request` accepts either an exact `METHOD /path` key or an unambiguous
operationId. Use repeatable `--path-param name=value`, `--query key=value`,
and `--header name:value` for parameters and `--body @file.json` for input.
Preview mutations with `--dry-run`; execution requires `--yes`. A preview
may fetch the API document but never sends the proposed request.

For an opaque binary body, use `--upload FILE --content-type MEDIA_TYPE`.
The input must be a regular file; the CLI creates a private disk snapshot so
preview metadata and transmission use the same bytes within an invocation.
`--max-upload-bytes` defaults to 1 GiB. The snapshot is removed on completion.
`--body` and `--upload` are mutually exclusive. Schema-assisted streaming is
available when the declared media type has no body schema; its validation
coverage is `declared_transport`, not validation of archive or tag contents.
Use bounded `--body` for a schema-bearing request, or `api raw` explicitly.

```bash
bin/igw-next api request 'POST /data/api/v1/projects/import/{name}' --path-param name=Example --upload project.zip --content-type application/zip --dry-run --json
```

Named resource workflows discover their routes from the selected Gateway's
catalog and verify successful changes with an independent read:

```bash
bin/igw-next resource types --offline --json
bin/igw-next resource describe ignition/schedule --json
bin/igw-next resource list ignition/schedule --limit 50 --offset 0 --json
bin/igw-next resource get ignition/schedule Example --collection core --json
bin/igw-next resource create ignition/schedule Example --body @schedule-fields.json --dry-run --json
bin/igw-next resource create ignition/schedule Example --body @schedule-fields.json --yes --json
bin/igw-next resource update ignition/schedule Example --body '{"description":"Day shift"}' --dry-run --json
bin/igw-next resource update ignition/schedule Example --body '{"description":"Day shift"}' --if-signature REVIEWED_SIGNATURE --yes --json
bin/igw-next resource delete ignition/schedule Example --dry-run --json
bin/igw-next resource delete ignition/schedule Example --if-signature REVIEWED_SIGNATURE --yes --json
```

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
bin/igw-next tag export --provider default --path Example --out tags.json --json
bin/igw-next tag import --provider default --in tags.json --dry-run --json
bin/igw-next tag import --provider default --in tags.json --yes --json
bin/igw-next tag import --provider default --path Destination --in tags.json --collision-policy Overwrite --dry-run --json
bin/igw-next tag import --provider default --path Destination --in tags.json --collision-policy Overwrite --yes --json
```

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
