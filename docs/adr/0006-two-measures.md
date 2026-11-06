# 0006. Two measures: app usage and active gameplay

- Status: Accepted
- Date: 2026-11-06

## Context

The owner wants to limit active game play, but also count passive screen time such as
YouTube, where there is no input while a video plays. Longer term, other apps should be
tracked and limited too.

## Decision

The product has two separate measures per child:

1. **App usage:** foreground time while the child is plausibly using the app, including passive use.
2. **Active gameplay:** a metered game's player in the foreground with input within 30s.

For a game, active gameplay is never more than its usage. The MVP implements active gameplay
only, for Roblox and Minecraft first. App usage comes after the MVP:

- first a long usage idle threshold, computed from existing logs in the core only;
- then an optional per-process audio signal (`fg_audio_active`).

Design details are in `docs/roadmap.md`.

## Alternatives considered

- **One measure (input-based):** under-counts videos.
- **One measure (foreground-only):** over-counts abandoned screens, and can't tell playing
  from a game left open.

## Consequences

The current `allowed` figure, which requires input, will be replaced by app usage. Detecting
"video playing" needs a new probe signal (Core Audio). Limiting non-game apps, such as single
sites in a browser, needs its own design.
