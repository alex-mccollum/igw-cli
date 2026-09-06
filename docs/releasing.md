# Releasing

This project uses a lightweight tag-based release flow.

## One-command release (recommended)

Use:

```bash
./scripts/release/cut.sh v0.1.0
```

`cut.sh` enforces this order:

1. Requires a clean working tree.
2. Verifies `CHANGELOG.md` includes `## [v0.1.0]`.
3. Runs `./scripts/release/dry-run.sh v0.1.0`.
4. Creates local tag `v0.1.0` (or verifies it already points to `HEAD`).
5. Runs `./scripts/release/checklist.sh v0.1.0`.
6. Pushes `HEAD` and `refs/tags/v0.1.0` to `origin`.

## Create a release

1. Ensure `main` is green:
   - `go test ./...`
   - `go build ./cmd/igw`
   - `./scripts/check-command-docs.sh`
   - `./scripts/lint-docs.sh`
2. Ensure `CHANGELOG.md` has a release heading for the tag:
   - `## [v0.1.0](https://github.com/alex-mccollum/igw-cli/compare/v0.0.0...v0.1.0) - YYYY-MM-DD`
3. Cut and push the release:

```bash
./scripts/release/cut.sh v0.1.0
```

4. GitHub Actions `release.yml` runs preflight + build + publish:
   - verifies `CHANGELOG.md` contains `## [v0.1.0]`,
   - verifies the release tag resolves to the workflow commit on tag-triggered runs,
   - verifies the release tag exists on `origin`,
   - builds and packages all platform artifacts,
   - runs packaged Linux `amd64` version and isolated executable smoke verification,
   - generates stable latest aliases (`igw_<os>_<arch>.<ext>`),
   - generates `checksums.txt`,
   - generates `release-manifest.json`,
   - publishes a GitHub Release with generated notes.

## Pre-push release guard (recommended)

Enable repo-managed hooks:

```bash
./scripts/install-git-hooks.sh
```

When enabled, `scripts/hooks/pre-push` automatically runs
`./scripts/release/checklist.sh <tag>` for any pushed `vMAJOR.MINOR.PATCH` tag.
This blocks tag pushes if changelog/tag checks fail.
When checklist runs from `pre-push` (or `cut.sh`), it skips dry-run push auth probes so local validation does not repeatedly prompt for SSH passphrases.

## Tag Failure Recovery

If a pushed release tag fails preflight due to release metadata drift (for example missing changelog heading):

1. Fix `CHANGELOG.md` and any release metadata on `main`.
2. Prefer creating the next patch release tag (for example `v0.4.1`) from the corrected commit.
3. Avoid force-moving an already published tag unless you explicitly intend to rewrite release history.

## Version contract

- Release artifacts must print the release tag in `igw version` output.
- CI enforces this contract for the packaged Linux `amd64` artifact with:

```bash
./scripts/check-version-contract.sh <binary-path> <tag>
```

- The check validates output starts with `igw version <tag>`.
- Release builds include commit/date metadata, so full output may be:
  - `igw version v0.3.1 (abc1234, 2026-02-22)`

## Checksums

- Release publishing generates `checksums.txt` with SHA-256 digests for all `.tar.gz` and `.zip` artifacts.
- Downloaded artifacts should be verified against this manifest before installation.

Verify one artifact deterministically:

```bash
ARCHIVE="igw_v0.4.0_linux_amd64.tar.gz"
grep "  ${ARCHIVE}$" checksums.txt | sha256sum -c -
```

## Release Manifest

- Release publishing generates `release-manifest.json`.
- Manifest fields include:
  - top-level `version`,
  - `artifacts[]` entries with `name`, `os`, `arch`, `archive`, `sha256`, and `url`.
- Host applications can consume this manifest to select the correct artifact and verify checksums without scraping HTML pages.

## Post-Release Smoke Check

After publishing, validate checksums and one installed artifact:

```bash
ARCHIVE="igw_v0.4.0_linux_amd64.tar.gz"
grep "  ${ARCHIVE}$" checksums.txt | sha256sum -c -
```

Then run:

```bash
igw version
igw gateway doctor --gateway-url http://127.0.0.1:8088 --json
```

## Manual release run

You can also run the workflow manually from GitHub Actions and provide `tag_name`.
The provided tag must already exist and be pushed.

## Produced artifacts

- Linux: `igw_<version>_linux_<arch>.tar.gz`
- macOS: `igw_<version>_darwin_<arch>.tar.gz`
- Windows: `igw_<version>_windows_<arch>.zip`
- Latest alias (Linux): `igw_linux_<arch>.tar.gz`
- Latest alias (macOS): `igw_darwin_<arch>.tar.gz`
- Latest alias (Windows): `igw_windows_<arch>.zip`

Latest aliases are published on each release so host tools can use stable URLs:

- `https://github.com/<owner>/<repo>/releases/latest/download/igw_linux_amd64.tar.gz`

Each archive includes:

- `igw` (or `igw.exe`)
- `LICENSE`
- `README.md`
- `docs/` with command, migration, automation, and catalog guidance

Additional release assets:

- `checksums.txt`
- `release-manifest.json`

## Local v1 qualification

Run the dry-run through the bounded runner on a shared Linux/WSL workstation.
Use an isolated output directory when preserving earlier artifacts. The command
requires all six OS/architecture targets; missing ZIP support is an error rather
than a smaller passing matrix. Python 3 supplies the isolated Linux executable
smoke, which exercises local profiles, migration, errors, and fixture HTTP without
contacting a configured Gateway. Other targets are compiled and packaged; native
execution requires their own qualification.
Each archive is built from a private staging directory and published after it
is complete, so reusing a distribution directory cannot include stale files
from a previous payload tree or ZIP archive.

The current v1 source remains under qualification. A local `v1.0.0` artifact
dry-run verifies format and behavior; it does not publish a release or complete
the remaining Gateway/workflow/performance gates.

The [local cutover evidence](qualification/README.md) retains the passing
six-target dry-run, 33-check packaged smoke, source-input identity, artifact
hashes, and independent stale-file/header checks. Its recorded precommit source
and Linux-only execution scope remain distinct from final release qualification.
