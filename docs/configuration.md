# Configuration

Runtime precedence is flags > environment > configuration, independently for
URL and token. The environment names remain `IGNITION_GATEWAY_URL` and
`IGNITION_API_TOKEN`. An explicit `--profile` selects a named profile; otherwise
the active profile is used. The selected profile replaces default fields before
environment and runtime flag overrides are applied.

Use the full API token (`name:key`). The CLI accepts persistent token input only
through `profile set NAME --token-stdin`; it does not expose a token-value flag.
Stored values never come implicitly from environment or runtime overrides.

The platform user configuration directory contains `igw/config.v1.json` after
setup or migration. Linux uses `$XDG_CONFIG_HOME` or `~/.config`; macOS uses the
platform Application Support directory; Windows uses `%AppData%`. A valid
legacy `config.json` remains readable until explicit migration. An invalid v1
file never causes fallback to legacy credentials.

See `docs/profiles.md` for profile edits, private storage, migration, rollback,
and revision preconditions. Canonical examples are in `docs/commands.md`.

Gateway URLs must be absolute HTTP(S) addresses without credentials, query,
fragments, traversal segments, or backslashes. Proxy base paths are preserved.
Configure the final reachable address explicitly, including when accessing a
Windows-hosted Gateway from WSL. Repository commands do not change WSL, Docker
Desktop, host services, or memory configuration.
