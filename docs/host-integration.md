# Host Integration Contract

This guide defines the recommended contract for host applications that call `igw` as an external tool.

## Goals

- Deterministic startup checks.
- Stable machine-readable behavior.
- Predictable fallback when persistent RPC is unavailable.

## Install/Update Channels

- Production/stable hosts: pin explicit versions (`vMAJOR.MINOR.PATCH`).
- Fast-moving/dev hosts: use `latest` alias artifacts from:
  - `https://github.com/<owner>/<repo>/releases/latest/download/igw_<os>_<arch>.<ext>`

Recommended bootstrap verification:

1. Download artifact + `checksums.txt`.
2. Verify SHA-256.
3. Run `igw version` and require success.

## Startup Handshake

On process startup, hosts should run an RPC handshake before workload requests:

1. Send `hello`.
2. Verify `protocol == "igw-rpc-v1"`.
3. Parse `protocolSemver`; reject unsupported future major versions.
4. Check required features with `features` or `capability`.

Minimum recommended feature checks:

- `call`
- `rpcWorkers`
- `rpcQueueSize`
- `callStatsV1`

## Runtime Strategy

1. Prefer `igw rpc` for repeated requests.
2. Configure bounded controls:
   - `--workers` for concurrency.
   - `--queue-size` for backpressure.
3. Use one-shot fallback (`igw call --json`) when RPC startup/handshake fails.

## Machine Contracts

Hosts should treat these as stable automation contracts:

- Exit code classes:
  - `0` success
  - `2` usage/config
  - `6` auth
  - `7` network/non-auth HTTP
- JSON stats schema:
  - `stats.version == 1`
  - `stats.timingMs`
  - `stats.bodyBytes`
- RPC queue telemetry for `call`:
  - `stats.rpc.queueWaitMs`
  - `stats.rpc.queueDepth`

## Recommended Session Pattern

1. Open `igw rpc` process.
2. Handshake (`hello`, optional `capability` checks).
3. Execute calls.
4. Optionally issue `cancel` for in-flight request IDs.
5. Send `shutdown` and close stdin.

If the stream fails, restart a fresh RPC session and re-run handshake.

## Compatibility Fallback

If handshake or capability checks fail:

1. Mark RPC unavailable for the current run.
2. Fall back to one-shot `igw call --json`.
3. Preserve the same output/exit-code handling path in host code.
