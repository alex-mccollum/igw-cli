# Qualified reference bundles

These directories preserve exact compressed vendor OpenAPI documents, registry
manifests, capture and acceptance receipts, and a checksummed reference manifest.
The source image, actual module inventory, parser, test executable, and exercised
workflows are identified. No customer configuration or credentials are included.

`ignition-8.3.9-defaults` was assembled from a fresh 32-module default Gateway
capture and 98 recorded checks: 10 lifecycle, 27 resource, 38 project/tag, and 23
operational checks. Its stable contract has 687 operations. The versioned
qualification policy covers the named workflow scopes; it does not qualify every
operation/request schema, deployment, module configuration, or Gateway version.

The lifecycle and workflow receipts all identify test executable
`63aaa19c9a60f3872419f3cadbd75102e21f440b0863f72d32fb7f67c560a800`.
The module inventory hash is
`8adf3d3f453ec516a9d29976c94cfb2696c89b85b1f05b35be4c4bc71392d4dd`.
The manifest's comparison is against the previously qualified default fixture;
equal contract hashes are not a proof of general backward compatibility.

Keep previous qualified bundles available when an update fails. Candidate
assembly is local and does not publish, select a new default, or authorize live
Gateway writes. Checksums establish integrity; the repository/release channel
providing a bundle remains the trust source. Preserve vendor license metadata
in the exact original document.
