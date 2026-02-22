# Security Policy

## Reporting a Vulnerability

Please report suspected vulnerabilities privately. Do not open a public issue.

- Preferred: GitHub private advisory
  - `https://github.com/alex-mccollum/igw-cli/security/advisories/new`
- Fallback: email `contact@mccollum.dev`
  - Subject: `igw-cli security report`

Please include:

- A clear description of the issue, impact, and attack preconditions.
- Steps to reproduce with exact commands/flags.
- Affected version(s) and commit (if known).
- Environment details (OS, shell, how `igw` was installed).
- Sanitized logs/output or proof-of-concept details.
- Any suggested mitigation.

Do not include production secrets, tokens, or sensitive data in reports.

## Supported Versions

Security fixes are provided for the latest released version of `igw-cli`.
If you report an issue on an older release, you may be asked to reproduce it on
the current release first.

## Handling

- Target acknowledgment time: within 3 business days.
- Target triage time: within 7 business days from acknowledgment.
- After triage, we will share severity, scope, and a remediation plan.
- We request coordinated disclosure until a fix is available.
- Public disclosure is expected after a fix is released or an agreed disclosure
  date is reached.
