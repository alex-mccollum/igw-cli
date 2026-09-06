# Automation

Use `--json` and the process exit code. A CLI invocation emits one JSON result;
human-readable output is intended for interactive use. The same command tree
supplies help, completion, and `schema [COMMAND...] --json` without connectivity.
Canonical executable examples are maintained in `docs/commands.md`.

## Result contract

| Field | Meaning |
| --- | --- |
| `version` | Envelope contract, currently `igw/v1` |
| `ok` | Whether this invocation succeeded under its reported scope |
| `outcome` | Such as `preview`, `accepted`, `completed`, `failed`, or `uncertain` |
| `data` | Structured API data or a typed workflow report |
| `error` | Redacted `kind`, `message`, `exitCode`, and optional `details` |
| `meta` | Target, catalog/reference provenance, freshness, validation, verification, warnings |
| `artifact` | A completed output file with path, bytes, and SHA-256 when applicable |

Generic HTTP success does not prove a change was applied as intended. Workflow
verification is limited to its stated observations. In particular, Gateway
restart observation is not module-health certification or unique node identity
proof. Preserve `uncertain` results and partial batch reports for review.

Exit codes remain 0/2/6/7. Auth failures are 6; usage/configuration errors are 2;
transport, non-auth HTTP, output, artifact, cancellation, and verification errors
are 7. Do not retry a mutation because its process failed or output was missing.
Read state first and deliberately decide whether another change is appropriate.

For request-body schema issues, `field` is a JSON Pointer into the submitted
body: `/` in a property name becomes `~1`, and `~` becomes `~0`. Array elements
use numeric tokens. An omitted `field` means the body root; `/` identifies an
empty-named property. For example, `/a~1b/~01/0` identifies the first element
under `{"a/b":{"~1":[...]}}`. Issue paths never include the rejected values.

## Agent workflow

1. Inspect the command schema and profile target without exposing credentials.
2. Discover and describe the operation on the selected Gateway, or inspect an
   explicit qualified reference when working offline.
3. Read relevant state and prepare the proposed change using `--dry-run`.
4. Review supported preconditions such as resource signatures, project content
   digests, or local profile revisions. They bind different kinds of state.
5. Apply the explicit change with `--yes` and its required preconditions.
6. Check the result's outcome and verification evidence, including warnings.

The full token comes from the environment or private profile storage. Do not
put credentials in argv, URLs, tracked files, diagnostic logs, or prompts.
Previews omit sensitive values; request-body/file identities can still describe
private data and should be retained only where appropriate.

## Batches and process integration

`api batch` accepts a bounded JSON array and produces ordered per-item results.
It processes the batch against one catalog scope and sends no proposed mutation
in preview mode. Default execution stops on failure; explicit continuation still
stops on auth, cancellation, and uncertain outcomes. It is sequential and not a
transaction. For independent processes, bound concurrency in the caller.

See [host integration](host-integration.md) for process spawning, stream
handling, parent deadlines, and adapter failure handling. A missing or invalid
result is a failed observation, not permission to replay.

Use `--out` for large artifacts and verify returned size/hash metadata. Preserve
exact JSON numbers in host decoders when they can exceed IEEE-754 integer
precision. Use a JSON selector in the host or tools such as `jq`; the CLI no
longer maintains a separate output-selection language.

## Catalog and freshness

The target's OpenAPI defines its documented wire contract. Keep its original
bytes and evidence separate from imported or bundled references. Schema-assisted
writes normally refresh within the invocation;
`--allow-stale-spec` is the explicit exception for a previously fetched target
snapshot. Offline inspection is explicit. Raw requests do not use a catalog.
A failed refresh preserves the last valid local snapshot but does not silently
make it fresh.
Pins constrain contract identity. See `docs/catalog.md` and
`docs/reference-updates.md` for cache, distribution, and update behavior.

## Verification scripts

`bash scripts/smoke.sh` runs isolated local executable checks on Linux, using
temporary XDG configuration paths and a loopback HTTP fixture. It requires
Python 3. Other platforms retain native Go unit/contract checks; this smoke
refuses to modify their real user configuration for isolation.
`IGW_SMOKE_LIVE=1 bash scripts/smoke.sh` additionally performs explicit read-only
checks against the configured Gateway. Both modes leave live Gateway state
unchanged. Local smoke checks deliberately exercise profile writes/migration in
isolated directories and request behavior
against a loopback fixture; they do not use production credentials for those
checks. On shared Linux/WSL, run the script through the
[bounded runner](development-safety.md).
Real workflow qualification uses the guarded disposable-Gateway suites described
in `docs/catalog.md` and `docs/compatibility-matrix.md`.
Performance scope and remaining optimization work are in `docs/performance.md`.
