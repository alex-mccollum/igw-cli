# Captured Gateway contracts

These fixtures contain the original, vendor-generated `/openapi.json` from
disposable official Ignition images. `capture.json` identifies the pinned image,
running Gateway version, module whitelist (omitted means image defaults), raw
and canonical hashes, and parser qualification. Gzip preserves the exact JSON
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

Fixture updates must retain the original bytes, image/version provenance, and
checksums. Review new captures before replacing a fixture; see the qualification
rules in [catalog docs](../../../docs/catalog.md).
