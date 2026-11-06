# 0003. Per-user collector started by a logon scheduled task

- Status: Accepted
- Date: 2026-11-06

## Context

Foreground window and last-input time are per interactive session; a service in session 0
cannot read them directly. v1 is measurement only, so tamper resistance is not yet needed.

## Decision

Run `coinslotw.exe` (GUI subsystem, no console) as the logged-on user, started by the
scheduled task `\Coinslot\Collector` at every user's logon:

- principal `BUILTIN\Users` (resolved by SID);
- `MultipleInstances Parallel`, so each fast-user-switch session gets one;
- no execution time limit.

A per-session named mutex (`Local\CoinslotCollector`) prevents duplicates in a session.
The config is admin-owned in Program Files. Logs go to per-user `%LOCALAPPDATA%`.

## Alternatives considered

- **Windows service plus a per-session helper:** tamper-resistant, but much more machinery.
  Deferred until enforcement.
- **Registry Run key:** no restart-on-failure; harder to manage centrally.

## Consequences

Children can kill the collector or edit their own logs; a gap shows up as missing samples or a
new `run` id. The process is killed at logoff without a `stop` record. The hardened service
is on the roadmap.
