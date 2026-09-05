# Qualified reference bundles

These directories preserve exact compressed vendor OpenAPI documents, registry
manifests, capture and acceptance receipts, and a checksummed reference manifest.
The source image, complete observed module inventory, parser, test executable,
and exercised workflows are identified. No customer configuration or credentials
are included.

| Selector | Operations | Active / installed modules | Workflow policy | Recorded checks |
| --- | ---: | ---: | --- | ---: |
| ignition-8.3.0-defaults | 672 | 32 / 32 | 2 | 87 |
| ignition-8.3.0-core | 446 | 1 / 32 | 2 | 87 |
| ignition-8.3.9-defaults | 687 | 32 / 32 | 1 | 98 |
| ignition-8.3.9-core | 454 | 1 / 32 | 2 | 99 |

The original `ignition-8.3.9-defaults` bundle remains byte-for-byte unchanged,
including its historical policy, identity, timestamps, and receipts. A separate
new default-profile qualification under policy 2 is retained with contributor
evidence. The other three embedded bundles are exact copies of reviewed
candidates under `internal/referencebuild/testdata/`. Core references record the
explicit OPC UA selection and retain all 31 inactive/disabled module records.
Historical manifests without a named profile retain their all-active rule.

8.3.0 does not advertise tag import/export. Its qualification requires explicit
pre-dispatch refusals and lists tag round trips as unavailable. 8.3.9 includes
live tag round trips. See `docs/compatibility-matrix.md` for the complete initial
version/profile matrix. These references qualify the named workflow scopes,
not every operation, request schema, deployment, or cross-version migration.

Keep previous qualified bundles available when an update fails. Adding a
selector does not select a default or authorize live Gateway writes. Checksums
establish integrity; the repository/release channel providing a bundle remains
the trust source. Comparisons against a different module set require review;
removed module routes do not alone establish a version regression. Preserve
vendor license metadata in the exact original document.
