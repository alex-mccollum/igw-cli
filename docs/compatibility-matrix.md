# Gateway qualification matrix

Qualification applies to a recorded image, observed module inventory, contract
policy, and set of exercised workflows. A version label alone does not establish
support for every API or module combination. The target Gateway's current
catalog determines which APIs the CLI can use.

## Default first-party modules

| Gateway | Observed modules | Operations | Current qualification | Tag transfer workflows |
| --- | ---: | ---: | --- | --- |
| 8.3.0 | 32 active | 672 | Policy 2 passed all 18 pipeline stages | APIs absent; import, preview, and export refuse before dispatch |
| 8.3.9 | 32 active | 687 | Policy 2 passed all 18 pipeline stages | Advertised; all 39 project/tag checks passed |

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

## Remaining module cells

Both versions still require current qualification with the explicit
`com.inductiveautomation.opcua` whitelist. A historical 8.3.0 capture of that
profile parses with 446 operations. Fresh guarded captures now confirm 446
operations on 8.3.0 and 454 on 8.3.9, each retaining one active OPC UA module
and 31 inactive/disabled modules. Original receipts and documents are in
`internal/moduleprofile/testdata/`. Complete workflow qualification is still
required for these two cells.

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
