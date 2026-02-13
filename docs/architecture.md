# Architecture

## Goal
A thin CLI wrapper around the Ignition Gateway HTTP API.

## MVP Commands
1. `call`
2. `config set|show`
3. `doctor`

## Contracts
- Auth header: `X-Ignition-API-Token`.
- Exit codes:
  - `0`: success (`2xx`)
  - `2`: usage/config errors
  - `6`: auth failures (`401`, `403`)
  - `7`: network/transport and non-auth HTTP failures
- Config precedence: flags > env > config file.

## Dependency Policy
MVP uses Go standard library only.
