# Roadmap

v1 (MVP) is measurement only on Windows. Everything below is **planned for the product
but out of MVP scope**: don't build it until v1's evening test has passed.

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
