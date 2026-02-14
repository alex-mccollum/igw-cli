# ADR-0001: Thin MVP CLI

## Status
Accepted

## Context
This repository started from zero and needed a practical, automatable API wrapper with minimal operational overhead.

## Decision
Build and keep a thin CLI architecture with:
- A generic `call` command as the execution core.
- Standard library dependencies only for MVP.
- Stable automation contracts (exit codes, JSON mode, mutation confirmation).
- A small set of selective convenience wrappers that delegate to `call`.

## Consequences
- Fast to implement and maintain.
- Stable interfaces for scripting.
- Endpoint ergonomics are added only for high-value workflows, while keeping the core generic.
