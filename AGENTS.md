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

## Atomic Commit Protocol
- Default to atomic commits after each completed, verifiable slice of work.
- Never include unrelated files in a commit.
- If the working tree is already dirty, stage only files touched for the current slice.
- Before commit, review `git status --short` and `git diff --staged` and confirm scope matches the slice.
- In a dirty working tree, do not use broad staging/commit commands like `git add .` or `git commit -a`.
- If unexpected modified files appear, stop and ask the user before committing.
- Use path-scoped staging and commit flow:
  - `git add -- <path1> <path2>`
  - `git commit -m "<type(scope): summary>"`
