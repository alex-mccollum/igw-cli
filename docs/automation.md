# Automation Patterns

This guide is for scripts, CI jobs, and coding agents.

## Core Contract

- Prefer `--json` whenever supported.
- Use exit codes for control flow:
  - `0`: success
  - `2`: usage/config error
  - `6`: auth failure (`401`, `403`)
  - `7`: network/transport or non-auth HTTP failure

## Common Flow

1. Configure or select runtime context.
2. Run read-only health checks.
3. Optionally run write-permission checks.
4. Execute API calls.
5. Write artifacts to files when needed.

## Host-App Bootstrap Contract

For applications that call `igw` as an external tool:

1. Pin a release tag (`vMAJOR.MINOR.PATCH`).
2. Resolve/download the matching artifact for OS/arch from GitHub Releases.
3. Verify archive SHA-256 with `checksums.txt` (or the `sha256` value in `release-manifest.json`).
4. Install `igw` in an app-managed bin directory.
5. Run `igw version` and require success before enabling gateway-backed features.
6. Probe capabilities with `igw api list --json` if your app gates behavior on available operations.
7. If using persistent mode, start `igw rpc` and run a `hello`/`capability` handshake before sending workload requests.

## Recommended Commands

Config and profiles:

```bash
igw config set --gateway-url http://127.0.0.1:8088 --json
igw config profile add dev --gateway-url http://127.0.0.1:8088 --api-key-stdin --json < token.txt
igw config profile use dev --json
```

Connectivity and auth:

```bash
igw doctor --json
igw doctor --check-write --json
```

API execution:

```bash
igw api sync --json
igw call --path /data/api/v1/gateway-info --json
igw api capability file-write --json
igw call --method POST --path /data/api/v1/scan/projects --yes --json
igw call --batch @batch.ndjson --batch-output ndjson
igw scan config --yes --json
```

Single-value extraction (for shell variables or quick checks):

```bash
igw call --path /data/api/v1/gateway-info --json --select response.status --raw
igw doctor --json --select checks.2.ok --raw
```

Subset extraction and compact JSON:

```bash
igw call --path /data/api/v1/gateway-info --json --select ok --select response.status
igw doctor --json --select ok --select checks.0.name --compact
```

Artifacts:

```bash
igw logs download --out gateway-logs.zip --json
igw diagnostics bundle download --out diagnostics.zip --json
igw backup export --out gateway.gwbk --json
```

Persistent machine mode:

```bash
igw rpc --profile dev
```

Persistent machine mode with handshake:

```bash
printf '%s\n' \
  '{"id":"h1","op":"hello"}' \
  '{"id":"cap1","op":"capability","args":{"name":"rpcWorkers"}}' \
  '{"id":"s1","op":"shutdown"}' | igw rpc --profile dev
```

Operational wait checks:

```bash
igw wait gateway --interval 2s --wait-timeout 2m --json
igw wait diagnostics-bundle --interval 2s --wait-timeout 5m --json
igw wait restart-tasks --interval 2s --wait-timeout 3m --json --select attempts --raw
```

## Notes

- `doctor` is read-only by default; add `--check-write` for write checks.
- `call` defaults `--method` to `GET` when `--path` is provided.
- `call --stream` can reduce memory overhead for large payload workflows.
- `call --batch` can reduce process startup/flag parsing overhead for many independent requests.
- `rpc` should be preferred for high-frequency host integrations because it amortizes process startup and supports bounded worker/queue controls.
- `--select` requires `--json`; dot paths support objects and array indexes (`checks.0.name`).
- Repeat `--select` for multiple selections.
- `--raw` requires exactly one `--select`.
- `--compact` requires `--json` and removes pretty indentation.
- `--timing` and `--json-stats` expose latency/runtime stats for automation diagnostics.
- API discovery defaults to `openapi.json` in CWD, then `${XDG_CONFIG_HOME:-~/.config}/igw/openapi.json`.
- If no default spec is present, `api` and `call --op` auto-sync and cache OpenAPI from the gateway.
- If you omit `--profile`, the active profile is used (when set).
