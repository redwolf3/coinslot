# Evening test: collect data and compare to reality

Goal: one evening of logs from both kids, a summary per kid, and a check of that summary
against what actually happened. Budget: ~15 minutes of setup, plus whatever you note down
during the evening.

## Before the evening

1. Finish [windows-setup.md](windows-setup.md): `install.ps1` run. The process inventory can be
   done before, or after the evening from the logs (`inventory`); see §2 there.
2. Have each kid sign out and back in, then confirm the collector is running (as the kid):

   ```powershell
   Get-Process coinslotw
   & 'C:\Program Files\Coinslot\coinslot.exe' summary
   ```

   `summary` with no arguments reads today's log for the current user. A few minutes in, it
   should show `collector observed` going up.

3. Sync clocks: the logs use the PC's clock. Use the PC's clock (or a phone set to the same
   time) for your notes.

## During the evening: keep a ground-truth log

Every time something changes, write down the time to the minute. A paper sheet next to the PC is fine:

| time  | kid  | what (game / app / launcher / updating / away / idle in menu) |
|-------|------|----------------------------------------------------------------|
| 18:02 | kid1 | Roblox launcher, updating                                      |
| 18:05 | kid1 | Roblox playing                                                 |
| 18:31 | kid1 | Roblox open, left to get a snack                               |
| ...   |      |                                                                |

Things worth deliberately trying once, because they test the edges of the rule:

- the game updating or loading (should be **open, not playing**, not active);
- the kid walking away with the game in front for >1 minute (active stops after 30s idle);
- alt-tab to Duolingo with the game still open (allowed time, plus game open-not-playing);
- switching users with a game left running (must not count for the other kid);
- putting the PC to sleep for a few minutes with a game open, then waking it (the `timeline`
  should show a `no data` gap covering the sleep; the timeline does not show `run` ids, so
  check the raw log: the `sample` records before and after the gap should have the same `run`,
  while a new `run` has a `start` record whose `reason` says why; see
  [#6](https://github.com/redwolf3/coinslot/issues/6));
- an Among Us discussion/voting round with no typing (expected to under-count; see below).

## After the evening: check the config, then produce the summaries

If the inventory wasn't done beforehand, do it now from the logs, and fix `config.json` until
the games and allowed apps match ([windows-setup.md](windows-setup.md) §2):

```powershell
& 'C:\Program Files\Coinslot\coinslot.exe' inventory 'C:\Users\kid1\AppData\Local\Coinslot\logs'
```

Then, as admin, in PowerShell (replace the account folder names):

```powershell
$cs = 'C:\Program Files\Coinslot\coinslot.exe'
& $cs summary -date (Get-Date -Format yyyy-MM-dd) 'C:\Users\kid1\AppData\Local\Coinslot\logs'
& $cs summary -date (Get-Date -Format yyyy-MM-dd) 'C:\Users\kid2\AppData\Local\Coinslot\logs'
```

If the evening ran past midnight, drop `-date` (it prints every day in the folder).

`timeline` takes the same arguments and prints what happened when, as stretches of one state,
to hold against your notes:

```powershell
& $cs timeline -date (Get-Date -Format yyyy-MM-dd) 'C:\Users\kid1\AppData\Local\Coinslot\logs'
```

```
2026-11-06  PC\kid1  (S-1-5-21-1001)
  18:02:05–18:05:10  roblox       open, not playing (RobloxPlayerInstaller.exe in front)
  18:05:10–18:31:40  roblox       ACTIVE
  18:31:40–18:36:00  roblox       open, not playing (idle)
  18:36:00–18:50:00  duolingo     allowed   [roblox open]
```

`(no data, …)` lines are time the collector did not observe (sleep, restart, not running).

Also copy both log folders somewhere safe (or send them to me). With the raw logs, a
config fix can be re-run against the same evening without collecting again:

```powershell
Copy-Item -Recurse 'C:\Users\kid1\AppData\Local\Coinslot\logs' "$HOME\Desktop\coinslot-kid1"
```

## Comparing to reality

For each kid and game, compare:

| summary line | your notes should say |
|--------------|-----------------------|
| `ACTIVE PLAY` per game, first–last time | total minutes actually playing; first and last play time |
| `open, not playing` per game | launcher/update/loading time + time the game was open but not in use |
| `allowed apps` | minutes in Duolingo etc. with the kid actually using it |
| `collector observed` | roughly the time the kid was signed in |

Walk the `timeline` next to your notes, row by row. For each of your note times, check the
segment covering it says the same thing. A disagreement of more than the idle threshold
(30s) at a boundary, or a different state, is a finding. The summary tells you how big the
error is; the timeline tells you where it comes from.

Rough expectations: active play within a minute or two per hour of play (5-second samples
plus the 30-second idle cut-off). Big misses usually mean:

- **Active far too low, open-not-playing high** → the player exe or window title in
  `config.json` is wrong, so the game is matched as a launcher or not at all. Check
  `fg_exe`/`fg_title` in the raw log at a time you know they were playing:
  `Select-String RobloxPlayer <log> | Select-Object -Last 3`.
- **Active too low during quiet game phases** → idle threshold too strict for that game.
  Re-run with `summary -idle 60` and compare. That changes only the report, not the logs.
- **Gaps** (observed much lower than signed-in time) → the collector wasn't running. The
  `timeline` shows them as `(no data, …)` lines. Look for `start` records (`Select-String '"type":"start"' <log>`). Each one is a collector start.
- **Time on the wrong kid** → should not happen. Send me the logs.

Write down the per-game differences (summary minus notes). That table is the acceptance
check for v1 and decides whether the next step is tuning (idle threshold, GPU/network
signals for idle menus) or moving on to budgets.
