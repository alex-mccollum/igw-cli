# Practical workflows

Executable examples are canonical in `docs/commands.md`. Use the command tree's
help or `schema COMMAND... --json` to inspect flags for a particular task.

For a new Gateway, configure its explicit address and full token, inspect the
effective profile, run the read-only doctor, then inspect the Gateway catalog.
Use a named profile for each target. Existing 0.x settings have an explicit
preview/migration/rollback path in `docs/profiles.md`.

For a configuration resource, discover its type and defaults, read current
state, and preview the proposed writable fields. Keep the reviewed signature
for replacement or deletion. Apply with confirmation and inspect the readback
result; stored configuration verification does not certify operational health.

For projects, inspect and export a ZIP, then use the import workflow and reviewed
content digest for replacement. For tags, inspect catalog capability and format/
collision policy support before import; readback verification has explicit
coverage limits. See `docs/compatibility-matrix.md` for actual qualified cases.

For diagnostics, inspect status first, explicitly preview/confirm collection,
and let the workflow poll completion before downloading a bounded artifact.
Gateway restart likewise requires an explicit baseline, one mutation, and
read-only observation under a deadline. It does not infer success from HTTP
availability alone.

For an API without a dedicated workflow, describe its complete operation, prepare
its encoded inputs, and use `api request`. When the vendor contract is incomplete,
`api raw` is an explicit escape hatch; it does not claim schema validation or
final-state verification. Keep those limits in automation result handling.

For offline work, inspect bundled references or exported snapshots. References
retain exact evidence and remain available without a live Gateway, but they do
not silently become the current target's contract or authorize writes.
