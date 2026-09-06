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
