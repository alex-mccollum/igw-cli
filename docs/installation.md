# Installation

## Requirements
- Go `1.23+` (for source install/build).
- Network reachability to your Ignition Gateway.
- Ignition API token with required permissions.

## Install from Source

```bash
go install github.com/alex-mccollum/igw-cli/cmd/igw@latest
```

## Install from Release Artifacts

1. Download the archive for your OS/architecture from GitHub Releases.
2. Extract `igw` (or `igw.exe`) and place it on your `PATH`.

## Verify

```bash
igw version
```

## Next

1. Configure gateway URL and token: `docs/configuration.md`.
2. Run your first health check: `igw doctor`.
3. Use canonical command examples: `docs/commands.md`.
