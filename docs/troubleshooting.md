# Troubleshooting

## Start with `doctor`

```bash
igw doctor
igw doctor --check-write
igw doctor --json
```

## Common Errors

### `401 Unauthorized`
- Token missing or invalid.
- Re-check `IGNITION_API_TOKEN`, profile token, or `--api-key`.

### `403 Forbidden`
- Token is valid but lacks required permissions/security-level mapping.
- Verify gateway permission mapping for the token/user.

### Network timeout / transport failures
- Confirm host and port reachability.
- If using WSL2 to a Windows-host gateway, verify firewall inbound rules for port `8088`.

### OpenAPI lookup failures (`api` commands or `call --op`)
- Run `igw api sync --json` explicitly.
- Override endpoint path when needed: `igw api sync --openapi-path /openapi.json --json`.
- Or pass a local spec directly: `--spec-file /path/to/openapi.json`.

## Helpful Inspection Commands

```bash
igw config show
igw api refresh --json --select sourceURL --raw
igw call --path /data/api/v1/gateway-info --json --select response.status --raw
```
