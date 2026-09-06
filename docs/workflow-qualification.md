# Workflow candidate qualification

The [release plan](plans/rebuild-v1.md) defines six task journeys. The opt-in
`TestLiveWorkflowJourneys` harness runs the prebuilt Linux `igw` executable as
separate processes against one fresh owned Gateway. It covers profile setup,
discovery, resource preview/change/conflict, project ZIP transfer, applicable
JSON tag transfers, log selection and human output, and complete backup/log/
diagnostic artifacts. Existing suites separately qualify HTTP request counts,
uncertain writes, restart, and broader contract behavior.

## Run from a clean candidate

Follow [workstation safety](development-safety.md) and the lifecycle procedure
in [catalog qualification](catalog.md). Use one clean source commit and record
its identity before and after the run. Build the CLI and contributor test binary
through the bounded runner before starting any Gateway. Preserve their hashes.

```bash
bash scripts/bounded-run.sh -- go build -trimpath -o /absolute/evidence/igw ./cmd/igw
bash scripts/bounded-run.sh -- go test -c -trimpath -o /absolute/evidence/testgateway.test ./internal/testgateway
```

Both output paths must be outside the clean checkout. For each pinned image,
first pass `TestLiveCaptureLifetime` using that exact contributor test binary.
Verify its receipt and independent empty name/ownership-label container queries
before starting a workflow. The engine must already be available; never recover
WSL/Docker Desktop or increase limits to make a check run.

Set `IGW_CAPTURE_TEST_DOCKER` to the Docker executable,
`IGW_ACCEPTANCE_TEST_IMAGE` to the verified immutable image reference, and
`IGW_TEST_MODULE_PROFILE` to the reviewed module profile. Then set
`IGW_JOURNEY_CLI_BINARY` to the absolute prebuilt CLI path and
`IGW_JOURNEY_EVIDENCE_DIR` to a new, nonexistent private output directory.

```bash
bash scripts/bounded-run.sh -- /absolute/evidence/testgateway.test -test.run '^TestLiveWorkflowJourneys$' -test.v -test.count=1 -test.timeout=8m
```

The CLI subprocesses inherit the same resource scope and total suite deadline.
Their profile/token/cache live in temporary isolated directories; user
configuration and runtime Gateway environment overrides are excluded. The
ephemeral token enters profile setup through stdin, never a command argument.
The current process harness requires Linux XDG configuration paths.

## Interpret and retain results

The suite writes `journeys.json` and the exact compressed OpenAPI capture.
The receipt binds the image, observed modules, catalog, contributor executable,
CLI executable, timestamps, cleanup, and final outcome. `executable.checks`
records each process's output mode, exit/outcome, output digest, and artifact
digest where applicable. Raw logs, configuration values, credentials, and
temporary backups/diagnostic bundles are not included in that receipt.

Process checks do not claim observed HTTP request counts. The separate top-level
`checks` array records the bootstrap's in-process observed requests. On 8.3.0,
missing tag APIs must produce explicit capability refusals; those refusals are
not tag-transfer success. Human log checks confirm timestamp units against the
requested incident window and retrieve an observed message with search.

Retain failed results, stop on any failed stage, and verify cleanup independently
by both owned-container name and ownership label. Do not replay a failed suite
without diagnosing the failure. Qualify both selected versions sequentially.
Preserve source/build/log/receipt identities together, excluding temporary
configuration and Gateway artifact contents. Historical qualification remains
historical when a later commit changes code.

## Recorded candidate

The [completed candidate record](qualification/workflow-v1/README.md) retains
passing 8.3.0 and 8.3.9 native journeys and their exact build identities. Run
long local gates as separate serial bounded jobs: normal tests, race package
groups, performance, and individual platform builds. Do not combine an entire
cold race suite or six cross-builds with preceding tests into one ten-minute
scope. If a guard expires, retain it, identify unfinished work, and use smaller
jobs with the same caps; do not repeat completed work or raise limits.
