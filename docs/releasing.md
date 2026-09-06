# Releasing

Releases use an explicit `vMAJOR.MINOR.PATCH` tag. The working v1 source is not
published by a local build or dry-run. Check [qualification status](qualification/README.md)
for the exact tested source and remaining live or platform checks before choosing
a release candidate. Earlier passing evidence does not qualify changed code.

## Prepare and verify

Use a clean candidate checkout and a new, unused tag. The examples use
`v1.0.0` as a prospective tag, not a claim that this release exists. Add the
matching `## [v1.0.0]` changelog heading and release notes before cutting it.
Require the candidate's CI, relevant live workflow qualification, and platform
checks to pass; the release workflow does not perform live Gateway qualification.

On shared Linux/WSL, use the [bounded runner](development-safety.md) for local
build/test/release commands. Run one job at a time. The local dry-run/cut helpers
require a Linux amd64 host because they execute the packaged Linux binary. They
also require Go, Python 3, Git, Bash, tar, and zip. Use a fresh distribution
directory to keep previous evidence separate:

```bash
DIST_DIR=bin/release-candidate bash scripts/bounded-run.sh -- bash scripts/release/dry-run.sh v1.0.0
```

The dry-run checks command docs and links, runs the Go suite, and builds Linux,
macOS, and Windows artifacts for amd64 and arm64. It requires all six targets;
missing ZIP support fails the run. It executes version and isolated smoke checks
on packaged Linux amd64 only, then creates latest aliases, checksums, and the
release manifest. Other targets require their own native qualification.
A dry-run permits a dirty checkout and labels its build metadata accordingly;
that result is not a clean release candidate. It creates no tag and publishes
nothing. It does not run the race detector, performance gate, or live suites.

Each archive is prepared in a private staging directory and published only when
complete. Reusing a distribution directory does not merge stale payload files
into a newly built archive, but a new directory keeps different releases' assets
and checksums unambiguous.

## Cut and publish

Publication is maintainer-managed. Agents stop after local preparation unless
the user explicitly delegates publication; a completed task or pre-push review
does not authorize authentication setup, pushing, or release creation.

For a maintainer explicitly initiating publication of the selected candidate:

```bash
bash scripts/bounded-run.sh -- bash scripts/release/cut.sh v1.0.0
```

On an isolated Linux amd64 release host, invoke the underlying script directly
if the workstation guard does not apply. `cut.sh` requires a clean working tree
and the matching changelog heading, runs the dry-run, creates the tag or verifies
it points to HEAD, runs the release checklist, then pushes HEAD and that tag to
`origin`. This command publishes Git refs; it is not another local check.

GitHub's [release workflow](../.github/workflows/release.yml) checks tag and
changelog metadata, verifies the tag is on `origin`, builds from the tag, verifies
the packaged Linux amd64 executable, and publishes all archives and metadata.
Manual workflow dispatch accepts `tag_name`, which must already exist on the
remote. Keep `main` CI green before dispatch; publication does not replace it.

Optional repository hooks add tag checks to manual pushes:

```bash
bash scripts/install-git-hooks.sh
```

The hook runs `scripts/release/checklist.sh` for pushed semver tags. The checklist
checks changelog/tag integrity and normally probes push authentication with
`git push --dry-run --no-verify`. Calls from `cut.sh` and the hook skip those
probes to avoid repeated authentication prompts and hook recursion.

If a published tag fails, fix the source and prefer a new patch tag. Do not
force-move an existing release tag as routine recovery.

## Artifacts and integrity

| Platform | Archive | Latest alias |
| --- | --- | --- |
| Linux | `igw_<version>_linux_<arch>.tar.gz` | `igw_linux_<arch>.tar.gz` |
| macOS | `igw_<version>_darwin_<arch>.tar.gz` | `igw_darwin_<arch>.tar.gz` |
| Windows | `igw_<version>_windows_<arch>.zip` | `igw_windows_<arch>.zip` |

`<version>` includes the `v` prefix; `<arch>` is `amd64` or `arm64`. Each archive
contains a matching top-level directory with the executable, LICENSE, README,
and `docs/`. Additional assets are `checksums.txt` and `release-manifest.json`.
The manifest has top-level `version` and `artifacts[]` entries with `name`, `os`,
`arch`, `archive`, `sha256`, and `url`. Latest aliases use stable filenames under
`https://github.com/alex-mccollum/igw-cli/releases/latest/download/`; the release
selected by `latest` can change, so use explicit tags for reproducibility.

Verify a downloaded Linux artifact against that release's checksum manifest:

```bash
ARCHIVE="igw_v1.0.0_linux_amd64.tar.gz"
grep "  ${ARCHIVE}$" checksums.txt | sha256sum -c -
```

After extraction, confirm the installed version. All three human version forms
(`igw version`, `igw --version`, and `igw -v`) start with `igw version <tag>`.
Release builds can append commit and date, for example
`igw version v1.0.0 (abc1234, 2026-09-06)`. The packaged Linux check uses
`scripts/check-version-contract.sh BINARY TAG` to enforce this contract.
See [installation](installation.md) for installation options and
[commands](commands.md) for an optional read-only Gateway check.
