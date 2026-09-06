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

All four cells passed contributor workflow qualification with the previous
parser on 2026-09-05. Candidate `511c5fe` additionally passed 48 and 55 native
journey checks on the two core profiles on 2026-09-06 UTC. These are historical
observations; format conversion and offline tests do not renew them.
Original evidence is [archived in Git](https://github.com/alex-mccollum/igw-cli/blob/65e643d/docs/qualification/workflow-v1/README.md).

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
