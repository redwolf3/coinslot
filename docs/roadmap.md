# Roadmap

v1 (MVP) is measurement only on Windows. Everything below is **planned for the product
but out of MVP scope**: don't build it until v1's evening test has passed.

## Product model: two measures

The final product tracks two separate things per child:

1. **App usage:** time an app is in the foreground while the child is plausibly using it,
   *including passive use* such as watching a video with no input. Applies to every app
   (and to sites, via browser window titles).
2. **Active gameplay:** the stricter measure for games: the game's player is in the
   foreground **and** there was input within 30s (the v1 rule).

For a game, `active_play(game) <= usage(game)`: Roblox in the foreground in menus or idle
counts as usage, not as active play. Budgets and limits (later) can target either measure.

**Priority:** the MVP measures active gameplay for Roblox and Minecraft first. App usage
reporting, then usage limits for other apps, come after the MVP.

### App usage: deciding "the child is using it"

Input alone is wrong for videos. Options, in order of cost:

| signal | what it catches | gaps | needs |
|--------|-----------------|------|-------|
| foreground + session active + not locked | anything visible | counts a paused or abandoned screen forever | nothing new (already logged) |
| ... + input within a long usage threshold (e.g. 10 min, configurable) | normal use, most videos | paused video counts up to the threshold; video watched >threshold with no input stops counting | nothing new; core-only |
| ... or the foreground process is playing audio | videos/streams with sound, regardless of input | muted video | Core Audio session query in the probe (COM) |

Plan: start with the long usage threshold, which is core-only and can be re-computed from MVP logs.
Add the audio signal as an optional `fg_audio_active` sample field (additive, schema stays
v1); the core then treats "foreground is playing audio" as present. Per-process "keep display
on" requests (what `powercfg /requests` shows) would also work but need admin rights,
so they don't fit the per-user collector.

Rules:
- Lock screen or no foreground window: not usage (already logged: `fg_class: none` or `LockApp.exe`).
- Session switched away (`console_active: false`): not usage.
- Browsers: identify the site by the window title, which is the active tab's title. Audio from a
  background tab does not count, because it isn't in the foreground.

Config: `idle_threshold_seconds` stays the gameplay threshold; add
`usage_idle_threshold_seconds`. Apps that match no rule still accumulate usage, keyed by exe,
so unknown apps show up in reports. The current `allowed` bucket becomes simply app usage
for those apps, and `class` later decides which budget an app's usage counts against.

Limiting non-game apps later has its own problems: closing a browser to stop one site is too blunt.
Design that when we get there.

## Post-MVP: measurement accuracy

### Controller (gamepad) input

**Why:** `GetLastInputInfo` sees only keyboard and mouse. A child playing with a controller
looks idle after 30s, so active play is under-counted. The MVP assumes keyboard and mouse only.

**Design:**

- **Probe (`windows/internal/win32`):** poll `XInputGetState` (`xinput1_4.dll`, Windows 8+)
  for user indices 0–3 on each tick. XInput does not need focus, so polling from the
  background collector works.
- **Activity detection:** a controller counts as active when buttons or triggers change, or a
  stick moves beyond its dead zone (`XINPUT_GAMEPAD_LEFT_THUMB_DEADZONE` etc.) compared with
  the previous tick. Do **not** use `dwPacketNumber` alone: analog stick drift changes it
  constantly, so a worn controller would never look idle.
- **Sampling caveat:** polling at the 5s sample interval misses short presses between ticks.
  Poll the controller faster (e.g. every 250 ms) in a small loop inside the probe, and report
  the time of the last detected change.
- **Record:** add an optional `pad_idle_s` to `sample` (seconds since last controller
  activity; absent if no controller is connected). This is additive, so schema `v` stays 1
  and old logs remain valid.
- **Core:** effective idle = `min(idle_s, pad_idle_s)` when `pad_idle_s` is present. Tests:
  synthetic logs with controller-only play, drifting sticks, controller unplugged mid-game.
- **Coverage gap:** XInput covers Xbox controllers and anything mapped to XInput (e.g. via
  Steam Input). Native PlayStation/Switch controllers need Raw Input/HID or
  `Windows.Gaming.Input`. Decide once we know which controllers are in the house.

### Idle-menu filtering

GPU/network activity to tell "sitting in a menu touching the mouse" from playing.
Only if the evening test shows it's needed.

## Later phases (not scheduled)

- Hardened collector: system service, tamper-resistant logs.
- Per-child shared budget, 5-minute buckets, parent-redeemed time tokens.
- Enforcement (warnings, then closing metered games).
- Central budget service (`service/`) and iOS client (`ios/`).
