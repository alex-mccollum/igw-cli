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

`TestLiveRestart` uses a new `IGW_RESTART_EVIDENCE_DIR` with the same pinned
image, module profile, clean test executable, lifecycle probe, and serialized
bounded invocation as the input suites below. It tests confirmation refusal,
conflicting flags, offline refusal, a read-only baseline preview, one full
Gateway restart, and independent task/doctor/preview reads after completion.
Only the Java Gateway process is restarted through the advertised API.

Before and after that command, `ObserveJavaProcess` verifies the exact owned
container, image, loopback binding, Docker limits, and running cgroup limits.
It checks the API-reported PID's `exe` link and reads its `comm` and `stat`, requiring
a Java process with a later start time after restart. The start counter is
[Linux `/proc/PID/stat` field 22](https://man7.org/linux/man-pages/man5/proc_pid_stat.5.html).
No environment or command line is captured. The container ID/start time must
remain unchanged and its Docker restart count must remain zero. No Docker,
Docker Desktop, or WSL restart/recovery command is used.

The executable identifies Java; `comm` is a thread name and can change while
the process remains Java. The first restart qualification attempt exposed this
harness assumption during its independent baseline check. It stopped before
any restart POST and cleaned up; that failed receipt remains historical.

`restart.json` retains projected CLI evidence, hashed node identity, pending
task counts, actual request counts, exact-confirmation counts, process evidence,
catalog provenance, and cleanup status. The exact compressed vendor document
is saved alongside it. Keep source/build/lifecycle/process results and an
independent empty-container check with this receipt. A test binary that builds,
a skipped test, or a prepared live harness is not Gateway qualification.

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

`TestLiveBodyInputs` qualifies the generic request body boundary against the
actual captured contract. Use the same clean-build/lifecycle procedure with a
new `IGW_INPUT_EVIDENCE_DIR`; all other image, Docker, and module-profile
variables match the query suite. The test exercises literal/file/stdin text,
explicit empty representations, opaque binary bodies and uploads, zero-request
previews, and invalid-input refusal. Encryption responses are checked as
structural flattened JWE envelopes; no decryption or cryptographic validation
is claimed. Payload observers hash consumed bytes without buffering, changing
empty-body framing, or recording credentials/query values.

On 8.3.9, the suite also exercises the advertised translations bulk-datafile
route with repeated `files` parts and explicit filenames. The field name is a
candidate for controlled qualification because the vendor document omits its
part schema. Success requires independent downloads matching every supplied
byte (including an empty file), a rejected stale signature, a verified opaque
overwrite, and file deletion. It creates a core translations singleton only if
absent, then removes it; otherwise its original configuration is compared after
file cleanup. The 8.3.0 contract lacks the bulk route and must refuse it before
dispatch. These version-specific expectations are not claims about other
Gateway/module configurations.

The new evidence directory receives the exact compressed OpenAPI document and
`body-inputs.json`, including failed-run receipts when possible. Record the
clean source commit, executable checksum, lifecycle and process exit results,
and an independent empty-container check alongside it. A compiled or skipped
test is not live acceptance; both process and complete receipt must pass.

The retained [body-input evidence](testdata/body-inputs/README.md) records passing
31-check 8.3.0 and 62-check 8.3.9 core-profile runs from source `03566a8`, each
preceded by a passing lifecycle probe. Independent downloads establish that
`files` worked for the exercised translations route. The 8.3.9 singleton
already existed, so the optional creation/deletion branch remains
unqualified. An anchored manifest preserves all 21 original files; offline
tests check provenance, cleanup, input/readback identities, and current parsing
without renewing the recorded parser or observation times.

`TestLiveBatch` uses the same clean-source build and per-image lifecycle gate,
with a new `IGW_BATCH_EVIDENCE_DIR`. It checks malformed-manifest and confirmation
refusal, online/offline previews, read-only and mixed batches, retained responses
after later validation/HTTP failures, and explicit continuation. Catalog fetch
counts distinguish one fresh invocation snapshot from per-item refreshes.
Receipts in `batch.json` retain ordered item outcomes, coverage, and prepared
input identities, while successful response bodies and ciphertext are inspected
only in memory. This suite does not force live transport failures or revoke
credentials midway through a batch; those stop behaviors have fixture coverage.
Passing still requires a complete receipt, a zero process exit, and independent
container absence checks. Compile or skip results do not establish live success.
The retained [batch evidence](testdata/batch/README.md) records passing 13-check
runs on both pinned core profiles from source `2ddc7f4`, including 29 item results
per run. Keep that source distinct from subsequent input-budget hardening.
