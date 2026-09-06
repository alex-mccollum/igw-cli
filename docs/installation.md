# Installation

These docs describe the working v1 source. The latest published release checked
on 2026-09-06 is v0.5.0, whose commands and output differ. Installing `latest`
does not install this checkout. Use the documentation at the selected release
tag, or build the working source below. See [migration](migration-v1.md) and
[current qualification status](qualification/README.md) before adopting v1.

## Build this checkout

Source builds require Go 1.25.7 or later, as declared in `go.mod`. On a shared
Linux/WSL workstation, use the [bounded runner](development-safety.md):

```bash
bash scripts/bounded-run.sh -- go build -o bin/igw ./cmd/igw
bin/igw version
```

On macOS, native Windows, or an isolated development host, build directly:

```bash
go build -o bin/igw ./cmd/igw
```

On Windows, use `-o bin/igw.exe`. Add the binary's directory to PATH or use its
explicit path. An unstamped checkout build normally reports `dev`; published
artifacts report their release tag. Go is not needed to run a prebuilt binary.

Gateway reachability and a permitted full API token (`name:key`) are required
for authenticated online operations. Command schemas, local file inspection,
and explicit bundled-reference discovery work without a Gateway or credentials.

## Install a published release

Replace `vX.Y.Z` with an existing release tag. Pin the installer and binary to
the same release for reproducible installation. Linux/macOS:

```bash
curl -fsSL https://raw.githubusercontent.com/alex-mccollum/igw-cli/vX.Y.Z/scripts/install.sh -o /tmp/igw-install.sh
bash /tmp/igw-install.sh --version vX.Y.Z --dir "$HOME/.local/bin"
```

Windows PowerShell:

```powershell
Invoke-WebRequest "https://raw.githubusercontent.com/alex-mccollum/igw-cli/vX.Y.Z/scripts/install.ps1" -OutFile "$env:TEMP\igw-install.ps1"
powershell -ExecutionPolicy Bypass -File "$env:TEMP\igw-install.ps1" -Version vX.Y.Z
```

The installers verify archive checksums. Their options are:

| Shell / PowerShell option | Purpose |
| --- | --- |
| `--version` / `-Version` | Explicit tag, or `latest` (default) |
| `--dir` / `-InstallDir` | Installation directory |
| `--repo` / `-Repo` | Alternate `OWNER/REPO` |

Developers can also install a published Go module version:

```bash
go install github.com/alex-mccollum/igw-cli/cmd/igw@vX.Y.Z
```

On shared Linux/WSL, run that command through the checkout's bounded runner.
The selected tag's `go.mod` determines its toolchain requirement.

## Manual artifacts and application bootstrapping

Download the archive for your platform and `checksums.txt` from the same
[release](https://github.com/alex-mccollum/igw-cli/releases).
`release-manifest.json` optionally provides artifact names, OS/architecture,
checksums, and download URLs for applications. See
[artifact naming and verification](releasing.md#artifacts-and-integrity) for the
single naming contract, latest aliases, archive contents, and checksum example.

Archives contain a top-level `igw_<version>_<os>_<arch>/` directory. Extract it
and place its `igw` or `igw.exe` on PATH. Run `igw version`, then follow the
[configuration guide](configuration.md) and [canonical commands](commands.md)
for a binary built from this source; use release-tag docs for older binaries.
