# ADR-0001: Thin MVP CLI

## Status
Accepted

## Context
This repository starts from zero and needs a practical, automatable API wrapper.

## Decision
Build a thin CLI first with:
- Generic `call` command.
- Minimal `config set|show` command.
- Lightweight `doctor` command.
- Standard library only for MVP.

## Consequences
- Fast to implement and maintain.
- Stable interfaces for scripting.
- Endpoint-specific ergonomics are deferred.
