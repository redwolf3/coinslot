# 0004. Log raw facts; the core re-classifies with the current config

- Status: Accepted
- Date: 2026-11-06

## Context

Game process names are not known in advance and must be inventoried on the real PC. A wrong
config at collection time would otherwise ruin collected data.

## Decision

Each sample logs raw facts:

- foreground pid, exe, path, title;
- idle seconds and console state;
- process start/exit events in the session.

Derived fields (`fg_class`, `is_metered`, `player_running`) are logged for convenience
but the core ignores them. It rebuilds process state from `proc` events and classifies each
sample with the *current* config. Schema changes within `v: 1` are additive only.

## Alternatives considered

- **Collector decides active/not active:** smaller logs, but any rule or config fix would need
  new data.
- **Full process list every sample:** simpler replay, but about 1 MB/hour of redundant data.

## Consequences

Config fixes and rule changes apply retroactively, which is how app usage (ADR 0006) can be
computed from MVP logs. Logs are larger and contain window titles, which are private, so they
are never committed.
