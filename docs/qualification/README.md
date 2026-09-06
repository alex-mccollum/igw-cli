# Current qualification status

The simplification implementation is complete; final candidate gates are in
progress. No release or hosted reference-update activation is claimed.

The work replaced the full OpenAPI/YAML model with a direct JSON engine, bounded
the disposable cache, separated two-file references from contributor evidence,
and made discovery compact by default. Profiles, current contract pins,
workflow commands, explicit changes, output envelopes, and exit codes remain.

Completed slices passed the full offline Go suite, 34 executable smoke checks,
docs checks, and update-coordinator tests. The [paired performance record](catalog-json-engine.json)
shows more than 2x faster reference startup and 52-60% lower peak RSS across
four references under the unchanged workstation guard. It identifies its exact
measured binaries; it is not fresh live-Gateway evidence.

Final checks still required: consolidated fixtures, race detector, minimum Go,
performance gates, six release artifacts, and the existing live workflow matrix
if the already-running engine is available. Workstation limits and host lifecycle
restrictions remain unchanged.

Previous qualification packets, failed attempts, and the completed workflow-v1
plan are [archived at 65e643d](https://github.com/alex-mccollum/igw-cli/blob/65e643d/docs/qualification/README.md).
Canonical vendor documents and small behavior fixtures remain in the active
repository. Historical qualification identities and dates remain visible in
reference manifests; reparsing is not renewed live qualification.

The weekly update workflow requires a dedicated runner and repository enablement.
A successful hosted run has not been verified. Embedded references and retained
target snapshots remain available during upstream failures. See
[reference maintenance](../reference-updates.md) and [compatibility scope](../compatibility-matrix.md).
