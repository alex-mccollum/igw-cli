# AGENTS.md

Project-local operating notes for `igw-cli`.

## External Reference
Primary reference: `../agent-playground-rules/docs/INDEX.md`

Read before major work:
- `../agent-playground-rules/docs/principles.md`
- `../agent-playground-rules/docs/workflow.md`
- `../agent-playground-rules/docs/playbooks/feature-delivery-playbook.md`
- `../agent-playground-rules/docs/playbooks/update-agent-files-playbook.md`
- `../agent-playground-rules/docs/playbooks/promote-learnings-playbook.md`

## Project Scope
- Build a lightweight Ignition Gateway API CLI in Go.
- Keep dependencies to Go standard library for MVP.
- Prioritize stable machine-friendly behavior over broad command surface.

## Canonical Commands
- `go test ./...`
- `go build ./cmd/igw`

## Delivery Rules
- Keep changes small and commit in logical slices.
- Maintain stable exit codes for automation.
- Avoid secret leakage in logs and output.

## Prompt Quality Gate
- Push back when requests are ambiguous.
- If objective, scope, constraints, or acceptance checks are missing, ask 1-3 focused clarifying questions before implementing.
- Do not proceed on major assumptions without explicit confirmation.
- When prompt quality is low, suggest a tighter prompt using the templates in `docs/prompting.md`.
