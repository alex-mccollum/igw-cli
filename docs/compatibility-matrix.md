# Gateway compatibility

The live target's document determines available operations. A version label
alone does not establish module support, permissions, or verified behavior.
See [current source qualification](qualification/README.md) before treating an
older workflow pass as qualification of this implementation.

| Reference | Modules | Operations | Tag routes |
| --- | --- | ---: | --- |
| 8.3.0 core | 1 active, 31 inactive | 446 | Absent |
| 8.3.0 defaults | 32 active | 672 | Absent |
| 8.3.9 core | 1 active, 31 inactive | 454 | Advertised |
| 8.3.9 defaults | 32 active | 687 | Advertised |

Candidate `a1d2b4b` passed the resource, project/tag, operational, and native CLI
journey suites on all four cells on 2026-09-06 UTC. Containment and cleanup were
verified separately for each cell. The [current qualification record](qualification/README.md)
identifies the exact source, binaries, and covered checks. The built-in manifests
retain their original historical qualification dates and identities.

Known boundaries remain explicit:

- 8.3.0 lacks the tag transfer APIs; refusal is tested, not counted as a round trip.
- Singleton metadata update and delete are verified. Configuration creation can
  be uncertain when readback cannot establish equality. Metadata-only creation
  is rejected; complete translations creation is not qualified.
- Diagnostics correlates the latest job using the available size/status fields;
  the API supplies no request-specific job identifier.
- Generic requests report acceptance. Workflows report completion only when
  their own observations establish the result; disconnected writes can remain
  uncertain and are never replayed automatically.

## Singletons

Fixture tests cover the 17 singleton types advertised by each retained default
catalog. On the pinned 8.3.0 and 8.3.9 core-opcua Gateways, translations metadata
update, stale-signature refusal, and deletion were verified. Creation with
`config` was acknowledged but both workflow and independent readback omitted
that field; the CLI reported `uncertain` and exit 7. Metadata-only creation was
rejected. These observations do not prove whether configuration was stored or
applied, and do not qualify complete translations creation or other singleton
types. Preserve the outcome and inspect state before another mutation.

Original attempt receipts are retained privately under the
[history cleanup policy](qualification/history-cleanup.md). These observations
are historical evidence, not a current-source pass.

## Additional request-body qualification

The privately retained historical body-input packet records 31-check 8.3.0 and
62-check 8.3.9 core-profile runs from source `03566a8`,
with lifecycle and cleanup evidence. The 8.3.9 run verified repeated `files`
parts and explicit filenames on the translations bulk-datafile route through
independent byte-for-byte downloads, stale-signature refusal, overwrite, and
file deletion. The singleton already existed, so its optional creation branch
was not qualified. The 8.3.0 catalog lacks that bulk route.

The vendor declares no part schema for that route. This establishes the tested
recipe's historical behavior, not exclusive part names or support for every
multipart endpoint. Current schema-bearing multipart requests remain refused.
The original transcript-test source remains in rewritten Git history; its local
execution packet is covered by the [privacy cleanup](qualification/history-cleanup.md).
Current binding tests use canonical vendor documents and focused fixtures. See
[current qualification status](qualification/README.md) for the source covered
by the latest live matrix; it does not turn this older packet into a new pass.
