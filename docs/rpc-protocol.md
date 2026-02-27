# RPC Protocol Contract

`igw rpc` exposes a newline-delimited JSON (NDJSON) request/response stream for host applications that need many calls in a single process.

## Transport

- Input: one JSON object per line on `stdin`.
- Output: one JSON object per line on `stdout`.
- Request order is accepted serially; response order may differ when `--workers > 1`.
- Empty input lines are ignored.

## Request Envelope

```json
{"id":"req-1","op":"hello","args":{"name":"rpcWorkers"}}
```

- `id` is optional and echoed back when provided.
- `op` is required.
- `args` is optional and operation-specific.

## Response Envelope

```json
{"id":"req-1","ok":true,"code":0,"status":200,"data":{"...": "..."}}
```

- `ok`: operation success.
- `code`: CLI contract exit code class (`0`, `2`, `6`, `7`).
- `status`: optional HTTP status for API-backed operations.
- `data`: operation payload.
- `error`: present when `ok=false`.

## Built-In Operations

- `hello`: protocol/version/features handshake.
- `capability`: feature query (`args.name` optional).
- `call`: execute one API call (same core behavior as `igw call` and `igw call --batch`).
- `reload_config`: clear runtime caches for config/spec resolution.
- `shutdown`: acknowledge and stop reading further input.

## Handshake Contract

`hello` returns:

- `protocol`: stable protocol family (`igw-rpc-v1`).
- `protocolSemver`: RPC schema version (`MAJOR.MINOR.PATCH`).
- `minHostSemver`: minimum host compatibility floor.
- `version`: CLI build version.
- `features`: capability map for additive feature detection.
- `ops`: operation list for quick probing.

Hosts should:

1. Verify `protocol` is recognized.
2. Parse `protocolSemver` and reject incompatible future major versions.
3. Gate optional behavior via `features` or `capability`.

## Compatibility Rules

- Breaking wire changes require a new protocol family (`protocol`) and major semver bump.
- Additive fields and operations are allowed in minor/patch versions.
- Existing fields (`id`, `ok`, `code`, `status`, `data`, `error`) are stable.
- Unknown fields must be ignored by hosts.

## Load Governance

`rpc` supports bounded execution controls:

- `--workers`: concurrent request workers (`>=1`).
- `--queue-size`: bounded in-memory queue capacity (`>=1`).

These controls provide predictable throughput and memory bounds for high-frequency hosts.
