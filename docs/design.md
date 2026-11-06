# Design (v1)

## Stack: Go

Go cross-compiles a single static `.exe` from any OS with no runtime to install on the
shared PC, and its type checker plus `GOOS=windows go build`/`go vet` catch Win32 binding
mistakes before anything is copied to Windows (Python+ctypes only fails at runtime there).
The pure core and the collector loop are plain Go tested with `go test` on any OS, so the
edit/test loop is seconds; only the thin Win32 probe needs the real machine.

## Layout

```
go.mod                          module github.com/redwolf3/coinslot
core/
  config/                       config schema, validation, exe/title -> rule matching
  record/                       JSONL record types (schema v1)
  activity/                     parse logs, replay process state, classify, credit intervals
  summary/                      daily summary per child (date, SID)
  inventory/                    foreground exes/titles and started processes from logs (config discovery)
  internal/synth/               synthetic log builder for tests
windows/
  cmd/coinslot/                 entrypoint: run | once | procs | summary | inventory
  collector/                    platform-neutral loop behind a Probe interface (tested with a fake)
  internal/win32/               the only Win32 code: foreground, idle, session processes
  scripts/                      install/uninstall scheduled task (PowerShell)
config/config.example.json      candidate rules — UNVERIFIED until inventoried
docs/
ios/  service/                  placeholders
```

Two builds of the same source: `coinslot.exe` (console; for `once`/`procs` diagnostics) and
`coinslotw.exe` (`-H=windowsgui`; no console window; used by the scheduled task).

## Collector

Every `sample_interval_seconds` (default 5) the collector:

1. Reads the foreground window -> PID -> exe, full path, title. For UWP apps the foreground
   window belongs to `ApplicationFrameHost.exe`; the probe resolves the hosted child process.
2. Reads idle seconds from `GetLastInputInfo`.
3. Enumerates processes **in its own Windows session only** (so kid A's game in a
   disconnected fast-user-switch session never counts for kid B), diffs against the previous
   tick, and logs `proc` start/exit events.
4. Classifies with the config and writes a `sample` record.

