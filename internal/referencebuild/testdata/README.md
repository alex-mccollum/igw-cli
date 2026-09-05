# Coordinator acceptance evidence

`ignition-8.3.9-update` retains the first complete real run of
`scripts/update-reference.py`, including its run receipt and all original files
in the independently readable qualified reference. This is historical test
evidence; it does not add another runtime selector or change the bundled default.

The clean source commit is `4734207735bae92b9d4a7b89ee08f668b9429288` and the
toolchain is Go 1.27.1 targeting Linux amd64 with CGO enabled. The run lasted
755.43 seconds, including cold builds with trimmed source paths. All 18 stages
passed: admission, engine capabilities, exclusive slot, source commit/status,
toolchain, both builds, fresh registry resolution, image pull, lifecycle probe,
capture, three workflow suites, source commit/status recheck, and qualification.

The four receipts identify test executable
`02a8334ff4b6a0e3785ff735fc6f4993c4b555e0003fea045f146145b1e4b71f`.
All 98 checks passed, including expected failed/partial workflow outcomes;
each receipt records successful cleanup. A separate post-run Docker query found
no qualification container. The image, 32-module inventory, and 687-operation
contract match the earlier qualified 8.3.9 reference. Raw/document identities
differ because the original generated documentation contains instance values.

Keep these files unchanged. Payload checksums and qualification scopes are in
`reference/reference.json`; `run.json` additionally binds orchestration to source
and toolchain provenance. The remote scheduled workflow has not been activated
or executed by this local run. See `docs/reference-updates.md` for the separate
runner and review requirements.

## Minimum version with capability-aware qualification

`ignition-8.3.0-policy2` retains the complete clean run from source
`ef5576291635090e3d95948d5fd2ebe949c82393`, using Go 1.27.1 on Linux amd64.
All 18 stages passed from 18:44:21 to 18:53:49 UTC on 2026-09-05 (568.08
seconds). The lifecycle/resource/project-tag/operational receipts contain
10/27/27/23 checks and identify test executable
`2ecddadc24565882d9902cf7916bdcb747b96c5c64cf5ad23e87ecde41e4160b`.
Every container was removed; an independent post-run query found none remaining.

The 672-operation catalog has 32 active first-party modules. Contract policy 2
produces the same identity as the two earlier minimum captures. Qualification
policy 2 requires all project checks and three observed zero-request refusals
for the absent tag APIs. The reference lists five successful scopes and records
`tags/memory-json` under `unavailableScopes`; it does not claim tag round trips.
All 23 backup/log/diagnostics checks passed in a separate fresh Gateway.

The original bundle and run receipt remain unchanged. The retained-data test
verifies original catalog, image manifests, and all four acceptance receipts
offline. It does not renew live evidence or add a runtime bundled selector.
The comparison with the default 8.3.9 baseline reports 15 absent operations and
other document changes, with `compatibility: requires_review`; qualification
covers the recorded workflows, not arbitrary cross-version API compatibility.
