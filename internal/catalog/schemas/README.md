# Embedded OpenAPI metaschemas

`oas3-schema.json` and `oas31-schema.json` are copied unchanged from
`github.com/pb33f/libopenapi` v0.38.7, `datamodel/schemas/`. Its MIT license is
retained here. These are the same document schemas used before the JSON engine
replacement, preserving the admission baseline while removing the model
library dependency. Runtime schema loading is disabled; updates are explicit,
reviewed dependency changes with document-validation regression checks.
