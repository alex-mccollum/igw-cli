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

## Host Integration Pattern (RPC Primary, CLI Fallback)

For a stricter host contract (startup checks, feature gating, and fallback rules), see `docs/host-integration.md`.

Shell adapter:

```bash
# 1) Probe protocol/capabilities on startup.
printf '%s\n' \
  '{"id":"h1","op":"hello"}' \
  '{"id":"cap1","op":"capability","args":{"name":"rpcWorkers"}}' \
  '{"id":"s1","op":"shutdown"}' | igw rpc --profile dev

# 2) Prefer rpc for repeated calls.
printf '%s\n' \
  '{"id":"c1","op":"call","args":{"method":"GET","path":"/data/api/v1/gateway-info"}}' \
  '{"id":"s1","op":"shutdown"}' | igw rpc --profile dev --workers 2 --queue-size 64

# 3) Fallback to one-shot call if rpc is unavailable.
igw call --profile dev --path /data/api/v1/gateway-info --json
```

Node.js adapter sketch:

```js
import { spawn } from "node:child_process";

const rpc = spawn("igw", ["rpc", "--profile", "dev", "--workers", "2"]);
rpc.stdin.write('{"id":"h1","op":"hello"}\n');
rpc.stdin.write('{"id":"c1","op":"call","args":{"method":"GET","path":"/data/api/v1/gateway-info"}}\n');
rpc.stdin.write('{"id":"s1","op":"shutdown"}\n');
```

Go adapter sketch:

```go
cmd := exec.Command("igw", "rpc", "--profile", "dev", "--workers", "2")
stdin, _ := cmd.StdinPipe()
stdout, _ := cmd.StdoutPipe()
_ = cmd.Start()
_, _ = io.WriteString(stdin, "{\"id\":\"h1\",\"op\":\"hello\"}\n")
_, _ = io.WriteString(stdin, "{\"id\":\"c1\",\"op\":\"call\",\"args\":{\"method\":\"GET\",\"path\":\"/data/api/v1/gateway-info\"}}\n")
_, _ = io.WriteString(stdin, "{\"id\":\"s1\",\"op\":\"shutdown\"}\n")
_ = stdin.Close()
_ = cmd.Wait()
_ = stdout.Close()
```

## Mutating Operations

Mutating commands require explicit `--yes`:

```bash
igw scan projects --yes
igw scan config --yes
igw tags import --in tags.json --yes --json
igw backup restore --in gateway.gwbk --yes --json
```

For the canonical command reference, see `docs/commands.md`.
