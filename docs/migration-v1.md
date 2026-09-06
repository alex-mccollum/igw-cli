# Migrate from 0.x to v1

The working source now uses one `igw` entrypoint. The old `igw-next` entrypoint,
legacy parser and wrappers, persistent RPC, and CWD OpenAPI loader are removed.
This is an intentional major-version interface change. The v1 source has not
been published by the rebuild workflow. See
[current qualification status](qualification/README.md) for the tested commits
and the checks still required for a release candidate.

## Configuration

The runtime environment names and flags > environment > configuration
precedence are unchanged. The v1 runtime reads valid legacy `config.json` when
no v1 file exists. Editing legacy settings requires explicit migration, which
creates `config.v1.json` while preserving the original bytes. Rollback requires
the unchanged legacy file and a reviewed v1 revision, and archives current v1
bytes before returning to legacy. See `docs/profiles.md` for exact behavior and
`docs/commands.md` for canonical commands.

Tokens are supplied through `IGNITION_API_TOKEN` or profile stdin input. Keep
the full `name:key` value. There is no token-value command-line flag. Stored URL
changes use `profile set NAME --url URL`; `--gateway-url` remains a runtime
override. Creating a first profile no longer silently selects it: use `--use`.

## Command mapping

| 0.x command or option | v1 equivalent or approach |
| --- | --- |
| `config set`, `config profile add/use/list` | `profile set/use/list`; explicit local `--dry-run` or `--yes` for edits |
| `config show` | `profile show` for the effective target; `profile list` for stored names/revision |
| `doctor`, `gateway info` | `gateway doctor` |
| `api sync/refresh` | `spec sync` |
| `api show`, `api search` | `api describe OPERATION`, `api list --search TEXT` |
| `api capability` | `api capabilities` for reviewed workflow prerequisites |
| `call --op OPERATION` | `api request OPERATION` |
| `call --method METHOD --path PATH` | `api raw --method METHOD --path PATH` for an explicit unvalidated escape hatch |
| `call --batch`, persistent `rpc` | `api batch --input @batch.json` for bounded sequential work; otherwise one process per request |
| `tags export/import` | `tag export/import`; inspect current flags and supported verification coverage |
| `restart tasks/gateway` | `gateway restart-tasks/restart`; restart includes bounded observation |
| `diagnostics bundle generate` | `diagnostics bundle collect` with preview/confirmation and polling |
| `wait diagnostics-bundle/restart-tasks` | Completion checks are part of the corresponding workflow; task status remains separately inspectable |
| `wait gateway` | Use explicit bounded read-only polling around `gateway doctor` when needed |
| `scan`, logger mutations, backup restore | Discover/describe the actual operation, then explicitly prepare and confirm it through the generic API surface; no inferred workflow verification |
| `--spec-file`, implicit CWD `openapi.json` | `spec import/inspect`, target-bound catalogs, or explicit `--reference` discovery |
| `--select`, `--raw`, `--compact` output transforms | Consume the single JSON result with a JSON tool; `api raw` describes request validation, not output formatting |
| RPC workers/queue controls and automatic fallback | Removed; use application-level process limits and do not replay uncertain writes |
| `schema --command PATH` | `schema COMMAND... --json` or `COMMAND... --help --json` |
| `config set --auto-gateway` | Configure the reachable HTTP(S) address explicitly; no WSL helper or host lifecycle action |

Resource/project changes have dedicated typed workflows with supported signatures
or reviewed content digests. They provide stronger preconditions and readback
than forwarding a generic request. A generic operation remains available when
an undocumented or incomplete schema cannot represent an intended request,
but it does not claim schema validation or workflow completion.

## Automation contract changes

Every JSON result is a single `igw/v1` envelope with `ok`, `outcome`, `data`,
and `meta`. `data` can be null; `error` is included only on failure. Completed
downloads add `artifact`. Gateway JSON stays structured
rather than becoming a JSON string. Preserve large numeric values when choosing
an application JSON decoder. Parse errors also use the envelope.

Exit codes remain 0 (success), 2 (usage/configuration), 6 (auth/permission), and
7 (transport, non-auth HTTP, artifact, cancellation, or verification failure).
`ok` does not imply final-state verification: inspect `outcome` and
`meta.verification`. An uncertain write must be inspected before any new change;
do not convert process failure into automatic fallback or replay.

Use `--out` for a deliberate artifact path and `--overwrite` for replacement.
Downloads no longer silently choose filenames. Batches are JSON arrays with
unique IDs, bounded inputs/results, ordered output, and explicit continuation
rules. They are not atomic transactions or a replacement for desired-state
orchestration.

Previews send no proposed mutation. They can require current read access,
including fresh state for resource/project workflows. Schema-assisted writes
refresh their catalog in the invocation; an imported or old CWD spec does not
silently authorize them. See `docs/catalog.md` for freshness, pins, and offline
reference semantics.

## Preserved release contracts

Human `igw version`, `igw --version`, and `igw -v` retain the
`igw version VERSION` prefix. JSON version output uses the common envelope.
Artifact/archive/latest-alias naming and `release-manifest.json` remain stable.
Existing published 0.x releases keep their own command contracts; scripts must
not assume that a major upgrade can retain old argv or response shapes.
