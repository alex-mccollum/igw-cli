# Configuration

## Precedence

Runtime values resolve in strict order:
1. CLI flags
2. Environment variables
3. Config file

## Environment Variables

- `IGNITION_GATEWAY_URL`
- `IGNITION_API_TOKEN`

## Config File Location

- Linux/macOS: `${XDG_CONFIG_HOME:-~/.config}/igw/config.json`
- Windows: `%AppData%\\igw\\config.json`

## Basic Setup

```bash
igw config set --gateway-url http://127.0.0.1:8088
igw config set --api-key-stdin < token.txt
igw config show
```

## Profiles

Use profiles when you target multiple gateways.

```bash
igw config profile add dev --gateway-url http://127.0.0.1:8088 --api-key-stdin --use
igw config profile list
igw config profile use dev
```

Notes:
- First added profile becomes active when no active profile exists.
- If `--profile` is omitted at runtime, the active profile is used (when set).

## WSL Helper

If Ignition runs on Windows host from WSL:

```bash
igw config set --auto-gateway
```
