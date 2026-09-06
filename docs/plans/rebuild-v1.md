# igw v1 workflow release plan

Status: active. Rebaselined with the user after the architecture review.
Implementation baseline: `8f8421f`. This plan supersedes the broad completion
criteria in the [historical execution record](rebuild-v1-history.md).

## Goal

Deliver a CLI that humans and shell agents can use to configure an Ignition
Gateway, transfer projects and tags, and investigate problems through status
and logs, using its own help, explicit changes, and trustworthy outcomes.

Keep the implemented architecture and features. Complete useful workflows;
additional OpenAPI feature coverage is not a release prerequisite unless a
required workflow needs it. Basic troubleshooting is part of the core release.

## Acceptance checklist

Each journey must run through the executable using documented commands, with
human and JSON output, positive results, and useful failure recovery.

| Journey | Required observations | Current status |
| --- | --- | --- |
| Set up and discover | Configure a profile, read Gateway information, discover an operation, inspect its inputs and catalog provenance | Implemented; final candidate walkthrough pending |
| Configure resources | Preview and apply changes, inspect readback, refuse stale signatures without mutation | Named workflows qualified historically; singleton recreation mismatch needs diagnosis |
| Deploy projects | Export and inspect a ZIP, import, and replace using a reviewed digest | Implemented and historically qualified; final candidate run pending |
| Transfer tags | JSON import/export using existing verification policies; explicit refusal where APIs are absent | Implemented; final candidate run pending |
| Troubleshoot | Retrieve relevant logs by time, severity, logger, and search; expose available exception context and pagination; distinguish no matches from failure | Filters implemented; human output and recovery guidance need improvement |
| Preserve evidence | Save complete backup/log/diagnostic artifacts; require confirmation for diagnostics generation | Implemented and historically qualified; final candidate run pending |

A generic accepted request is not a verified state change. Preserve uncertain
outcomes and never replay writes automatically. Diagnose the existing singleton
failure with the committed comparison evidence; fix a CLI defect if found, or
document the Gateway limitation without weakening verification to force a pass.

## Delivery slices

1. Rebaseline the roadmap and preserve linked historical evidence.
2. Improve human previews/results, readable logs, command examples, and
   configuration/connectivity/authentication/catalog recovery guidance. Preserve
   command names, the `igw/v1` JSON contract, and exit codes.
3. Resolve singleton recreation behavior and run the six documented journeys.
   Fix only issues that prevent those journeys or violate existing contracts.
4. Qualify the completed candidate against pinned 8.3.0 and 8.3.9 Gateways,
   recording unavailable capabilities explicitly. Run unit/race tests,
   executable scenarios, performance, docs, and all six artifact checks.
   Native platform claims require native evidence, not cross-compilation.

Use focused checks during changes, followed by one complete candidate gate.
Rerun affected checks after fixes. Keep historical receipts immutable; they are
supporting evidence, not proof that a later executable ran. Commit small verified
slices and preserve the user's unrelated script permission changes.

## OpenAPI authority and availability

The target's OpenAPI describes the advertised contract. Actual Gateway responses
establish behavior and state. Reviewed workflow rules define completion checks.
Retain live refresh, immutable snapshots, provenance, last-known-good documents,
and bundled offline references. Stale/reference data must remain visibly labeled.

The existing manual reference updater remains usable. Provisioning, activating,
and observing its dedicated scheduled runner is a separate infrastructure
follow-up, not a blocker for this release. No active schedule is claimed.

## Boundaries and evidence

Keep the current Go/Cobra/catalog/execution architecture, explicit `--yes`,
reviewed preconditions, credential isolation, atomic artifacts, exit codes
0/2/6/7, environment names, configuration precedence, and artifact naming.
Existing restart, batch, and other commands retain their documented behavior
and regression coverage. Do not add a separate automated diagnosis engine.

Defer unused serialization formats, broader tag policies, new workflow families,
MCP, persistent RPC, fleet orchestration, and desired-state deployment. No pushes,
tags, or publication are included. Workstation resource caps and WSL/Docker
lifecycle safeguards stay unchanged; an unavailable engine is not authorization
to repair the host.

- [Architecture](../architecture.md)
- [Command contracts](../commands.md) and [troubleshooting](../troubleshooting.md)
- [Gateway evidence and limits](../compatibility-matrix.md)
- [Local qualification evidence](../qualification/README.md)
- [Reference updater](../reference-updates.md)
- [Historical execution record](rebuild-v1-history.md)
