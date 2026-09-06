# Bundled API references

Each selector contains `reference.json` and the exact vendor document compressed
as `openapi.json.gz`. Runtime format is `igw/reference/v2`. Four selectors cover
8.3.0 and 8.3.9 with core and image-default module profiles.

Current catalog identity is separate from original live qualification identity,
parser, date, and scope. The manifest points to the original evidence by URI and
SHA-256. Historical packets remain in Git at `65e643d`; format conversion does
not renew their qualification. New packets are assembled by `igw-capture qualify`
and retained separately from the runtime payload.

See [reference maintenance](../../../docs/reference-updates.md) and
[the catalog contract](../../../docs/catalog.md). Review changes against the same
module profile. Preserve vendor license metadata in the original document.
