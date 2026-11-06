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

## 2. Inventory process names (do this before trusting any numbers)

The rules in `config.json` are **unverified candidates**. Confirm them on the real PC.

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
