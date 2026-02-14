# Contributing

Thanks for contributing to `igw-cli`.

## Ground Rules

- Keep the CLI lightweight; default to the Go standard library and add dependencies only when they provide clear, durable value.
- Keep behavior machine-friendly and stable for automation.
- Keep edits small and focused.

## Local Development

```bash
go test ./...
go build ./cmd/igw
```

## Pull Request Checklist

- Add or update tests for behavior changes.
- Update docs when command behavior changes.
- Preserve stable exit codes and error-classification behavior.
- Do not include secrets in output, tests, docs, or commit history.
