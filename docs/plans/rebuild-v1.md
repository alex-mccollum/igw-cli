# igw v1 workflow release plan

Status: completed within the agreed workflow scope; candidate remains unpublished.
Rebaselined with the user after the architecture review.
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
| Set up and discover | Configure a profile, read Gateway information, discover an operation, inspect its inputs and catalog provenance | Qualified on both pinned core-opcua Gateways |
| Configure resources | Preview and apply changes, inspect readback, refuse stale signatures without mutation | Named workflows qualified; translations creation remains explicitly limited |
| Deploy projects | Export and inspect a ZIP, import, and replace using a reviewed digest | Export/import/reviewed replacement qualified on both Gateways |
| Transfer tags | JSON import/export using existing verification policies; explicit refusal where APIs are absent | JSON transfers qualified on 8.3.9; explicit unavailable-API refusals on 8.3.0 |
| Troubleshoot | Retrieve relevant logs by time, severity, logger, and search; expose available exception context and pagination; distinguish no matches from failure | Human/JSON filters, search, timestamps, empty pages, and downloads qualified |
| Preserve evidence | Save complete backup/log/diagnostic artifacts; require confirmation for diagnostics generation | Complete artifacts and diagnostics preview/collection qualified on both Gateways |

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

The human-output slice passed the full unit suite, native build, 33 executable
smoke checks, command/docs checks, and focused race checks. Resource failures
retain comparison evidence in human mode; logs preserve stack/context and
unknown fields, escape terminal controls, and leave JSON unchanged. Doctor
states its limited scope. Logs: `bin/workflow-human-{checks,race}.log`.

`TestLiveWorkflowJourneys` runs the built Linux CLI as separate processes with
isolated configuration. It covers the six journeys and reuses existing transfer
checks; executable identity and per-command results are separate from observed
HTTP request counts. The candidate passed 48 native checks on 8.3.0 and 55 on 8.3.9.
See [workflow qualification](../workflow-qualification.md) for invocation.

## Completion evidence

Candidate `511c5fe` passed both pinned native journey runs, normal tests,
package-complete race coverage, performance budgets, docs, 34 smoke checks,
and all six artifact audits. Go 1.25.7 passed a build and focused regressions.
Race and packaging each needed separate remaining-work jobs after the original
ten-minute guard stopped a combined job; limits were not raised and the original
timeouts remain recorded. See the [qualification record](../qualification/workflow-v1/README.md)
for source/binary identities, exact scope, limitations, and all retained evidence.

Live journeys found and fixed help-flag ordering and missing non-ready diagnostics
file size handling. Tag capability selection was corrected in contributor
harnesses. Translations creation remains uncertain or rejected as documented;
verification was not weakened. No further implementation gate remains in this
revised plan. Scheduled-runner activation and native non-Linux qualification
remain follow-ups; no publication is included.

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
