# Releasing

This project uses a lightweight tag-based release flow.

## Create a release

1. Ensure `main` is green:
   - `go test ./...`
   - `go build ./cmd/igw`
2. Create and push a semantic version tag:

```bash
git tag v0.1.0
git push origin v0.1.0
```

3. GitHub Actions `release.yml` builds cross-platform artifacts and publishes a GitHub Release with generated notes.

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
