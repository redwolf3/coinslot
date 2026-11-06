# AGENTS.md

Guidance for AI coding agents (and humans) working in this repository. This file is the
single source of project rules; `CLAUDE.md` imports it.

## Project

Coinslot is an open-source, per-user **active-use** screen time meter. Windows first;
iOS and a central service later. Language: Go, one module at the repo root
(`github.com/redwolf3/coinslot`), standard library only.

### Current scope: v1 = measurement only

- No blocking, no tokens, no time caps, no server. Do not build them.
- `ios/` and `service/` stay empty placeholders until explicitly started.
- Goal: reliably measure, per child account, active play time per metered game,
  separate from allowed apps and from time a game is merely open (launcher, download, update).
- Focus games: Roblox and Minecraft (Among Us is in config but secondary).

### Two measures (product model)

The final product tracks **app usage** (foreground time, including passive use such as
videos) and **active gameplay** (the stricter rule below) separately. The MVP builds active
gameplay only. Keep logging the raw foreground facts every sample so app usage can be
computed from existing logs later. See [ADR 0006](docs/adr/0006-two-measures.md).

### Active-play rule

A second counts as active play only when all hold:

1. A metered game's **player** process is running (not launcher/installer/updater).
2. Its window is the foreground window (`GetForegroundWindow` -> process).
3. Last input (`GetLastInputInfo`) was within the idle threshold (default 30s, configurable).

MVP assumes keyboard and mouse only. Post-MVP items (controller polling, idle-menu
filtering, app usage, limits) are in [docs/roadmap.md](docs/roadmap.md). **Don't build
roadmap items until v1's evening test has passed.**

Never hard-code process names. They live in config and must be confirmed by inventory on
the real machine (`coinslot procs`, raw collector logs).

## Documentation map

Read the relevant document before changing an area. Keep documents current in the same commit
as the change.

| Path | What | Read when |
|------|------|-----------|
| [README.md](README.md) | Overview, layout, quick start | Always first |
| [docs/README.md](docs/README.md) | Index of all docs | Looking for anything |
| [docs/design.md](docs/design.md) | Architecture, collector, config, counting model, record schema | Changing any code |
| [docs/mvp-1.md](docs/mvp-1.md) | MVP 1 checklist: what's done, what's left | Before starting any MVP work; update it in every PR so it matches the state once that PR is merged |
| [docs/roadmap.md](docs/roadmap.md) | Planned post-MVP work and its designs | Before proposing new features |
| [docs/adr/](docs/adr/) | Architecture Decision Records: why things are the way they are | Before changing a past decision |
| [docs/analysis/](docs/analysis/) | Research and investigations (e.g. game state signals) | Before researching something again |
| [docs/guidelines/development.md](docs/guidelines/development.md) | Workflow, testing, schema/config changes, commit style | Before writing code |
| [docs/windows-setup.md](docs/windows-setup.md) | Inventory, install, verify on the PC | Touching install/packaging |
| [docs/evening-test.md](docs/evening-test.md) | v1 acceptance test procedure | Touching the summary or classifier |

## Architecture rules

- `core/` is pure and platform-independent: no syscalls, no OS-specific imports. All
  classification, counting, and summarisation logic lives here and is unit-tested with
  synthetic logs (`core/internal/synth`).
- `windows/collector/` is the platform-neutral collector loop behind a `Probe` interface;
  it is tested on any OS with a fake probe.
- `windows/internal/win32/` is the only place Win32 calls live. Keep it thin: read facts,
  return them, no decisions. Standard-library `syscall` only.
- The collector writes raw facts plus derived fields. Raw facts (exe, path, title, process
  start/exit events) must be sufficient to re-classify a log with a corrected config. The core
  classifies from raw facts, never from the collector's derived fields.
- Records are versioned (`v`), carry user SID, run id and sequence number, and timestamps
  with local offset. Schema changes are additive within a version; see the guidelines.

## Deployment (v1)

- Runs per-user at logon via a scheduled task. A hardened system service is deferred.
- Config is machine-wide and admin-owned (`C:\Program Files\Coinslot\config.json`).
- Logs are per-user under `%LOCALAPPDATA%\Coinslot\logs\YYYY-MM-DD.jsonl`.

## Commands

```sh
go test ./...                                  # all tests (any OS)
go vet ./... && GOOS=windows GOARCH=amd64 go vet ./...
GOOS=windows GOARCH=amd64 go build ./...       # Windows compile check
gofmt -l .                                     # must print nothing
windows/scripts/package.sh                     # -> dist/coinslot/ (exes, config, install scripts)
```

There is no local Windows; Win32 code is exercised only by CI (`windows-latest` smoke steps)
and on the real PC.

## Working rules

- Small, shippable commits. Each commit builds and passes all checks above.
- Merge pull requests with **squash and merge**.
- Every PR updates `docs/mvp-1.md` (the active milestone checklist) as part of its changes: tick,
  add, or reword items so the checklist is accurate the moment the PR is merged. Reference the PR
  or issue number on the items it completes.
- Ask the owner before any large design change. Record accepted decisions as an ADR.
- Write findings from research or log analysis to `docs/analysis/` so they are not redone.
- Question requirements that don't make sense or don't follow standards. Don't silently comply.
- Don't add third-party dependencies without asking (stdlib-only is a deliberate decision, ADR 0002).
- Don't commit logs from the real PC; they contain children's activity and window titles.
