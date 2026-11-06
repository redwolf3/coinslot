# 0007. MVP input is keyboard and mouse only

- Status: Accepted
- Date: 2026-11-06

## Context

`GetLastInputInfo` does not see gamepad input, so controller play would look idle. The
children currently use keyboard and mouse.

## Decision

The MVP uses `GetLastInputInfo` only. Controller polling is planned for the product but not
in the MVP:

- poll XInput with dead-zone change detection (not the packet number, because of stick drift);
- log an optional `pad_idle_s` field.

The design is in `docs/roadmap.md`.

## Alternatives considered

- Build controller polling now: it adds probe complexity for an input that isn't used yet.

## Consequences

If a controller is used, active play is under-counted with no warning. Revisit when
controllers appear or the evening test shows unexplained under-counting.
