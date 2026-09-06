# Performance

Run `bash scripts/perf-gate.sh` through the bounded runner. It checks real catalog
opening, command discovery, typed loopback requests, loaded operation lookup,
conditional revalidation, and 32 MiB streamed artifacts. Each metric must be
present and within its budget. Go B/op is cumulative allocation, not peak RSS.

## JSON contract engine

The JSON engine removes the full OpenAPI model, YAML rendering, duplicate
operation metadata decoding, and repeated reference verification. Document and
contract hashing avoid extra encoded-document copies. A paired process run with
Go 1.27.1 under the unchanged 8 GiB/two-CPU guard observed:

| Reference | Startup before / after | Peak RSS before / after |
| --- | --- | --- |
| ignition-8.3.0-core | 0.69 / 0.31 s | 148.5 / 71.5 MiB |
| ignition-8.3.0-defaults | 1.12 / 0.49 s | 248.6 / 108.7 MiB |
| ignition-8.3.9-core | 0.76 / 0.33 s | 161.0 / 77.8 MiB |
| ignition-8.3.9-defaults | 1.63 / 0.80 s | 399.2 / 158.9 MiB |

Each row uses five interleaved samples per binary and unchanged reference bytes.
All operation inventories and raw/document/contract identities match. The
[measurement record](qualification/catalog-json-engine.json) retains binary
hashes, samples, and inventory checksums. These are local startup observations,
not portable latency guarantees or fresh live-Gateway qualification. Historical model-backend measurements remain in
[Git history](https://github.com/alex-mccollum/igw-cli/blob/65e643d/docs/performance.md).

## Unchanged catalog revalidation

Each new process loads and validates its selected target snapshot. A conditional
304 or byte-identical response reuses that invocation's catalog. Reuse preserves
fresh write verification and publishes updated metadata before transferring
ownership. The revalidation benchmark covers load, conditional HTTP, and cache
publication rather than only a warmed lookup.
