# RPC migration

Persistent RPC is removed in v1. There is no NDJSON handshake, queue, worker,
cancel operation, or compatibility fallback server in the rebuilt CLI.

Use one bounded `igw` process or an explicit sequential `api batch`. Preserve
uncertain mutation outcomes; do not replay a request when changing transports.
See `docs/host-integration.md` for the process contract and
`docs/migration-v1.md` for command and result migration.

Historical 0.x protocol behavior remains in repository history and applies
only to those older releases.
