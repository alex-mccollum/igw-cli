# Changelog

All notable user-facing changes to `igw` are documented here.

## [v0.3.0](https://github.com/alex-mccollum/igw-cli/compare/v0.2.0...v0.3.0) - 2026-02-21

### Added
- `igw api tags` to list unique OpenAPI tags from the active/local spec.
- `igw api stats` to summarize API operations by method, tag, and path prefix.
- New focused docs pages:
  - `docs/installation.md`
  - `docs/configuration.md`
  - `docs/examples.md`
  - `docs/troubleshooting.md`
- Docs quality lint script `scripts/lint-docs.sh` to catch:
  - placeholder metadata values in docs,
  - hardcoded local absolute paths,
  - known default-port drift patterns,
  - references to missing `docs/*.md` pages.

### Changed
- CI docs job now runs both command-doc consistency checks and docs-quality lint checks.
- Top-level docs indexing improved:
  - `README.md` now links to focused docs pages,
  - `docs/README.md` includes the new docs map.

### Fixed
- Test reliability across Linux/macOS/Windows for API sync/spec fallback flows by isolating user config directories in CLI tests.

## [v0.2.0](https://github.com/alex-mccollum/igw-cli/compare/v0.1.0...v0.2.0) - 2026-02-16

### Added
- `igw api sync` and `igw api refresh` to fetch and refresh cached OpenAPI docs from the gateway.
- `igw wait gateway`, `igw wait diagnostics-bundle`, and `igw wait restart-tasks` for polling operational readiness.
- JSON extraction/output controls for automation:
  - Repeatable `--select` for subset JSON object output.
  - `--raw` for a single selected plain value.
  - `--compact` for one-line JSON.
- Expanded machine-oriented `--json` support across command flows for agent/script usage.

### Changed
- Safer and simpler defaults across common commands:
  - `igw call` defaults `--method` to `GET` when `--path` is provided.
  - `igw tags export` defaults `--provider=default` and `--type=json`.
  - `igw tags import` defaults `--provider=default`, infers `--type` from file extension, and defaults `--collision-policy=Abort`.
  - `igw logs download`, `igw diagnostics bundle download`, and `igw backup export` now default output filenames when `--out` is omitted.
- API discovery now checks default spec locations first and auto-syncs OpenAPI from the gateway for `api` and `call --op` when needed.
- `igw doctor` behavior is now read-first by default, with write validation available via `--check-write`.

### Fixed
- Improved wrapper flag validation and command contract consistency for invalid/ambiguous inputs.
- Improved machine-output contract consistency for automation error handling.
