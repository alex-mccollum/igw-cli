# ADR-0002: One typed v1 CLI

## Status

Accepted for the active v1 rebuild; supersedes ADR-0001's argument-parser and
standard-library-only MVP choices. Release readiness still requires the workflow
release gates and final-source Gateway evidence.

## Decision

Keep Go and a thin operational scope. Use one Cobra command tree and typed
execution/workflow services, a versioned result contract, target-bound HTTP,
private streamed artifacts, and versioned profile/catalog storage. Use a JSON
contract index and direct `jsonschema/v6` validation behind a catalog boundary
checked against actual captures. The JSON engine supersedes
the original libopenapi model/validator choice.

The target Gateway's exact OpenAPI document supplies the documented wire
contract; actual responses/validation and reviewed workflow policy establish
state, permissions, effects, and completion. Keep those authorities distinct.

Remove persistent RPC, the legacy CLI registry/parser, and the CWD OpenAPI
index. Host tools use bounded CLI processes or explicit sequential batches.
Migration preserves legacy configuration with explicit confirmation and rollback.
Keep exit codes, environment names, per-field precedence, and artifact naming.

## Consequences

There is one command surface to discover, test, complete, document, and package.
Workflows can share prepared requests and catalog scope without parsing argv.
The v1 interface intentionally breaks 0.x argv and JSON shapes; the migration
guide makes these changes explicit. Complete schema inspection and real Gateway
qualification of the selected release workflows remain required. Additional
serialization coverage is deferred unless those workflows need it; the current
release criteria are in `docs/qualification/README.md`.
