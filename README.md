# igw

`igw` lets people and shell agents discover and operate an Ignition 8.3+
Gateway: inspect its actual API contract, preview a change, apply it explicitly,
and verify the outcome.

The CLI uses one `cmd/igw` entrypoint. See the
[current qualification status](docs/qualification/README.md) for verified scope.
Existing 0.x users should read the [migration guide](docs/migration-v1.md).

## Start here

Build from this checkout with the [workstation safeguards](docs/development-safety.md):

```bash
bash scripts/bounded-run.sh -- go build -o bin/igw ./cmd/igw
```

The following examples assume `igw` is on PATH; use `bin/igw` for that local
build. To install a published version, see [installation](docs/installation.md).
No v1 tag or release is published by the local rebuild process.

```bash
igw version
igw schema --json
igw profile set dev --url http://127.0.0.1:8088 --use --dry-run --json
igw profile set dev --url http://127.0.0.1:8088 --use --yes --json
igw profile set dev --token-stdin --yes --json < private-token.txt
igw gateway doctor --json
igw api list --search gateway --json
igw api describe 'GET /data/api/v1/gateway-info' --json
```

Use the full API token (`name:key`), including its name and colon. Tokens can
also come from `IGNITION_API_TOKEN`; the target can come from
`IGNITION_GATEWAY_URL`. Per-field precedence remains flags > environment >
configuration. [Profiles](docs/profiles.md) provide explicit migration and
rollback while preserving existing settings.

## Discover, preview, apply, verify

Use `api list` and `api describe` to learn what the selected Gateway advertises.
Resource, project, tag, and operational commands share a typed execution core
and add workflow-specific checks. Every mutation requires `--yes`; previews
send no proposed mutation. Replacement/deletion requires supported reviewed
preconditions, such as resource signatures.

`--json` emits one `igw/v1` result, including errors. Read `outcome` and
`meta.verification` as well as `ok`: an accepted generic request does not prove
its final state, and a disconnected write can be uncertain. Exit codes remain
0 (success), 2 (usage/configuration), 6 (auth/permission), and 7 (transport,
non-auth HTTP, artifact, or verification failure).

The target's OpenAPI document supplies its documented wire contract. The CLI
preserves exact vendor bytes with provenance, bounded snapshots, freshness,
pins, and explicit offline references. Bundled references remain available
without connectivity and do not silently authorize writes to another Gateway.
See the [catalog design](docs/catalog.md) and [update pipeline](docs/reference-updates.md).

## Documentation

- [Commands](docs/commands.md): canonical examples and input contracts.
- [Automation](docs/automation.md): result handling, previews, batches, and artifacts.
- [Migration](docs/migration-v1.md): 0.x command changes and configuration recovery.
- [Compatibility](docs/compatibility-matrix.md): actual Gateway evidence and limits.
- [Architecture](docs/architecture.md): package boundaries and design decisions.
- [Troubleshooting](docs/troubleshooting.md): configuration, API, and transport failures.
- [Releasing](docs/releasing.md): version, artifact, and verification contracts.
- [Qualification](docs/qualification/README.md): current checks and remaining limits.

Run contributor tests through the bounded runner, one job at a time:

```bash
bash scripts/bounded-run.sh -- go test ./...
bash scripts/bounded-run.sh -- bash scripts/smoke.sh
```

The smoke script separates local executable checks from explicit live reads.
Real Gateway qualification uses owned disposable containers with independently
verified limits and cleanup. Repository work never controls WSL or Docker
Desktop lifecycle or alters host memory settings.
