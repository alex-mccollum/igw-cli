# Architecture

## Goal
A thin CLI wrapper around the Ignition Gateway HTTP API.

## Core Commands
1. `api list|show|search|tags|stats`
2. `api sync|refresh`
3. `call`
4. `config set|show|profile`
5. `doctor`
6. `gateway info`
7. `scan <projects|config>`
8. `logs <list|download|loggers|logger set|level-reset>`
9. `diagnostics bundle <generate|status|download>`
10. `backup <export|restore>`
11. `tags <export|import>`
12. `restart <tasks|gateway>`
13. `wait <gateway|diagnostics-bundle|restart-tasks>`
14. `exit-codes`
15. `schema`

## Contracts
- Auth header: `X-Ignition-API-Token`.
- Exit codes:
  - `0`: success (`2xx`)
  - `2`: usage/config errors
  - `6`: auth failures (`401`, `403`)
  - `7`: network/transport and non-auth HTTP failures
- Config precedence: flags > env > config file.
- Config supports WSL host auto-detection via `config set --auto-gateway`.
- Profiles supported for multi-gateway workflows (`config profile add|use|list`, runtime `--profile`).
- Mutating calls require explicit `--yes`.
- `doctor` is read-only by default; `--check-write` enables write permission checks.
- `call` supports optional retries for idempotent methods and `--out` file output.
- `completion bash` outputs profile-aware shell completion.
- Wrapper commands delegate to `call` so they share auth/config/timeout/JSON/exit behavior.

## Dependency Policy
Default to Go standard library dependencies; add third-party packages only when they provide clear, durable value.

## OpenAPI Discovery Model
- Query a committed or local OpenAPI JSON snapshot.
- Prefer local cached OpenAPI whenever available.
- Keep `call` generic and use `api` commands for endpoint lookup.
- Default spec lookup: `openapi.json` in CWD, then `${XDG_CONFIG_HOME:-~/.config}/igw/openapi.json`.
- If default spec files are missing, auto-sync OpenAPI from the gateway and cache to config dir.
- `call --op <operationId>` resolves method/path from local spec for ergonomic calls.
- Convenience wrappers (`gateway info`, `scan projects`, `scan config`) delegate to the same core `call` behavior.
