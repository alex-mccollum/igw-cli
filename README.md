# igw

`igw` is a lightweight CLI wrapper for the Ignition Gateway API.

## Principles
- Standard library only for MVP.
- Generic API execution first (`call`).
- Stable exit codes for automation.

## Commands
- `igw api list|show|search`: query local OpenAPI docs for endpoint discovery.
- `igw call`: generic HTTP executor for Ignition endpoints (or `--op` by operationId).
- `igw config set|show|profile`: local config + profile management.
- `igw doctor`: connectivity + auth checks (URL, TCP, read access, write access).
- `igw gateway info`: convenience read wrapper.
- `igw scan projects`: convenience write wrapper.

## Configuration Sources
Precedence is strict:
1. CLI flags
2. Environment variables
3. Config file

Environment variables:
- `IGNITION_GATEWAY_URL`
- `IGNITION_API_TOKEN`

Config file path:
- `${XDG_CONFIG_HOME:-~/.config}/igw/config.json` on Linux/macOS

## Examples
List endpoints from a local OpenAPI snapshot:

```bash
igw api list --spec-file ../autoperspective/openapi.json --path-contains gateway
```

Show one endpoint contract:

```bash
igw api show \
  --spec-file ../autoperspective/openapi.json \
  --path /data/api/v1/gateway-info
```

Search endpoint docs:

```bash
igw api search --spec-file ../autoperspective/openapi.json --query scan
```

Call gateway info:

```bash
igw call \
  --gateway-url http://127.0.0.1:8088 \
  --api-key "$IGNITION_API_TOKEN" \
  --method GET \
  --path /data/api/v1/gateway-info
```

Call with JSON output envelope:

```bash
igw call \
  --gateway-url http://127.0.0.1:8088 \
  --api-key "$IGNITION_API_TOKEN" \
  --method POST \
  --path /data/api/v1/scan/projects \
  --json
```

Set config once:

```bash
igw config set --gateway-url http://127.0.0.1:8088
igw config set --api-key-stdin < token.txt
# WSL helper:
igw config set --auto-gateway
```

Manage profiles:

```bash
igw config profile add dev --gateway-url http://127.0.0.1:8088 --api-key-stdin --use
igw config profile add prod --gateway-url http://10.10.0.5:8088 --api-key-stdin
igw config profile list
igw config profile use prod
```

Check config (token is masked):

```bash
igw config show
```

Run connectivity/auth checks:

```bash
igw doctor --gateway-url http://127.0.0.1:8088 --api-key "$IGNITION_API_TOKEN"
```

Use convenience wrappers:

```bash
igw gateway info --profile dev --json
igw scan projects --profile dev --yes
```

Enable bash completion:

```bash
source <(igw completion bash)
```

Run end-to-end smoke checks:

```bash
IGW_PROFILE=dev ./scripts/smoke.sh
```

Call by operationId from local spec:

```bash
igw call \
  --spec-file ../autoperspective/openapi.json \
  --op gatewayInfo \
  --json
```

Write safety + automation flags:

```bash
# dry-run helper (adds query dryRun=true)
igw call --method POST --path /data/api/v1/scan/projects --dry-run --yes --json

# retries (idempotent methods only)
igw call --method GET --path /data/api/v1/gateway-info --retry 2 --retry-backoff 250ms --json

# write response body to file
igw call --method GET --path /data/api/v1/gateway-info --out gateway-info.json
```

## Exit Codes
- `0`: success (`2xx`)
- `2`: usage/config errors
- `6`: auth failures (`401`, `403`)
- `7`: network/transport and non-auth HTTP failures

## Build

```bash
go build -trimpath -ldflags="-s -w" -o bin/igw ./cmd/igw
```

## Test

```bash
go test ./...
```
