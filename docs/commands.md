# Commands

Build:

```bash
go build ./cmd/igw
```

Test:

```bash
go test ./...
```

Example call:

```bash
igw call \
  --gateway-url http://127.0.0.1:8088 \
  --api-key "$IGNITION_API_TOKEN" \
  --method GET \
  --path /data/api/v1/gateway-info
```
