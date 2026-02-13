# Commands

Build:

```bash
go build ./cmd/igw
```

Test:

```bash
go test ./...
```

Generic call:

```bash
igw call \
  --gateway-url http://127.0.0.1:8088 \
  --api-key "$IGNITION_API_TOKEN" \
  --method GET \
  --path /data/api/v1/gateway-info
```

Config:

```bash
igw config set --gateway-url http://127.0.0.1:8088
igw config set --api-key-stdin < token.txt
igw config show
```

Doctor:

```bash
igw doctor --gateway-url http://127.0.0.1:8088 --api-key "$IGNITION_API_TOKEN"
```
