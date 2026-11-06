# Game state signals: Roblox, Minecraft, Among Us

- Date: 2026-11-06
- Status: **unverified.** From general knowledge, not yet checked on the family PC.
- Question: do these games expose whether the player is *in a game* versus in menus or the
  app's home screen, beyond process name and foreground window?

## Summary

None offers an official local "in game" API. The usable local signals:

| Game | Window title | Process | Log files | Network presence |
|------|--------------|---------|-----------|------------------|
| Roblox | Static ("Roblox") | `RobloxPlayerBeta.exe` may also host the app home screen, not only games | Yes: join/leave lines | Yes: web presence API |
| Minecraft Java | **Changes with state** | Generic `javaw.exe` | Yes: `latest.log` | No |
| Minecraft Bedrock | Static ("Minecraft") | `Minecraft.Windows.exe` | None usable known | Possibly Xbox Live presence |
| Among Us | Static | `Among Us.exe` | Unity `Player.log`; content unknown | Unknown |

## Roblox

- **Process ambiguity (matters for MVP accuracy):** newer Roblox desktop builds appear to run
  the home screen, avatar editor and catalog in the same player process. If so, the current rule
  counts browsing the Roblox app as active play.
- **Client logs:** `%LOCALAPPDATA%\Roblox\logs\*.log` contain lines on joining a game (place
  id, server) and on disconnect/leave. The third-party launcher Bloxstrap uses them for Discord
  rich presence, which suggests they are reliable enough in practice. The format is
  undocumented and changes between versions.
- **Web presence API:** `presence.roblox.com` gives offline / online (app or website) / in game /
  in Studio, plus the place if privacy allows. It needs the network, the kids' Roblox user IDs,
  and usually an auth cookie. It fits the later central service, not the local collector.
- **Roblox parental controls:** a linked parent account shows screen time and can set daily
  limits. Useful as an external cross-check, but per Roblox account, not per Windows account.

## Minecraft

- **Java:** the window title is like `Minecraft 1.21.x` in menus, with `- Singleplayer` or
  `- Multiplayer (3rd-party Server)` / `(LAN)` appended in a world (`*` means modded). It is
  already captured in `fg_title`, so a title rule can separate menus from play retroactively
  (ADR 0004). `.minecraft/logs/latest.log` records world loads and server connections.
- **Bedrock:** the title is static and there is no known client log. Xbox Live presence may expose
  "in menus" or "playing", but that is unverified and needs the network.

## Among Us

- Unity log at `%USERPROFILE%\AppData\LocalLow\Innersloth\Among Us\Player.log`. Whether it
  records lobby, game or meeting transitions is unknown.

## Implications

1. Minecraft Java's title is a free signal; only a config rule is needed if menu time matters.
2. Tailing game log files is the only local way to tell "in game" for Roblox (and maybe Among
   Us). If needed, it belongs in the probe as raw events, interpreted by the core. It is
   brittle; don't build it unless the evening test shows menu time is significant.
3. Network presence APIs belong with the central service.
4. Open product question: should Roblox home/catalog browsing and Minecraft menus count
   against the budget? Under ADR 0006 they count as app usage either way.

## To verify on the PC

- [ ] Open the Roblox app without joining a game, then run `coinslot procs`. Is `RobloxPlayerBeta.exe` running?
- [ ] Save the latest `%LOCALAPPDATA%\Roblox\logs\` file after an evening; confirm join/leave lines.
- [ ] Which Minecraft edition do the kids run? For Java, confirm the title format in `fg_title`.
- [ ] Among Us `Player.log`: any state transitions?
