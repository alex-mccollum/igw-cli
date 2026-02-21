# Examples

All commands below are examples. Replace placeholders for your environment.

## Health and Connectivity

```bash
igw doctor
igw doctor --check-write
igw doctor --json --select ok --raw
```

## Gateway Metadata

```bash
igw gateway info --json
igw call --path /data/api/v1/gateway-info --json
```

## API Discovery

```bash
igw api list --path-contains gateway
igw api show /data/api/v1/gateway-info
igw api tags --json
igw api stats --json
igw api sync --json
```

## Wrapper Operations

```bash
igw logs list --query limit=5 --json
igw diagnostics bundle status --json
igw restart tasks --json
```

## Mutating Operations

Mutating commands require explicit `--yes`:

```bash
igw scan projects --yes
igw tags import --in tags.json --yes --json
igw backup restore --in gateway.gwbk --yes --json
```

For the canonical command reference, see `docs/commands.md`.
