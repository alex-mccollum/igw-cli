# Project Docs

- `docs/installation.md`: install options, installer scripts, checksum verification, and manifest usage.
- `docs/configuration.md`: config precedence, env vars, profiles, and WSL helper.
- `docs/examples.md`: practical command flows and wrapper examples.
- `docs/troubleshooting.md`: common failures and remediation commands.
- `docs/architecture.md`: architecture and contract notes.
- `docs/development-safety.md`: bounded local validation and WSL/Docker incident safeguards.
- `docs/plans/rebuild-v1.md`: accepted v1 rebuild goal, roadmap, and verification evidence.
- `docs/catalog.md`: Gateway catalog authority, snapshot storage, and freshness policy.
- `docs/rebuild-preview.md`: development CLI entrypoint, implemented contracts, and remaining work.
- `docs/automation.md`: machine-oriented automation patterns (`--json`, exit codes, workflow).
- `docs/rpc-protocol.md`: persistent RPC wire contract, handshake fields, and compatibility rules.
- `docs/host-integration.md`: recommended host adapter contract (rpc primary, fallback, startup checks).
- `docs/commands.md`: canonical command examples.
- Command example source of truth: update `docs/commands.md` first; keep `README.md` to a short onboarding subset that links back here.
- `docs/releasing.md`: release process and artifact expectations.
- `docs/decisions/`: architecture decision records.
- `scripts/smoke.sh`: local end-to-end smoke validation script.
