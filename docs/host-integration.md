# Host and agent integration

Spawn one `igw` process for a bounded request or use an explicit sequential
`api batch`. Persistent RPC, worker queues, and automatic compatibility fallback
are removed in v1. The shared execution core is inside the CLI; host applications
consume its versioned process contract.

At startup, inspect `version --json`, `exit-codes --json`, and `schema --json`.
Require `version: "igw/v1"` on result envelopes. The application version is in
`data.version` for the version command. Pin an appropriate binary/release and
use operation-level catalog inspection before making assumptions about a
particular Gateway's version or installed modules.

Pass arguments as an array without a shell. Set a deliberate target/profile,
pass the complete token through the environment or private profile stdin setup,
and drain both output streams. Avoid command-line tokens and logging secret
input. Use CLI deadlines plus an outer process deadline with cleanup for only
the child your adapter created.

Interpret the exit code and JSON envelope together. A successful process with
an invalid envelope is an adapter error; a failed or disconnected mutation can
have taken effect. Keep `uncertain` and partial results. Never turn a handshake,
parse, or transport failure into an automatic second write.

Use current previews and supported preconditions, then inspect verification
metadata. Resource signatures, project content digests, and local profile
revisions protect their documented scopes; none is a universal server transaction
or job identifier. Generic HTTP acceptance is not workflow verification.

Use returned artifact metadata for files and a lossless JSON decoder for large
numbers. Do not assume fields from the 0.x envelope or RPC protocol remain.
See `docs/automation.md`, `docs/migration-v1.md`, and `docs/commands.md`.
