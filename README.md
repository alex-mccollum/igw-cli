# igw

`igw` is a lightweight CLI wrapper for the Ignition Gateway API.

## Principles
- Default to the Go standard library; add third-party dependencies only when they provide clear, durable value.
- Generic API execution first (`call`).
- Stable exit codes for automation.

## Install

From source:

```bash
go install github.com/alex-mccollum/igw-cli/cmd/igw@latest
```

From GitHub Releases:

- Download the archive for your OS/architecture.
- Extract it and place `igw` (or `igw.exe`) on your `PATH`.

Verify install:

```bash
igw version
```

## Quickstart (60 Seconds)

The commands in this section are examples. Replace placeholder values for your environment.

Assumptions:
- You can reach your Ignition Gateway.
- You have an Ignition API token with the permissions you need.
- Commands below use `bash` syntax.

```bash
# Example values (replace these)
export IGW_GATEWAY_URL="http://127.0.0.1:8088"
export IGW_TOKEN_FILE="$HOME/.config/igw/token.txt"

# 1) Set your gateway URL
igw config set --gateway-url "$IGW_GATEWAY_URL"

# 2) Set your API key from a local file
igw config set --api-key-stdin < "$IGW_TOKEN_FILE"

# 3) Verify connectivity and permissions
igw doctor

# 4) Run a read call
igw gateway info --json
```

Note:
- `IGW_GATEWAY_URL` and `IGW_TOKEN_FILE` above are shell-local helper variables used in examples.
- The runtime environment variables recognized by `igw` are `IGNITION_GATEWAY_URL` and `IGNITION_API_TOKEN`.

If you are in WSL and Ignition is running on Windows host:

```bash
igw config set --auto-gateway
```

## Commands
- `igw api list|show|search`: query local OpenAPI docs for endpoint discovery.
- `igw call`: generic HTTP executor for Ignition endpoints (or `--op` by operationId).
- `igw config set|show|profile`: local config + profile management.
- `igw doctor`: connectivity + auth checks (URL, TCP, read access, write access).
- `igw gateway info`: convenience read wrapper.
- `igw scan projects`: convenience write wrapper.
- `igw logs ...`: list/download logs and manage logger levels.
- `igw diagnostics bundle ...`: generate/status/download diagnostics bundles.
- `igw backup export|restore`: download or restore gateway backups.
- `igw tags export|import`: tag import/export helpers.
- `igw restart tasks|gateway`: restart task status and gateway restart trigger.

## Defaults
- `igw call` defaults `--method` to `GET` when `--path` is provided.
- `igw tags export` defaults `--provider` to `default` and `--type` to `json`.
- `igw tags import` defaults `--provider` to `default`, infers `--type` from the import file extension (`.json`, `.xml`, `.csv`, fallback `json`), and defaults `--collision-policy` to `Abort`.
- `igw logs download`, `igw diagnostics bundle download`, and `igw backup export` default `--out` filenames when output is an interactive terminal.

## Mutation Safety
- Mutating operations require explicit `--yes` confirmation.
- This includes commands like `scan projects`, `logs logger set`, `logs level-reset`, `diagnostics bundle generate`, `backup restore`, `tags import`, and `restart gateway`.

## Configuration Sources
Precedence is strict:
1. CLI flags
2. Environment variables
3. Config file

Environment variables:
- `IGNITION_GATEWAY_URL`
- `IGNITION_API_TOKEN`

Profiles:
- If `--profile` is omitted and an active profile is set, that active profile is used.
- The first profile created by `igw config profile add` becomes active automatically if no active profile exists.

Config file path:
- Linux/macOS: `${XDG_CONFIG_HOME:-~/.config}/igw/config.json`
- Windows: `%AppData%\\igw\\config.json`

## Examples
All commands below are examples. Replace placeholders for your environment.

Get gateway metadata:

```bash
igw gateway info --json
```

Run health/auth checks:

```bash
igw doctor
```

Run a generic API call:

```bash
igw call --method GET --path /data/api/v1/gateway-info --json
```

For full command examples (wrappers, profiles, API discovery, completions, and smoke checks), use `docs/commands.md`.

## Auth and Connectivity Troubleshooting
- `401 Unauthorized`: token missing/invalid.
- `403 Forbidden`: token authenticated but lacks required gateway permission/security-level mapping.
- Timeout from WSL2 to Windows host: verify gateway host IP and Windows firewall inbound access for port `8088`.

## Exit Codes
- `0`: success (`2xx`)
- `2`: usage/config errors
- `6`: auth failures (`401`, `403`)
- `7`: network/transport and non-auth HTTP failures

## Compatibility Policy

- Exit codes are stable within minor releases.
- JSON output field names are stable within minor releases.
- Breaking CLI or output contract changes are introduced only in a major release.

## Versioning Policy

- Releases follow semantic versioning: `vMAJOR.MINOR.PATCH`.
- `PATCH`: bug fixes and non-breaking internal/doc changes.
- `MINOR`: new backward-compatible commands/flags/behavior.
- `MAJOR`: breaking behavior or output contract changes.

## Build

```bash
go build -trimpath -ldflags="-s -w" -o bin/igw ./cmd/igw
```

## Test

```bash
go test ./...
```

## Releasing

See `docs/releasing.md` for tag-based release steps and artifact naming.
