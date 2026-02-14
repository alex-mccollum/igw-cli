# Prompting Guide

Use this guide to get faster, higher-quality outcomes when building `igw-cli`.

## Core Strategy

1. Request one slice at a time.
2. Define strict constraints up front.
3. Define exact verification commands and expected results.
4. Require small, logical commits.
5. Require a concise handoff summary.

## Base Prompt Template

```text
Goal:
Build/modify <one thing>.

Scope:
Only touch <files/areas>.
Do not do <non-goals>.

Constraints:
- Go standard library only
- Preserve stable exit codes
- Keep CLI UX minimal
- Follow the project workflow in `AGENTS.md`

Acceptance checks:
- Run: go test ./...
- Run: go run ./cmd/igw <command> ...
- Expect: <exact output/status/behavior>

Delivery:
- Implement directly unless blocked
- Commit in small logical slices
- End with: what changed, files touched, verification, residual risk
```

## Feature Prompt Example

```text
Add `igw config set --auto-gateway`.

Scope:
- Add detection logic (ip route first, then /etc/resolv.conf)
- Wire flag into `config set`
- Add tests for detection precedence and CLI behavior

Constraints:
- stdlib only
- do not change existing flags or exit codes

Acceptance checks:
- go test ./...
- go run ./cmd/igw config set --auto-gateway
- go run ./cmd/igw config show

Delivery:
- one implementation commit
- one tests/docs commit
```

## Bugfix Prompt Example

```text
Fix timeout classification in `igw doctor` so timeouts and HTTP auth errors produce distinct hints.

Scope:
- only touch doctor/check error handling and related tests

Constraints:
- no dependency changes
- no output contract break

Acceptance checks:
- go test ./...
- verify doctor output for timeout and 403 paths

Delivery:
- write/update tests first
- then implement fix
- summarize before/after behavior
```

## Refactor Prompt Example

```text
Refactor config precedence handling for clarity.

Scope:
- internal/config and directly related tests only

Constraints:
- no behavior changes
- no flag/env name changes

Acceptance checks:
- go test ./...
- existing CLI behavior unchanged

Delivery:
- commit refactor only
- include short note proving no behavior drift
```

## Release-Readiness Prompt Example

```text
Prepare a release-readiness pass for MVP.

Scope:
- README command examples
- doctor/call smoke coverage
- verify build command

Constraints:
- no new features

Acceptance checks:
- go test ./...
- go build ./cmd/igw
- go run ./cmd/igw doctor --timeout 8s
- go run ./cmd/igw call --method GET --path /data/api/v1/gateway-info --json

Delivery:
- docs commit
- test/hardening commit
```

## Prompting Anti-Patterns

- "Improve this" without acceptance checks.
- Multiple unrelated features in one prompt.
- Missing constraints (deps, files, output compatibility).
- No explicit verification commands.
- No commit/handoff expectations.

## Fast Iteration Loop

1. Ask for one slice.
2. Verify with commands.
3. Commit.
4. Ask for next slice.

Short loops produce better quality than large all-at-once prompts.
