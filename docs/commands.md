# Commands

Build:

```bash
go build ./cmd/igw
```

Test:

```bash
go test ./...
```

API docs discovery:

```bash
igw api list --spec-file ../autoperspective/openapi.json --path-contains gateway
igw api show --spec-file ../autoperspective/openapi.json --path /data/api/v1/gateway-info
igw api search --spec-file ../autoperspective/openapi.json --query scan
```

Generic call:

```bash
igw call \
  --gateway-url http://127.0.0.1:8088 \
  --api-key "$IGNITION_API_TOKEN" \
  --method GET \
  --path /data/api/v1/gateway-info
```

Call by operationId:

```bash
igw call \
  --gateway-url http://127.0.0.1:8088 \
  --api-key "$IGNITION_API_TOKEN" \
  --spec-file ../autoperspective/openapi.json \
  --op gatewayInfo
```

Mutation safety + automation:

```bash
igw call --method POST --path /data/api/v1/scan/projects --yes
igw call --method POST --path /data/api/v1/scan/projects --dry-run --yes --json
igw call --method GET --path /data/api/v1/gateway-info --retry 2 --retry-backoff 250ms
igw call --method GET --path /data/api/v1/gateway-info --out gateway-info.json
```

Config:

```bash
igw config set --gateway-url http://127.0.0.1:8088
igw config set --auto-gateway
igw config set --api-key-stdin < token.txt
igw config show
```

Profiles:

```bash
igw config profile add dev --gateway-url http://127.0.0.1:8088 --api-key-stdin --use
igw config profile add stage --gateway-url http://10.0.1.5:8088 --api-key-stdin
igw config profile list
igw config profile use stage
```

Doctor:

```bash
igw doctor --gateway-url http://127.0.0.1:8088 --api-key "$IGNITION_API_TOKEN"
```

Convenience wrappers:

```bash
igw gateway info --profile dev --json
igw scan projects --profile dev --yes
```
