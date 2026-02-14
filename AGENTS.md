# AGENTS.md

Project-local operating notes for `igw-cli`.

## Project Scope
- Build a lightweight Ignition Gateway API CLI in Go.
- Default to Go standard library dependencies; add third-party packages only when they provide clear, durable value.
- Prioritize stable machine-friendly behavior over broad command surface.

## Canonical Commands
- `go test ./...`
- `go build ./cmd/igw`

## Delivery Rules
- Keep changes small and commit in logical slices.
- Maintain stable exit codes for automation.
- Avoid secret leakage in logs and output.
