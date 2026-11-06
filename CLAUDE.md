# Coinslot — project rules

Per-user "active-use" screen time meter. Windows first; iOS and a central service later.

## Current scope: v1 = measurement only

- No blocking, no tokens, no time caps, no server. Do not build them.
- `ios/` and `service/` stay empty placeholders until explicitly started.
- Goal: reliably measure, per child account, active play time per metered game,
  separate from allowed apps and from time a game is merely open (launcher, download, update).

## Two measures (product model)

The final product tracks **app usage** (foreground time, including passive use such as
videos) and **active gameplay** (the stricter rule below) separately; see `docs/roadmap.md`.
The MVP builds active gameplay only, focused on Roblox and Minecraft. Keep logging the raw
foreground facts every sample so app usage can be computed from existing logs later.

## Active-play rule

A second counts as active play only when all hold:

1. A metered game's **player** process is running (not launcher/installer/updater).
2. Its window is the foreground window (`GetForegroundWindow` -> process).
3. Last input (`GetLastInputInfo`) was within the idle threshold (default 30s, configurable).

MVP assumes keyboard and mouse only. Post-MVP (planned, see `docs/roadmap.md`): controller
polling via XInput (`pad_idle_s`), and GPU/network activity to filter idle menus.
Don't build roadmap items until v1's evening test has passed.

Never hard-code process names. They live in config and must be confirmed by
inventory on the real machine (`coinslot procs`, raw collector logs).

## Architecture rules

- Language: Go (single module at repo root).
- `core/` is pure and platform-independent: no syscalls, no OS-specific imports. All
  classification, counting, and summarisation logic lives here and is unit-tested
  with synthetic logs.
- `windows/collector/` is the platform-neutral collector loop behind a `Probe` interface;
  it is tested on any OS with a fake probe.
- `windows/internal/win32/` is the only place Win32 calls live. Keep it thin: read
  facts, return them, no decisions.
- Collector writes raw facts plus derived fields. Raw facts (exe, path, title, process
  start/exit events) must be sufficient to re-classify a log with a corrected config.
- Records are versioned (`v`), carry user SID, run id and sequence number, and timestamps
  with local offset, so later work (shared per-child budget, 5-minute buckets, tokens,
  enforcement, a central service) can consume them without a format break.

## Deployment (v1)

- Runs per-user at logon via a scheduled task. A hardened system service is deferred.
- Config is machine-wide and admin-owned (`C:\Program Files\Coinslot\config.json`).
- Logs are per-user under `%LOCALAPPDATA%\Coinslot\logs\YYYY-MM-DD.jsonl`.

## Process

- Small, shippable commits. Each commit builds and passes `go test ./...`.
- Before committing: `go vet ./...`, `go test ./...`, and
  `GOOS=windows GOARCH=amd64 go build ./...`.
- Ask the owner before any large design change.
