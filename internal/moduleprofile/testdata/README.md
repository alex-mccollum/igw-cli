# Observed OPC UA profiles

These are original fresh disposable-Gateway capture receipts and losslessly
compressed vendor documents, captured serially on 2026-09-05 with
`--modules com.inductiveautomation.opcua`. Both captures passed the existing
resource guard, parser validation, and owned-container cleanup. Independent
Docker queries found no remaining qualification container after either run.

| Gateway | Captured at (UTC) | Operations | Inventory |
| --- | --- | ---: | --- |
| 8.3.0 | 19:09:05 | 446 | One active OPC UA module; 31 inactive/disabled modules |
| 8.3.9 | 19:07:54 | 454 | One active OPC UA module; 31 inactive/disabled modules |

The captures used the contributor executable from the complete clean 8.3.9 run
at source `f5993f97223c35a2ab3fea292cd3e526bcbaf786`. Its SHA-256 is
`19a64a0a2d157dff07b1a159b5e2268888fc82153e51b18d05adad3684169908`.
Both pinned images had passed lifecycle checks with the same acceptance test
binary; the original default-profile run/lifecycle receipts are retained under
the [archived contributor fixtures](../../../docs/qualification/README.md#historical-evidence) (`65e643d:internal/referencebuild/testdata`).

All 32 module records remain present in each receipt. Inactive modules belong
to the vendor's `healthy` collection, with state `INACTIVE`, startup action
`disabled`, and no pending upgrade. These are observed profile states, not
missing modules or proof of operational health. Fixture tests verify the
inventory identity and original raw-document checksum. Keep the original
receipts unchanged.

These captures establish the profile and parser evidence needed to implement
qualification. They do not contain resource, project, tag, or operational
workflow acceptance for this profile and are not qualified reference bundles.
The separate qualified runtime bundles now live in
[reference bundles](../../reference/bundles/README.md). These earlier fixtures
remain focused on module-inventory validation; their capture dates and evidence
are not replaced by subsequent workflow runs.
