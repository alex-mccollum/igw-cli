# Troubleshooting

Start with `profile show` to inspect the selected target and token presence,
then `gateway doctor --json` for a read-only Gateway request. See
`docs/commands.md` for canonical commands and `docs/migration-v1.md` if an old
command or flag is rejected.

`gateway doctor` establishes that the Gateway information request completed.
It does not test each module, device, project, or database connection. Its human
output states that scope; JSON preserves the Gateway information response.

## Investigate a failed configuration or deployment

1. Preserve the failed command's JSON result. Check its `outcome`, error kind,
   and verification evidence. Read the current resource or export the affected
   project/tags before deciding whether another change is needed.
2. Query recent warnings with `igw logs list --min-level WARN --since 1h`.
   Inspect the message, logger, timestamp, stack, and context. Narrow the
   selection by logger or search term after observing the relevant entries.
3. Use absolute `--since` and `--until` times for the incident window. Retain
   the same window and filters while following the reported page offset.
   No events on one page does not establish that the Gateway has no problems.
4. Use `--json` when passing events to an agent or script. Preserve exception
   context and distinguish observed errors from inferred causes. The CLI
   retrieves evidence; it does not automatically diagnose or repair a device.
5. Save the complete log database with `logs download`, or collect a diagnostic
   bundle explicitly when more context is needed. Log download is the Gateway's
   database artifact, not a plain-text transcript. Bundle generation requires
   `--yes`; review its preview and use a suitable explicit timeout.

All steps must use the same profile and effective Gateway URL as the failed
change. See [operational command examples](commands.md) for complete invocations.
Human mode includes recovery guidance without changing JSON or exit codes.

## Configuration and authentication

Exit 2 means usage or configuration needs attention. The v1 reader rejects
ambiguous, invalid, or unknown configuration fields and never silently falls
back from an invalid v1 file to legacy credentials. Inspect profile source and
revision, then follow `docs/profiles.md` for migration or rollback.

A 401 or 403 returns exit 6. Verify the full `name:key` token, required secure
connection, and Gateway security-level mapping. A 403 alone does not prove that
a supplied token is valid. Rotate or clear stored tokens through explicit stdin
input or clearing; do not place them in command arguments or diagnostic output.

## Transport and artifacts

Exit 7 covers transport and non-auth HTTP failures, artifacts, cancellation,
and unsuccessful verification. Confirm the configured host, port, scheme, and
proxy path. The CLI does not forward tokens to another origin or replay an
uncertain write. Inspect current state before another mutation.

For WSL-to-Windows access, configure the reachable Gateway address explicitly
and inspect reachability separately. Repository validation does not restart or
repair WSL/Docker Desktop, change Windows services, or alter memory settings.
See `docs/development-safety.md` for the recorded incident and safeguards.

Artifact failures preserve the previous destination. Check the explicit output
path, capacity, overwrite choice, and supported local filesystem behavior.
Configuration writes require a private local directory; an existing lock file
is normal and must not be deleted to bypass another writer. Retry guidance is
reserved for active writer contention. Permission, invalid-file, and unsupported
locking errors require correcting the underlying filesystem problem.

## Catalog and verification

Refresh with `spec sync`. Inspect a file with `spec inspect`, or explicitly
import it for offline reference use. The old `--spec-file` and CWD lookup are
removed. Fresh writes cannot rely silently on a stale or imported document.

If validation reports a vendor gap, inspect the complete operation and reported
corrections. Do not infer undocumented state or permissions from a schema.
Generic raw requests remain available with explicit confirmation but do not
claim schema validation. Verification failures retain their stated evidence
and limits; see `docs/catalog.md` and `docs/compatibility-matrix.md`.

## Translations singleton qualification limit

Translations creation can return `uncertain`/exit 7 when the Gateway
acknowledges the request but omits `config` from readback. Do not replay it
automatically. The [singleton compatibility notes](compatibility-matrix.md#singletons)
record the tested versions, observed limits, and original evidence.