It is single-instance per session (`Local\` named mutex). Logs go to
`%LOCALAPPDATA%\Coinslot\logs\YYYY-MM-DD.jsonl` (local date), opened in append mode per write.

A **run** (one `run` id) is a self-contained stretch of log: a `start` record, a full `present`
process snapshot, then diffs and samples. The collector starts a new run when the process
starts, when the local date changes (so each daily file replays on its own), and after
any failed write (the logged process state may have lost an event, so it is re-snapshotted).
The first sample of a run credits 0s, so a run change costs at most one interval.

The collector does **not** decide what counts as active play. It records raw facts plus
convenience fields; the core decides. Because raw facts (exe, path, title, process events)
are logged, a day can be re-classified after correcting the config.

## Config

```json
{
  "schema": 1,
  "sample_interval_seconds": 5,
  "idle_threshold_seconds": 30,
  "apps": [
    {"app": "roblox", "class": "metered", "role": "player", "exe": ["RobloxPlayerBeta.exe"]},
    {"app": "minecraft-java", "class": "metered", "role": "player",
     "exe": ["javaw.exe"], "title_contains": ["Minecraft"]}
  ]
}
```

- `class`: `metered` | `allowed`. `role`: `player` | `launcher` (launcher also covers
  installers/updaters; defaults to `player`).
- Rules are evaluated in order; first match wins. Exe and title matching are case-insensitive.
- `title_contains` rules can only match the foreground window (background processes have
  no title in the process list), so they never set `player_running` on their own.

## Core: classification and counting

The core re-classifies every sample from **raw facts** with the *current* config. It ignores
the collector's derived fields (`fg_class`, `is_metered`, `player_running`), so fixing a
process name in the config fixes past days too.

Per sample, with the run's process set rebuilt from `proc` events:

| state | condition |
|-------|-----------|
| present | `console_active` and `idle_s` known and `<= idle_threshold_seconds` |
| active play (app A) | present, and foreground matches A's metered **player** rule |
| allowed (app B) | present, and foreground matches an allowed rule |
| open, not playing (app A) | any of A's player/launcher processes running, or A in foreground, and A not active |

A process identified only by a title-qualified rule (e.g. `javaw.exe` + "Minecraft") is
remembered by pid once seen in the foreground with that title, and counts as open until it exits
(or the pid is reused by a different exe). Seeing it in the foreground before the process
snapshot lists it does not lose that identity.

App usage (foreground time including passive use such as videos) is a separate, planned
measure; see [roadmap.md](roadmap.md#product-model-two-measures). The v1 `allowed` figure
requires input and will be superseded by it.

These states are not exclusive. Roblox open behind Duolingo counts as Duolingo allowed time and
Roblox open-not-playing time.

**Credit:** each sample credits `min(dt_s, 2 × interval)` seconds to the state it saw, covering
the time since the previous sample in the same run. The first sample of a run credits 0.
Sleep/hibernate, a stalled collector, or a killed-and-restarted collector therefore never credit
more than ~10s. Intervals that cross local midnight are split between the two days.

**Daily summary** per (local date, SID): observed, present, active play per game (with first/last
active time), open-not-playing per game and in total (no double count), allowed per app.
`coinslot summary` prints it as text or `-json`.

## Records (JSONL, schema v1)

Every line has a common header:

| field  | meaning |
|--------|---------|
| `v`    | schema version (1) |
| `type` | `start` \| `sample` \| `proc` \| `error` \| `stop` |
| `ts`   | wall time, local with offset, ms precision (`2026-11-06T19:02:05.120-05:00`) |
| `run`  | random id per collector start (detects restarts/gaps) |
| `seq`  | monotonic per run |
| `user` | `HOST\account` |
| `sid`  | account SID — stable identity key for later per-child budgets |

`sample` fields:

| field | meaning |
|-------|---------|
| `dt_s` | monotonic seconds since previous sample in this run (0 for first) |
| `console_active` | this session is the active console session (false when switched away) |
| `fg_pid`, `fg_exe`, `fg_path`, `fg_title` | foreground process facts (empty when none, e.g. lock screen) |
| `fg_host_exe` | set when the window was hosted (e.g. `ApplicationFrameHost.exe`) |
| `fg_app`, `fg_class`, `fg_role` | config match for the foreground (`fg_class`: `metered`/`allowed`/`unknown`/`none`) |
| `is_metered` | foreground matches a metered rule (player or launcher) |
| `player_running` | any metered player process is running in this session |
| `players_running` | app ids of running metered players |
| `idle_s` | seconds since last keyboard/mouse input in this session; **omitted** when the read failed (treated as not present) |

`proc`: `event` (`present` in a run's snapshot, then `start`/`exit`), `pid`, `exe`, `path`, and on `present` records `titles`: the distinct foreground window titles (up to 8, oldest first) the collector saw for that process. They let replay re-identify title-qualified apps such as Minecraft Java across run boundaries, even after the title changed to one that no longer matches.
`start`: `reason` (`startup` \| `new_day` \| `write_recovery`), host, session id, collector version, config path + SHA-256, interval, idle threshold.
`error`: `op`, `msg` (deduplicated: logged when the message for an op changes).

Durations are computed by the core from `dt_s` (capped, to ignore sleep/hibernate gaps),
not by assuming the nominal interval. 5-minute buckets, budgets, and tokens are derivable
from `ts` + `dt_s` + `sid` without format changes.

## Known gaps (v1)

- **Assumption: input is keyboard and mouse only.** Gamepad input does not update
  `GetLastInputInfo`, so controller play would look idle. Controller polling is planned
  post-MVP; see [roadmap.md](roadmap.md#controller-gamepad-input).
- Kids can delete or edit their own logs (per-user folder) or kill the collector. Accepted for
  measurement; the hardened service is deferred.
- Window titles are logged and may include browser page titles. Logs stay local.
