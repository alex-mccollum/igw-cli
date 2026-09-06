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
- On a shared Linux/WSL workstation, run these and other build/test scripts
  through `bash scripts/bounded-run.sh -- <command>`; see
  `docs/development-safety.md`. Run one validation job at a time.

## Workstation Safety
- Never automatically start, stop, restart, terminate, unregister, or repair WSL
  distributions or Docker Desktop. An unavailable engine is a blocker for live
  container checks, not permission to recover the host.
- Do not alter host memory settings, Windows services, registry, or Docker/WSL
  configuration as part of repository validation.
- Stop heavy work after a host/engine failure. Preserve the failed result and
  inspect logs read-only; do not retry the same load or raise limits automatically.
- Local Go builds, tests (including race tests), and captured-schema parsing must
  use the bounded runner. If its hard limits cannot be verified, stop that check
  or use an isolated CI runner. Do not fall back to an unbounded invocation.
- Do not overlap builds, tests, image pulls, or Gateway captures. The bounded
  runner limits Linux child processes; Docker workloads require separate limits.
- Live Gateway capture must use the guarded contributor binary. Before using
  a newly qualified image, run the opt-in lifecycle probe documented in
  `docs/catalog.md`. It verifies admission, kernel limits, lifetime termination,
  and cleanup. The 8.3.9 image passed that probe after the 2026-09-05 incident.
  Cleanup may target only the disposable container whose exact ID and ownership
  label were verified; leftover qualification containers block new captures.

## Delivery Rules
- Keep changes small and commit in logical slices.
- Leave Git authentication, pushes, and release publication to the user unless
  they explicitly delegate that specific action. Completing local work or
  preparing for a push is not authorization to publish or request credentials.
- Maintain stable exit codes for automation.
- Avoid secret leakage in logs and output.

## Recommended Script Usage (Situational)
- Script usage is recommended in these cases; it is not required for unrelated edits.
- If you change command docs or command shapes, run `./scripts/check-command-docs.sh` and `./scripts/lint-docs.sh`.
- If you change auth, network handling, exit codes, or machine-readable output behavior, run `go test ./...` and `./scripts/smoke.sh`.
- If you change release flow, packaging, or version metadata behavior, run `./scripts/release/dry-run.sh vX.Y.Z`.
- Maintainer-managed release and hook procedures are documented in `docs/releasing.md`; they are not automatic task-completion steps.

## Project Contracts
- Exit codes are part of the automation contract:
  - `0`: success (local operations or HTTP `2xx`)
  - `2`: usage/config errors
  - `6`: auth failures (`401`, `403`)
  - `7`: network/transport and non-auth HTTP failures
- Gateway mutations and profile edits require explicit `--yes` confirmation.
- Configuration precedence is strict: flags > environment > config file.
- Runtime environment variable names are stable: `IGNITION_GATEWAY_URL`, `IGNITION_API_TOKEN`.
- Command examples are canonical in `docs/commands.md`; keep `README.md` as onboarding and link back to docs.
- Release artifacts must satisfy the documented version and artifact naming contract in `docs/releasing.md`.

## See Also
- `docs/automation.md`
- `docs/releasing.md`
- `scripts/smoke.sh`
