# Architecture

## Goal
A thin CLI wrapper around the Ignition Gateway HTTP API.

## MVP Commands
1. `api list|show|search`
2. `call`
3. `config set|show`
4. `doctor`

## Contracts
- Auth header: `X-Ignition-API-Token`.
- Exit codes:
  - `0`: success (`2xx`)
  - `2`: usage/config errors
  - `6`: auth failures (`401`, `403`)
  - `7`: network/transport and non-auth HTTP failures
- Config precedence: flags > env > config file.
- Config supports WSL host auto-detection via `config set --auto-gateway`.

## Dependency Policy
MVP uses Go standard library only.

## OpenAPI Discovery Model
- Query a committed or local OpenAPI JSON snapshot.
- Do not depend on runtime `/openapi` availability.
- Keep `call` generic and use `api` commands for endpoint lookup.
- `call --op <operationId>` resolves method/path from local spec for ergonomic calls.
