# Commands

This file is the canonical command example reference.
For script/agent workflow guidance, see `docs/automation.md`.

Defaults and behavior:
- `igw call` defaults `--method` to `GET` when `--path` is provided.
- `--field` extracts one value from JSON output (requires `--json`), with dot paths and array indexes (`checks.0.name`).
- `igw tags export` defaults `--provider=default` and `--type=json`.
- `igw tags import` defaults `--provider=default`, infers `--type` from `--in` file extension (`.json`, `.xml`, `.csv`; fallback `json`), and defaults `--collision-policy=Abort`.
- `igw logs download`, `igw diagnostics bundle download`, and `igw backup export` default output filenames whenever `--out` is omitted.
- Mutating commands require `--yes`.
- API discovery defaults to `openapi.json` in the current directory, then `${XDG_CONFIG_HOME:-~/.config}/igw/openapi.json`.

Build:

```bash
go build ./cmd/igw
```

Test:

```bash
go test ./...
```

Version:

```bash
igw version
```

API docs discovery:

```bash
igw api list --spec-file /path/to/openapi.json --path-contains gateway
igw api show --spec-file /path/to/openapi.json --path /data/api/v1/gateway-info
igw api show --spec-file /path/to/openapi.json /data/api/v1/gateway-info
igw api search --spec-file /path/to/openapi.json --query scan
```

Generic call:

```bash
igw call \
  --gateway-url http://127.0.0.1:8088 \
  --api-key "$IGNITION_API_TOKEN" \
  --method GET \
  --path /data/api/v1/gateway-info

# Method defaults to GET when omitted.
igw call \
  --gateway-url http://127.0.0.1:8088 \
  --api-key "$IGNITION_API_TOKEN" \
  --path /data/api/v1/gateway-info
```

Call by operationId:

```bash
igw call \
  --gateway-url http://127.0.0.1:8088 \
  --api-key "$IGNITION_API_TOKEN" \
  --spec-file /path/to/openapi.json \
  --op gatewayInfo
```

Mutation safety + automation:

```bash
igw call --method POST --path /data/api/v1/scan/projects --yes
igw call --method POST --path /data/api/v1/scan/projects --dry-run --yes --json
igw call --method GET --path /data/api/v1/gateway-info --retry 2 --retry-backoff 250ms
igw call --method GET --path /data/api/v1/gateway-info --out gateway-info.json
igw call --method GET --path /data/api/v1/gateway-info --json --field response.status
```

Config:

```bash
igw config set --gateway-url http://127.0.0.1:8088
igw config set --auto-gateway
igw config set --api-key-stdin < token.txt
igw config set --gateway-url http://127.0.0.1:8088 --json
igw config show
```

Profiles:

```bash
igw config profile add dev --gateway-url http://127.0.0.1:8088 --api-key-stdin --use
igw config profile add stage --gateway-url http://10.0.1.5:8088 --api-key-stdin
igw config profile add dev --gateway-url http://127.0.0.1:8088 --api-key-stdin --json
igw config profile list
igw config profile use stage
igw config profile use stage --json
```

Profile behavior:
- If there is no active profile yet, the first `config profile add` becomes active automatically.
- If `--profile` is omitted at runtime, the active profile is used when set.

Doctor:

```bash
igw doctor --gateway-url http://127.0.0.1:8088 --api-key "$IGNITION_API_TOKEN"
igw doctor --gateway-url http://127.0.0.1:8088 --api-key "$IGNITION_API_TOKEN" --check-write
igw doctor --gateway-url http://127.0.0.1:8088 --api-key "$IGNITION_API_TOKEN" --json --field checks.0.name
```

Convenience wrappers:

```bash
igw gateway info --profile dev --json
igw scan projects --profile dev --yes
```

Admin wrappers:

```bash
# Logs
igw logs list --profile dev --query limit=5 --json
igw logs download --profile dev --out gateway-logs.zip
# If --out is omitted, defaults to gateway-logs.zip.
igw logs loggers --profile dev --json
igw logs logger set --profile dev --name com.inductiveautomation --level DEBUG --yes --json
igw logs level-reset --profile dev --yes --json

# Diagnostics bundle
igw diagnostics bundle generate --profile dev --yes --json
igw diagnostics bundle status --profile dev --json
igw diagnostics bundle download --profile dev --out diagnostics.zip
# If --out is omitted, defaults to diagnostics.zip.

# Backups
igw backup export --profile dev --out gateway.gwbk
# If --out is omitted, defaults to gateway.gwbk.
igw backup restore --profile dev --in gateway.gwbk --yes --json

# Tags
igw tags export --profile dev --out tags.json
igw tags import --profile dev --in tags.json --yes --json
igw tags import --profile dev --in tags.json --collision-policy Overwrite --yes --json

# Restart
igw restart tasks --profile dev --json
igw restart gateway --profile dev --yes --json
```

Shell completion:

```bash
source <(igw completion bash)
```

Smoke test script:

```bash
IGW_PROFILE=dev ./scripts/smoke.sh
```
