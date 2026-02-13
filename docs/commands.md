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

Config:

```bash
igw config set --gateway-url http://127.0.0.1:8088
igw config set --auto-gateway
igw config set --api-key-stdin < token.txt
igw config show
```

Doctor:

```bash
igw doctor --gateway-url http://127.0.0.1:8088 --api-key "$IGNITION_API_TOKEN"
```
