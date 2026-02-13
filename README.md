# igw

`igw` is a lightweight CLI wrapper for the Ignition Gateway API.

## Principles
- Standard library only for MVP.
- Generic API execution first (`call`).
- Stable exit codes for automation.

## Commands
- `igw api list|show|search`: query local OpenAPI docs for endpoint discovery.
- `igw call`: generic HTTP executor for Ignition endpoints.
- `igw config set|show`: local config management.
- `igw doctor`: connectivity + auth checks.

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
```

Check config (token is masked):

```bash
igw config show
```

Run connectivity/auth checks:

```bash
igw doctor --gateway-url http://127.0.0.1:8088 --api-key "$IGNITION_API_TOKEN"
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
