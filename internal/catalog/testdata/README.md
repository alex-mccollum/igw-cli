# Catalog fixtures

The four canonical vendor documents live in `internal/reference/bundles/`.
`captured_test.go` checks identities, operation counts, request validation, and
known vendor gaps against each version/module profile. Synthetic tests isolate
individual binding and schema edge cases. `legacy-keyboard-config.json` is the
small vendor-derived regression for the keyboard definition adapter.

Repeated captures, failed-run receipts, and superseded parser qualification
records remain available in Git at commit `65e643d`.
