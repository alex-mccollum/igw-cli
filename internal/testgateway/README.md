# Disposable Gateway test infrastructure

This contributor package operates only on freshly created, digest-pinned
official Ignition containers. It requires the repository's bounded Linux
validation scope and a running Docker engine with cgroup v2 limits.

The engine-wide `igw-qualification` name is an exclusive slot. Each invocation
uses a random ownership label, verifies the exact ID before removal, and refuses
admission while any qualification container remains. No host/engine recovery
or broad cleanup is performed.

The container has separate memory, swap, CPU, and PID limits, checked in both
Docker configuration and its running cgroup. The original image entrypoint runs
under a ten-minute timeout with fifteen seconds for termination. Cleanup uses
an independent deadline after caller cancellation. Credentials enter a private
file through a tar stream, never Docker arguments or environment values.

`TestLiveCaptureLifetime` is opt-in via `IGW_CAPTURE_TEST_IMAGE` and optionally
`IGW_CAPTURE_TEST_DOCKER`. Compile the test binary in a guarded build job, then
run that test alone in a separate guarded invocation. It verifies applied
limits, exclusive admission, a shortened lifetime, and exact-ID cleanup.
Normal tests use fake Docker responses and never provide live Gateway evidence.

Image qualification currently supports `linux/amd64`. The runner inspects the
image platform/configuration digest, specifies that platform at creation, and
checks the container's image ID before it starts. Capture v3 and workflow v2
receipts retain that observed provenance; historical receipts are not rewritten.
Use `IGW_LIFECYCLE_EVIDENCE` with a new file path to retain a lifecycle receipt
after successful containment, lifetime, and independent cleanup checks. It also
records the exact test-binary checksum. The resolved registry manifest's config
digest must match this image ID when preparing a qualified reference bundle.

`TestLiveAPIResourceContract` is separately enabled with
`IGW_ACCEPTANCE_TEST_IMAGE`. It commissions a fresh Gateway, creates a dedicated
test security level, and adds that level to the observed read/write AnyOf
policies using their signatures. It preserves existing administrator access
and refuses public or unexpected policies. A temporary API key receives that
level; the returned credential is the complete `name:key` value and stays in
memory. Browser authentication and CSRF tokens are confined to bootstrap.

All subsequent requests run through `nextcli.App` with an API token and no
cookie jar. Checks cover denied anonymous/bare-key access, catalog acquisition,
preview without mutation, basic-schedule creation, partial update, stale
signature rejection, and deletion with independent state reads. The extended
test also runs resource type/descriptor/page discovery and named-resource
previews, create/update/delete completion, duplicate-create refusal, and a
reviewed-signature conflict through the dedicated workflow commands. The optional
`IGW_ACCEPTANCE_EVIDENCE` path receives a new atomic receipt only after every
assertion passes and the owned container has been removed. It contains the
image/version, test-binary hash, catalog provenance, and check outcomes, never
credentials or resource configuration values. Each receipt qualifies only the
checks present in that run; earlier generic-only receipts remain historical.

`TestLiveQueryFilters` separately qualifies list filter encoding. Compile one
test executable from a clean source commit and first run the lifecycle probe
with that same executable and pinned image. Run the query test alone under the
bounded runner with `IGW_ACCEPTANCE_TEST_IMAGE`, `IGW_CAPTURE_TEST_DOCKER`,
`IGW_TEST_MODULE_PROFILE`, and a new `IGW_QUERY_EVIDENCE_DIR`. Serialize images
and invocations; never overlap compilation, lifecycle checks, or live suites.

The suite creates two schedules and two disabled projects in its disposable
Gateway. It verifies actual resource, project, and log filtering, exact special
characters, combined filters, pagination, nonmatches, generic API parity,
preview without dispatch, and invalid/duplicate-key refusal before dispatch.
The observed module profile is checked before API credentials are provisioned.
The evidence directory retains the exact compressed OpenAPI document and a
receipt with catalog/image identities, executable checksum, per-check HTTP
request counts, and cleanup status. A failed run also writes a failed receipt
when possible; retain it without retrying host failures. Success requires both
a passing test process and the completed receipt, followed by an independent
empty qualification-container query. Record the clean source commit and its
before/after Git status alongside the run. This evidence supplements the
reference matrix; it does not rewrite older workflow receipts or expand their
qualification claims.
