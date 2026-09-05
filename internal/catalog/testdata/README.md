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
Current policy `ignition-openapi/5` also qualifies 288 duplicate unused settings
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
captures use version 3 receipts with distinct document and contract hashes and
observed module inventory. Version 2 and 3 historical hashes are verified with
their recorded policy; current qualification never relabels those receipts.

Fixture updates must retain the original bytes, image/version provenance, and
checksums. Review new captures before replacing a fixture; see the qualification
rules in [catalog docs](../../../docs/catalog.md).

`legacy-keyboard-config.json` is an extracted schema, not a qualified Gateway
capture. It comes from the `config` property of the POST
`/data/api/v1/resources/ignition/keyboard_layout` request's array items in the
historical 8.3.0 OPC UA capture. Original document SHA-256 (raw bytes) is
`de174add02ef1557f603edae7c7802ea298bce35301b9675b5b620f4247b2e9c`.
The schema retains vendor annotations, examples, and constraints; only JSON
formatting changed during extraction. The source identifies the Inductive
Automation EULA at `/res/sys/license.html`. Tests use reduced surrounding
operations to verify the four observed embedding positions, strict expansion
of local definitions, preserved key constraints, and rejection of unreviewed
reference scopes. Expanding this schema matches the corresponding retained
8.3.9 schema exactly. This extraction does not establish live 8.3.0 acceptance.

`ignition-8.3.0-defaults` retains the exact fresh default-module document captured
on 2026-09-05, with 32 observed modules and successful owned-container cleanup.
The original `capture.json` records parser version 10's validation failure;
`capture-run.json` records the stopped coordinator, and `lifecycle.json` records
the passing containment probe. These are immutable historical receipts. Current
`qualification.json` records the current successful 672-operation parser result
after review of EAM/SFC parameter defects. Tests check both the original raw hash
and all current-parser identities and adjustments. The failed capture must not
be relabeled as live workflow acceptance or shipped as a qualified reference.

`ignition-8.3.0-defaults-repeat` retains the second clean coordinator capture
from `40a8502`. Its original parser-11 receipt passed; the subsequent transfer
suite failed because the Gateway lacks tag import/export. Complete run evidence
is retained in `internal/referencebuild/testdata/ignition-8.3.0-incomplete/`.
These two minimum-version documents differ only in schema array order and
example timestamps. Contract policy 2 uses the reviewed keyboard reference
scopes so both current hashes agree; policy 1's differing original hashes still
verify. Current parser qualification does not imply live workflow acceptance.
