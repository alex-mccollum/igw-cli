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
