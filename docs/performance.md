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
| ignition-8.3.0-core | 0.71 / 0.31 s | 146.9 / 67.9 MiB |
| ignition-8.3.0-defaults | 1.18 / 0.51 s | 246.1 / 106.2 MiB |
| ignition-8.3.9-core | 0.73 / 0.33 s | 158.5 / 72.2 MiB |
| ignition-8.3.9-defaults | 1.62 / 0.53 s | 392.6 / 114.3 MiB |

Each row uses five interleaved samples per binary, identical Go 1.27.1 release
build settings (CGO disabled, trimmed paths, stripped symbols), and unchanged
vendor bytes. The final candidate is `a1d2b4b`; the baseline is `511c5fe`.
All operation inventories and raw/document/contract identities match. The
[measurement record](qualification/simplification.json) retains binary
hashes, samples, and inventory checksums. These are local startup observations,
not portable latency guarantees. Live workflow results are recorded separately
in the same candidate record. Historical model-backend measurements remain in
[Git history](https://github.com/alex-mccollum/igw-cli/blob/65e643d/docs/performance.md).

## Unchanged catalog revalidation

Each new process loads and validates its selected target snapshot. A conditional
304 or byte-identical response reuses that invocation's catalog. Reuse preserves
fresh write verification and publishes updated metadata before transferring
ownership. The revalidation benchmark covers load, conditional HTTP, and cache
publication rather than only a warmed lookup.
