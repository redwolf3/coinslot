# MVP 1: measurement only (Windows)

Tracks the base work for the first merge to `main`. Scope and rules: [AGENTS.md](../AGENTS.md).
Post-MVP items live in [roadmap.md](roadmap.md), not here.

**Goal:** reliably measure, per child account, active play time per metered game (Roblox and
Minecraft first), separate from allowed apps and from time a game is merely open (launcher,
download, update). No blocking, tokens, caps, or server.

**Done when:** the evening test's per-game active minutes match the ground-truth notes within
about 1–2 minutes per hour of play, for both kids.

## 1. Repo skeleton

- [x] Layout: `core/`, `windows/`, `config/`, `docs/`, empty `ios/` and `service/`
- [x] README, LICENSE (Apache-2.0), `.gitignore`
- [x] Agent and project rules: `AGENTS.md` at the root; `CLAUDE.md` imports it
- [x] Stack chosen and justified: Go, stdlib only ([ADR 0002](adr/0002-go-stdlib-only.md))
- [x] Docs structure: design, roadmap, ADRs, analysis, guidelines, operator guides

## 2. Logging-only collector

- [x] Config maps process names → metered/allowed, player/launcher, optional title match
- [x] Sample every 5s (configurable) to daily JSONL with timestamp, user, foreground process,
      window title, is_metered, player_running, idle_seconds
- [x] Records carry schema version, SID, run id, sequence number, `dt_s`, local-offset timestamps
- [x] Process start/exit events so logs can be re-classified ([ADR 0004](adr/0004-raw-facts-reclassify.md))
- [x] Own-session processes only (fast user switching safe); `console_active` logged
- [x] UWP apps resolved through `ApplicationFrameHost.exe`
- [x] Single instance per session; no console window (`coinslotw.exe`)
- [x] Diagnostics: `coinslot procs`, `coinslot once`
- [x] Platform-neutral loop behind `Probe`, tested with a fake probe
- [x] Win32 probe runs on a real Windows machine (CI `windows-latest` smoke test)

## 3. Pure core: classifier, active-play counter, summary generator

- [x] Parser tolerant of truncated or corrupt lines
- [x] Classifier applies the active-play rule from raw facts with the current config
- [x] Replay rebuilds per-run process state from `proc` events
- [x] Credit model: capped backward credit, midnight split ([ADR 0005](adr/0005-credit-model.md))
- [x] Unit tests with synthetic logs (evening scenario, two kids, gaps, midnight, re-logon)
- [x] End-to-end test: collector → log file → summary

## 4. Daily summary per child

- [x] Active minutes per game, with first and last active time
- [x] Minutes open but not playing, per game and in total (no double count)
- [x] Minutes in allowed apps
- [x] `coinslot summary` with text or `-json` output, `-date` and `-idle` options, and log files/dirs as input

## 5. Run instructions and deployment

- [x] Packaging script and CI artifact (`coinslot-windows-amd64`)
- [x] `install.ps1` / `uninstall.ps1`: per-user logon scheduled task ([ADR 0003](adr/0003-per-user-scheduled-task.md))
- [x] Setup and inventory guide ([windows-setup.md](windows-setup.md))
- [x] Evening data collection and comparison guide ([evening-test.md](evening-test.md))

## 6. Validation on the family PC (owner)

- [ ] Inventory process names; correct `config.json` and remove `UNVERIFIED` notes
      (Roblox, Minecraft, Duolingo; Among Us optional)
- [ ] Check whether the Roblox app home screen runs as `RobloxPlayerBeta.exe`
      ([analysis](analysis/2026-11-06-game-state-signals.md#to-verify-on-the-pc))
- [ ] Note the Minecraft edition (Java or Bedrock)
- [ ] `install.ps1` runs cleanly; the collector starts at each kid's logon
- [ ] Collect one evening of logs for both kids, with ground-truth notes
- [ ] Summaries match the notes within tolerance; record the result in `docs/analysis/`

## 7. Release

- [ ] Fixes from validation applied (config, idle threshold, bugs)
- [ ] Pull request reviewed and merged to `main`
