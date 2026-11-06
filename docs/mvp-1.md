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

## 6. Manual tasks: owner, on the family PC

Steps only you can do, in order. Guides: [windows-setup.md](windows-setup.md),
[evening-test.md](evening-test.md). Tick each item here (or tell an agent to) as you go.

### 6a. Get the build

- [ ] Download `coinslot-windows-amd64` from the latest successful CI run (GitHub → Actions →
      `ci` → run on this branch → Artifacts; requires being signed in to GitHub) and extract it.
      Alternative: install Go and Git Bash, then run `windows/scripts/package.sh`.
- [ ] Unblock the extracted files. They are unsigned and downloaded, so Windows marks them:
      `Get-ChildItem <folder> -Recurse | Unblock-File`. If Defender/SmartScreen still blocks
      or quarantines `coinslot.exe`, note it here; signing may become a task.

### 6b. Inventory process names (signed in as each kid, or with them)

- [ ] Roblox: `coinslot.exe -config .\config.json procs` before launching and while in a game;
      identify the player and launcher/updater exes.
- [ ] Roblox app home screen: open the app *without* joining a game and run `procs`. Is
      `RobloxPlayerBeta.exe` running? ([analysis](analysis/2026-11-06-game-state-signals.md#to-verify-on-the-pc))
- [ ] Minecraft: note the edition (Java or Bedrock) and the player and launcher exes. For Java, note
      the window title in menus vs in a world (`coinslot.exe ... once` while it's in front).
- [ ] Duolingo (and any other allowed app): app or browser? Note the exe and window title.
- [ ] Among Us (optional): player exe.
- [ ] Edit `config.json` with the confirmed names; delete `note` fields you've confirmed.
      Send the confirmed names so `config/config.example.json` can be updated.

### 6c. Install

- [ ] In an admin PowerShell, in the package folder:
      `powershell -ExecutionPolicy Bypass -File .\install.ps1`. Report any error output.
- [ ] Confirm the task exists: Task Scheduler → `\Coinslot\Collector` (or
      `Get-ScheduledTask -TaskPath \Coinslot\`).
- [ ] Have each kid sign out and back in.
- [ ] As each kid: `Get-Process coinslotw` shows it running, and
      `& 'C:\Program Files\Coinslot\coinslot.exe' summary` shows `collector observed` increasing.
- [ ] Check the PC's clock is correct (your notes are compared against it).

### 6d. Evening test

- [ ] Keep ground-truth notes for both kids (time, kid, what), per [evening-test.md](evening-test.md).
- [ ] Try the edge cases once: game updating/loading, walking away >1 min with the game in front,
      alt-tab to Duolingo with the game open, switching users with a game running.
- [ ] Afterwards, as admin: run `summary` on each kid's log folder.
- [ ] Back up both kids' log folders off the PC's profile directories (don't commit them).
- [ ] Compare summary vs notes per kid per game. Send the differences, the summaries, and
      (optionally) the logs. Results get recorded in `docs/analysis/`.

## 7. Release

- [ ] Agent: apply fixes from validation (config example, idle threshold, bugs)
- [ ] Agent: re-tick section 6 items as confirmed; record the evening-test result in `docs/analysis/`
- [ ] Owner: open or approve the pull request and merge to `main`
