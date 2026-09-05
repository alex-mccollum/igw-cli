# Gateway qualification matrix

Qualification applies to a recorded image, observed module inventory, contract
policy, and set of exercised workflows. A version label alone does not establish
support for every API or module combination. The target Gateway's current
catalog determines which APIs the CLI can use.

## Qualified version and module cells

| Gateway | Profile | Observed modules | Operations | Tag transfer workflows |
| --- | --- | --- | ---: | --- |
| 8.3.0 | image-defaults | 32 active | 672 | APIs absent; import, preview, and export refuse before dispatch |
| 8.3.9 | image-defaults | 32 active | 687 | Advertised; all 39 project/tag checks passed |
| 8.3.0 | core-opcua | 1 active, 31 inactive | 446 | APIs absent; all 27 applicable project/tag checks passed |
| 8.3.9 | core-opcua | 1 active, 31 inactive | 454 | Advertised; all 39 project/tag checks passed |

All four cells passed all 18 pipeline stages under workflow qualification
policy 2 on 2026-09-05. Each includes 10 lifecycle, 27 resource, and 23
operational checks in addition to the applicable project/tag checks.

The minimum reference covers catalog validation, containment, basic schedule
resources, disabled projects, and backup/log/diagnostics workflows. It lists tag
round trips separately as unavailable. The 8.3.9 candidate covers those same
five scopes plus tag round trips. Negative tests exercise expected
failures; they do not imply a missing workflow passed a round trip.

Original clean-source run receipts and complete candidates are retained under
`internal/referencebuild/testdata/`. Each reference manifest binds exact vendor
bytes, registry manifests, image configuration, module inventory, parser,
contract identity, test executable, workflow scopes, and receipt checksums.
See [reference updates](reference-updates.md#recorded-real-run) for dated results
and [catalog authority](catalog.md) for freshness and pinning semantics.

Both policy-2 candidates are independently readable by specifying their
directories; they are not yet embedded runtime selectors. The original bundled
8.3.9 reference remains unchanged. Reopening historical evidence never renews
its live verification timestamp.

## Observed module profiles

The core runs use the explicit `com.inductiveautomation.opcua` whitelist and
retain all 32 installed module observations. The selected OPC UA module is
active/enabled; the other 31 are inactive/disabled. Both complete pipelines ran
from clean commit `28d7334`, with the same acceptance executable. Original
receipts and qualified references are retained in
`internal/referencebuild/testdata/ignition-8.3.0-core/` and
`internal/referencebuild/testdata/ignition-8.3.9-core/`. Separate preceding
capture-only fixtures remain in `internal/moduleprofile/testdata/` with their
original observation times.

The coordinator's `--module-profile core-opcua` passes the same selection to
capture and every workflow suite. Module policy `igw-module-profile/1` verifies
the resulting inventory and refuses unexpected module states or substituted
profiles. IA documents fully qualified identifiers and the
module whitelist in its [container guide](https://www.docs.inductiveautomation.com/docs/8.3/platform/docker-image#module-identifiers-table)
and [environment reference](https://docs.inductiveautomation.com/docs/8.3/appendix/reference-pages/platform-environment-variables).
The whitelist value expresses intent; observed modules establish the test cell.

This matrix does not qualify third-party modules, every first-party workflow,
arbitrary cross-version migrations, or production operational health. Remote
scheduled qualification still needs a provisioned runner and an observed run.
The [execution plan](plans/rebuild-v1.md) tracks remaining request, workflow,
migration, performance, and release gates for the full v1 goal.
