# Bundled API references

Each selector contains `reference.json` and the exact vendor document compressed
as `openapi.json.gz`. Runtime format is `igw/reference/v2`. Four selectors cover
8.3.0 and 8.3.9 with core and image-default module profiles.

Current catalog identity is separate from original live qualification identity,
parser, date, and scope. The manifest points to the original evidence by URI and
SHA-256. Original reference manifests and their capture files remain in Git at
`c08fe22`; private execution logs follow the
[history cleanup policy](../../../docs/qualification/history-cleanup.md). Format
conversion does not renew qualification. New packets are assembled by
`igw-capture qualify` and retained separately from the runtime payload.

The four `capturedAt` values were recovered from their original `capture.json`
receipts at `c08fe22`, after checking the archived manifest and capture file
SHA-256 values and matching image/raw-document identity. Assembly dates,
qualification identities, evidence hashes, and compressed payloads are unchanged.

See [reference maintenance](../../../docs/reference-updates.md) and
[the catalog contract](../../../docs/catalog.md). Review changes against the same
module profile. Preserve vendor license metadata in the original document.
