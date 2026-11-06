# Windows setup and process inventory (v1)

## 1. Get a build

Either download the `coinslot-windows-amd64` artifact from the latest CI run on GitHub
(Actions tab), or build it yourself (Git Bash, with Go installed):

```sh
windows/scripts/package.sh      # -> dist/coinslot/
```

The binaries are unsigned. After extracting a downloaded zip, remove the "downloaded from the
internet" mark so SmartScreen doesn't block them:

```powershell
Get-ChildItem .\coinslot-windows-amd64 -Recurse | Unblock-File
```

## 2. Inventory process names

The rules in `config.json` are **unverified candidates**. The numbers can't be trusted until
the names are confirmed. There are two ways to do this, and either is fine:

- **After collecting (recommended, least effort):** install with the example config and let the
  kids play. Then run `inventory` on their logs (below) and fix `config.json`. Logs keep the
  raw facts, so the same evening re-summarises correctly with the fixed config. No second
  session is needed.
- **Before collecting:** check by hand with `procs`/`once`, as in the second part of this section.

### From logs: `inventory`

As admin, after an evening (or as the kid, without the path, for today's own log):

```powershell
$cs = 'C:\Program Files\Coinslot\coinslot.exe'
& $cs inventory 'C:\Users\kid1\AppData\Local\Coinslot\logs'
```

It lists:

- **FOREGROUND:** each program that was in front, with total time, time with input, full path,
  its top window titles, and how the current config matches it (`app/class/role` or `-`).
  The game's **player** is the exe that was in front for most of the play time with
  the game's title.
- **PROCESSES:** programs started while collecting, with running time, start count, and path.
  Launchers, installers and updaters show up here, usually started just before the player and
  in front only briefly or not at all.

Anything with `match -` that is a game, launcher, or allowed app needs a rule. Edit
`C:\Program Files\Coinslot\config.json` as admin, re-run `inventory` until the matches are
right, then re-run `summary`. `-json` gives the full lists; `-top 0` shows every row.

### By hand, before collecting: `procs` and `once`

Signed in as a child (or with the child next to you), open PowerShell in the package folder:

```powershell
.\coinslot.exe -config .\config.json procs > before.txt
# start the game through its normal launcher, wait until in-game
.\coinslot.exe -config .\config.json procs > ingame.txt
Compare-Object (Get-Content before.txt) (Get-Content ingame.txt)
```

The `MATCH` column shows how the config classifies each process. For each game, find:

- the **player** process (the one that owns the game window while playing);
- **launcher/installer/updater** processes (present during start-up/updates, not while playing).

`.\coinslot.exe -config .\config.json once` prints a single sample with the foreground
process, title, and classification. Run it with `Start-Sleep 5; .\coinslot.exe ... once`
and switch to the game window during the sleep.

Edit `config.json`: fix exe names, delete the `note`s you've confirmed, and repeat for Roblox,
Minecraft, Among Us, and each allowed app (Duolingo is usually a browser tab, hence the
`title_contains` rule).

## 3. Install (admin PowerShell, in the package folder)

```powershell
powershell -ExecutionPolicy Bypass -File .\install.ps1
```

This copies the binaries and `config.json` to `C:\Program Files\Coinslot` (users can read,
not edit) and registers the scheduled task `\Coinslot\Collector`, which starts
`coinslotw.exe` (no window) at every user's logon, as that user. Re-running keeps the
installed `config.json`; pass `-OverwriteConfig` to replace it.

The collector starts at each child's **next** logon (sign out and back in), or start it now
in the current session with `& 'C:\Program Files\Coinslot\coinslotw.exe'`.

## 4. Check it is running

As the child:

```powershell
Get-Process coinslotw
Get-Content "$env:LOCALAPPDATA\Coinslot\logs\$(Get-Date -Format yyyy-MM-dd).jsonl" -Tail 3
```

You should see a `sample` line every 5 seconds. If the collector cannot start (e.g. bad
config), it writes an `error` record with `"op":"startup"` to the same log.

As admin, to read a child's logs: `C:\Users\<child>\AppData\Local\Coinslot\logs\`.

## Uninstall

```powershell
powershell -ExecutionPolicy Bypass -File .\uninstall.ps1
```

Logs stay in each user's profile.

## Notes

- `coinslotw.exe` is killed at logoff without writing a `stop` record; the core treats the
  last sample of a run as its end.
- Updating config: edit `C:\Program Files\Coinslot\config.json` as admin. It is read at
  collector start, so it applies from the next logon. Logs record the config's SHA-256.
