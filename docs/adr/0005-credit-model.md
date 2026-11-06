# 0005. Time credit model: capped backward credit, midnight split

- Status: Accepted
- Date: 2026-11-06

## Context

Samples arrive about every 5s, but the collector can be paused (sleep, hibernate, a stalled
process) or restarted. Wall clocks can jump. Days are local dates.

## Decision

Each sample credits `min(dt_s, 2 × interval_s)` seconds to the state it observed, covering
the time since the previous sample of the same run. `dt_s` comes from the monotonic clock.
The first sample of a run credits 0. An interval crossing local midnight is split between
the two days.

## Alternatives considered

- **Nominal interval per sample:** wrong when samples are late or missed.
- **Wall-clock differences:** break on clock changes.
- **Forward credit (state holds until the next sample):** over-credits the last sample before
  a gap or logoff.

## Consequences

Error is at most about one interval per state transition. Gaps never inflate totals. 5-minute
buckets and budgets can be built from the same intervals.
