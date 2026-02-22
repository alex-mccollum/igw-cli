# Releasing

This project uses a lightweight tag-based release flow.

## Create a release

1. Ensure `main` is green:
   - `go test ./...`
   - `go build ./cmd/igw`
   - `./scripts/check-command-docs.sh`
   - `./scripts/lint-docs.sh`
2. Create and push a semantic version tag:

```bash
git tag v0.1.0
git push origin v0.1.0
```

3. GitHub Actions `release.yml` builds cross-platform artifacts and publishes a GitHub Release with generated notes.

## Version contract

- Release artifacts must print the release tag in `igw version` output.
- CI enforces this contract for the Linux `amd64` artifact with:

```bash
./scripts/check-version-contract.sh <binary-path> <tag>
```

- The check validates output starts with `igw version <tag>`.
- Release builds include commit/date metadata, so full output may be:
  - `igw version v0.3.1 (abc1234, 2026-02-22)`

## Post-Release Smoke Check

After publishing, validate one installed artifact:

```bash
igw version
igw gateway info --gateway-url http://127.0.0.1:8088 --api-key "$IGNITION_API_TOKEN" --json
```

## Manual release run

You can also run the workflow manually from GitHub Actions and provide `tag_name`.

## Produced artifacts

- Linux: `igw_<version>_linux_<arch>.tar.gz`
- macOS: `igw_<version>_darwin_<arch>.tar.gz`
- Windows: `igw_<version>_windows_<arch>.zip`

Each archive includes:

- `igw` (or `igw.exe`)
- `LICENSE`
- `README.md`
