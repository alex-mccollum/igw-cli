# AGENTS.md

Project-local operating notes for `igw-cli`.

## Project Scope
- Build a thin, reliable CLI wrapper for the Ignition Gateway HTTP API.
- Keep the command surface focused on high-value operational workflows; prefer depth and consistency over breadth.
- Preserve strong automation ergonomics: deterministic behavior, stable machine-readable output, and predictable error signaling.
- Default to Go standard library dependencies; add third-party packages only when they provide clear, durable net value.

## Canonical Commands
- `go test ./...`
- `go build ./cmd/igw`

## Delivery Rules
- Keep changes small and commit in logical slices.
- Maintain stable exit codes for automation.
- Avoid secret leakage in logs and output.

## Project Contracts
- Exit codes are part of the automation contract:
  - `0`: success (`2xx`)
  - `2`: usage/config errors
  - `6`: auth failures (`401`, `403`)
  - `7`: network/transport and non-auth HTTP failures
- Mutating operations require explicit `--yes` confirmation.
- Configuration precedence is strict: flags > environment > config file.
- Runtime environment variable names are stable: `IGNITION_GATEWAY_URL`, `IGNITION_API_TOKEN`.
- Command examples are canonical in `docs/commands.md`; keep `README.md` as onboarding and link back to docs.
- Release artifacts must satisfy the documented version and artifact naming contract in `docs/releasing.md`.
