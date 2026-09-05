# Captured Gateway contracts

These fixtures contain the original, vendor-generated `/openapi.json` from
disposable official Ignition images. `capture.json` identifies the pinned image,
running Gateway version, module whitelist (omitted means image defaults), raw
and capture-time hashes, and parser qualification. Gzip preserves the exact JSON
bytes while avoiding large repetitive files in Git.

The JSON retains Inductive Automation's license and attribution metadata.
These are reference contracts, not a customer's configuration and not write
authority for a different Gateway. Parser compatibility policy is reported in
each receipt; `validated` means the adapted model was validated, not that the
original document is fully compliant with OpenAPI.

`ignition-8.3.9-defaults` was captured on 2026-09-05. The original has 343 path
parameters with `allowReserved: false` and seven empty response objects.
The adapter omits that inapplicable path annotation and represents an unknown
response internally without inventing status codes or payload constraints.
Current policy `ignition-openapi/3` also qualifies 288 duplicate unused settings
identifiers across mirrored resource request variants. Reference-bearing or
otherwise unreviewed variants remain schema errors. The current qualification
records 638 total adjustments without changing the historical capture receipt.

`ignition-8.3.9-defaults-repeat` retains a second capture from a fresh container
using the same image and modules. Its 56 reordered `oneOf` arrays, four reordered
`enum` arrays, and two changing example timestamps must not change the contract
hash. Raw and canonical document hashes still expose these differences.

Historical version 1 `capture.json` receipts used the canonical document hash
as `contractSha256`. Keep those receipts unchanged. A separate
`qualification.json` records current parser/policy versions, all three
identities, operation count, and compatibility totals for each capture. New
captures use version 2 receipts with distinct document and contract hashes.

Fixture updates must retain the original bytes, image/version provenance, and
checksums. Review new captures before replacing a fixture; see the qualification
rules in [catalog docs](../../../docs/catalog.md).
