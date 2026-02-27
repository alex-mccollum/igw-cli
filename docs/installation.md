# Installation

## Requirements
- Go `1.23+` (for source install/build).
- Network reachability to your Ignition Gateway.
- Ignition API token with required permissions.

## Install for Host Applications/Agents (Recommended)

Linux/macOS:

```bash
curl -fsSL https://raw.githubusercontent.com/alex-mccollum/igw-cli/vX.Y.Z/scripts/install.sh -o /tmp/igw-install.sh
bash /tmp/igw-install.sh --version vX.Y.Z --dir "$HOME/.local/bin"
```

Windows (PowerShell):

```powershell
Invoke-WebRequest "https://raw.githubusercontent.com/alex-mccollum/igw-cli/vX.Y.Z/scripts/install.ps1" -OutFile "$env:TEMP\igw-install.ps1"
powershell -ExecutionPolicy Bypass -File "$env:TEMP\igw-install.ps1" -Version vX.Y.Z
```

Installer options:
- `--version` / `-Version`: explicit release tag to pin (`vMAJOR.MINOR.PATCH`).
- `--dir` / `-InstallDir`: install target directory.
- `--repo` / `-Repo`: alternate GitHub repo (`OWNER/REPO`).

## Install for Developers (Go)

```bash
go install github.com/alex-mccollum/igw-cli/cmd/igw@vX.Y.Z
```

## Install for Operators/CI (Release Artifacts)

1. Download:
   - your OS/arch archive from GitHub Releases,
   - `checksums.txt`,
   - optional `release-manifest.json` (machine-readable artifact metadata).
2. Verify checksums:

```bash
ARCHIVE="igw_vX.Y.Z_linux_amd64.tar.gz"
grep "  ${ARCHIVE}$" checksums.txt | sha256sum -c -
```

3. Extract `igw` (or `igw.exe`) and place it on your `PATH`.

Manifest notes (`release-manifest.json`):
- Includes release `version` and artifact entries (`name`, `os`, `arch`, `archive`, `sha256`, `url`).
- Intended for host apps/agents that need deterministic install/bootstrap logic.

## Verify

```bash
igw version
```

## Next

1. Configure gateway URL and token: `docs/configuration.md`.
2. Run your first health check: `igw doctor`.
3. Use canonical command examples: `docs/commands.md`.
