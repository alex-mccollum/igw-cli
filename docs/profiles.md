# Profiles and v1 configuration migration

The v1 CLI manages its own profiles through `profile set`, `use`,
`remove`, `list`, and `show`. Canonical examples are in `docs/commands.md`.
Every local change requires either `--dry-run` or `--yes`, exclusively. Profile
commands never contact a Gateway, commission it, or change host configuration.

`set NAME` creates or updates only explicit fields. `--url` stores the URL;
`--token-stdin` reads a token through EOF, capped at 16 KiB; `--clear-token`
removes a stored token. `--use` selects the profile after the edit. Omitting a
field preserves it, including when changing the URL. Tokens are opaque strings:
surrounding whitespace is trimmed, embedded control characters and malformed
UTF-8 are refused, and an empty stdin token requires explicit `--clear-token`.
Stdin reads honor cancellation and the invocation deadline.

Use a named profile for each Gateway. `use NAME` selects it; `use --default`
clears the selection and uses the preserved default fields plus environment and
flags. Removing the active profile is refused until another profile or the
defaults are selected. Creating the first profile does not silently activate
it: specify `--use` when that is intended.

Runtime resolution remains flags > environment > configuration, independently
for URL and token. An explicit `--profile` selects a named profile instead of
the active one. A selected profile replaces the default URL and token, after
which `IGNITION_GATEWAY_URL`, `IGNITION_API_TOKEN`, and the runtime URL override
are applied. `profile show` reports this effective target and token presence.
`profile list` reports stored names, active selection, source, and revision.
Generated shell completions suggest local profile names for `--profile` and
profile arguments without exposing URLs/tokens or contacting a Gateway.

Local edits never save environment values or runtime overrides. They reject
`--gateway-url` and `--profile` to keep the stored edit explicit: use `set NAME
--url URL`. Tokens have no command-line value flag and are absent from previews,
success reports, and errors. Configuration files contain plaintext credentials;
use the environment when persistence is unnecessary.

## Storage and migration

Both files live under the platform user configuration directory in `igw`:

| File | Purpose |
| --- | --- |
| `config.json` | Legacy defaults, active selection, and named profiles |
| `config.v1.json` | Versioned configuration used by the v1 CLI |
| `config.v1.lock` | Persistent file used for cooperative process locking |
| `config.v1.rollback-REVISION.json` | Preserved v1 bytes from explicit rollback |

With no files, profile setup creates v1 directly. With only a legacy file,
the v1 CLI can read it, but profile edits require explicit migration.
Migration copies all recognized default and profile fields into a v1 document
without changing the legacy bytes. It preserves existing active selection and
per-field resolution; it does not automatically name or select legacy defaults.

The v1 document contains `schemaVersion: "igw-config/1"`, a random `revision`,
and `configuration` with the existing fields. A migrated document also retains
the private SHA-256 of the original legacy bytes for rollback checks. That hash
is never emitted in command reports. The default runtime prefers v1 whenever
present; an invalid v1 file does not silently fall back to legacy credentials.
A separately retained 0.x executable continues to read `config.json`. It does
not read or update the v1 copy; avoid editing either file during migration.

Migration copies settings only. It does not promote a legacy CWD OpenAPI file
or import it as a fresh Gateway contract. V1 catalogs retain their separate
target-bound storage and freshness rules in `docs/catalog.md`.

Reads are capped at 1 MiB. Duplicate keys, malformed Unicode, null fields,
unknown or incorrectly cased fields, wrong types, invalid profile names,
missing active profiles, and credentialed/query-bearing/fragment-bearing URLs
are refused. Resolve such errors in a private copy before retrying migration;
the CLI does not guess how to discard or reinterpret unknown configuration.
URL traversal segments and backslashes are also refused, consistently with
runtime target validation. Previews validate the resulting document and its
encoded size before reporting an applicable edit.

## Review, concurrency, and rollback

A preview reports stored targets and token presence before and after the edit,
with `applied: false` and `outcome: "preview"`. It creates no directories,
temporary files, or locks. A successful write reports `applied: true`,
`outcome: "completed"`, and a new opaque revision. Token values and token hashes
are not exposed. `set` reports `tokenChange` as `preserved`, `set`, or `cleared`,
so a rotation remains visible even when token presence stays true.
`--if-revision` requires the current v1 revision from `profile
list` or `data.before.revision` in a preview. A stale revision fails with exit 2.
This precondition binds the configuration being edited; the command still
specifies the proposed edit. A preview does not reserve a future revision.

Legacy files have no v1 revision: migration previews describe current legacy
settings, and `migrate --yes` reads them again. Migration does not accept
`--if-revision`. The source bytes are rechecked after acquiring the writer lock
within the confirmed invocation. Do not edit configuration with another tool
between review and execution.

Rollback requires a migrated v1 file, an explicit current `--if-revision`, and
an unchanged legacy file. It first preserves the exact current v1 bytes in a
private archive, including any edits since migration, then deactivates v1 so
runtime reads use legacy again. It never replaces an archive with different
content. If interrupted after archive creation, retrying the same revision can
reuse that identical archive. If legacy was edited or removed, rollback refuses
and keeps v1 active. Archives remain private local recovery material; they are
not automatically deleted or loaded. A later migration reads the legacy file
again and generates a new revision.

Writers require a real private directory (0700 on Unix), use unique 0600
temporary files, flush file contents, and publish complete files. New files and
archives use no-clobber publication. Unix replacement uses same-directory
rename; the Go API does not promise atomic rename on non-Unix platforms.
Windows inherits the user's directory ACL; POSIX mode bits are not an ACL
guarantee. Filesystems lacking the required hard-link or locking operations
fail explicitly. Use a private local filesystem rather than a shared network
configuration directory.

The OS file lock is held only during confirmed writes and released when its
descriptor closes or the process exits. A competing writer fails promptly;
it does not wait indefinitely. Never delete the lock file to unlock it: the
file's existence is normal. Locks and random revisions coordinate this CLI's
writers. They do not prevent an external editor or the legacy executable from
changing files. Do not edit these files concurrently or reuse an old revision
after manually altering a document. Publication is not a multi-file transaction
or a guarantee against storage failure or power loss.

Implementation references: Go's [file publication API](https://pkg.go.dev/os#Rename)
and Microsoft's [file locking contract](https://learn.microsoft.com/en-us/windows/win32/api/fileapi/nf-fileapi-lockfileex).
