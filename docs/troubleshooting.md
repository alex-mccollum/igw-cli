# Troubleshooting

Start with `profile show` to inspect the selected target and token presence,
then `gateway doctor --json` for a read-only Gateway request. See
`docs/commands.md` for canonical commands and `docs/migration-v1.md` if an old
command or flag is rejected.

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
is normal and must not be deleted to bypass another writer.

## Catalog and verification

Refresh with `spec sync`. Inspect a file with `spec inspect`, or explicitly
import it for offline reference use. The old `--spec-file` and CWD lookup are
removed. Fresh writes cannot rely silently on a stale or imported document.

If validation reports a vendor gap, inspect the complete operation and reported
corrections. Do not infer undocumented state or permissions from a schema.
Generic raw requests remain available with explicit confirmation but do not
claim schema validation. Verification failures retain their stated evidence
and limits; see `docs/catalog.md` and `docs/compatibility-matrix.md`.
