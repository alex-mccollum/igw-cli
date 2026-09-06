# Historical execution-record privacy cleanup

The unpublished history was rewritten on 2026-09-06 to remove 422 obsolete
local qualification files, including execution logs, machine-path receipts,
and duplicate run artifacts. These paths were absent from both the published
base and the current tree. The removal changed no current application, build,
configuration, vendor-document, or behavior-fixture bytes.

The published base and release history remain unchanged. Historical source IDs
inside qualification records and executable metadata retain their original
meaning: the cleanup does not regenerate or renew qualification. The
[commit map](history-map.json) locates their rewritten counterparts, including
the subsequent reference-baseline fix and documentation audit. Its head pair
marks the boundary after replaying that work with identical trees and before
this separate privacy metadata repair. Locator and documentation repairs do
not claim new live Gateway qualification.

Current reference evidence locators point to the rewritten archive. The exact
archived reference manifests and their evidence checksums are unchanged.
Private execution packets removed from that archive are no longer public
artifacts; a link to this page explains their intentional absence. Some old
test commits refer to removed local receipts and cannot replay those historical
checks from public Git alone. Current fixtures and tests are preserved.

Original history is retained in a private recovery bundle outside the checkout.
Do not add that bundle, local worktree exports, or full execution logs to Git.
Use the existing ignored `bin/` or another private directory for new raw run
outputs. Reviewed compact qualification summaries remain in this directory.

This cleanup preserves public author attribution and the intentional security
contact. It removes accidental local execution detail, not project ownership.
