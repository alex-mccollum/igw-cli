# Contributing

Thanks for contributing to `igw-cli`.

## Ground Rules

- Keep the CLI lightweight; default to the Go standard library and add dependencies only when they provide clear, durable value.
- Keep behavior machine-friendly and stable for automation.
- Keep edits small and focused.

## Local Development

Use Go 1.25.7 or later, as declared in `go.mod`. On shared Linux/WSL
workstations, run each check through the verified resource guard:

```bash
bash scripts/bounded-run.sh -- go test ./...
bash scripts/bounded-run.sh -- go build -o bin/igw ./cmd/igw
bash scripts/bounded-run.sh -- bash scripts/check-command-docs.sh
bash scripts/bounded-run.sh -- bash scripts/lint-docs.sh
```

Run one validation job at a time. If admission fails, diagnose it without
bypassing limits or changing WSL/Docker Desktop settings. On other supported
development platforms, run the underlying Go commands directly. See
[workstation safety](docs/development-safety.md) for exact requirements and
[reference maintenance](docs/reference-updates.md) for the coordinator, which
creates its own bounded stages and must be invoked directly.

Documentation has one [index](docs/README.md). Keep executable CLI examples in
[commands](docs/commands.md), current results in
[qualification status](docs/qualification/README.md), and workflow evidence
limits in [compatibility](docs/compatibility-matrix.md). The docs lint checks
authored Markdown links/headings and command examples without Gateway access.

## Pull Request Checklist

- Add or update tests for behavior changes.
- Update docs when command behavior changes.
- Preserve stable exit codes and error-classification behavior.
- Do not include secrets in output, tests, docs, or commit history.
