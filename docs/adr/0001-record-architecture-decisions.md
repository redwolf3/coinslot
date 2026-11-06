# 0001. Record architecture decisions

- Status: Accepted
- Date: 2026-11-06

## Context

The project is built in short iterations, often by AI agents that start each session without
history. Decisions made in chat get lost, then revisited or silently reversed.

## Decision

Record each significant decision as an ADR in `docs/adr/` using `template.md`. Accepted
ADRs are immutable apart from their status; a changed decision gets a new, superseding ADR.

## Alternatives considered

- Decisions only in `design.md`: it describes the current state, not why, and loses rejected options.
- Commit messages only: not discoverable.

## Consequences

Agents read `docs/adr/` before changing a past decision (see `AGENTS.md`). ADRs 0002–0007
backfill decisions made during initial development.
