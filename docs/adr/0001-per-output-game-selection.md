# 0001: Games are chosen per output; versions stay library-wide

Date: 2026-10-24. Status: accepted by the owner.

## Context

Users want to search the library and tick which games go into their outputs.
The spec covers which *version* of a game is used (overrides apply to every
profile, §2.1 and §2.7) but not which *games* an output gets. Profiles only
have a system list, which the UI doesn't expose. The most common reason to
curate is a handheld's SD card, which can't hold a PC-sized library, while the
PC output should keep everything.

## Decision

- Inclusion is chosen per output. A game's state on an output is the first that
  exists of: the user's tick on the game, the user's tick on its system, the
  output's default. New games therefore follow their system or the default.
- The version choice stays library-wide.
- Ticks and version picks are saved at once. Output folders change only on
  Apply or Sync; Apply regenerates outputs from the database without rescanning
  source folders.
- Ticks are stored per variant `game_key` (content based), so they survive DAT
  renames and regrouping.

Details and phases: `docs/game-selection-plan.md`.

## Alternatives rejected

- **One include list for every output:** can't express "everything on the PC,
  a few hundred games on the SD card".
- **Named collections that outputs subscribe to:** more flexible, but one more
  concept to learn; "same games as <output>" covers the shared case later.
- **Per-output version choice:** contradicts spec §2.1, and nobody asked for it.
- **Updating outputs automatically after each change:** SD-card copies and CHD
  conversions would start on their own, and today's full Sync per click is too
  slow for hundreds of ticks.
- **Ticks keyed by group id:** lost when a DAT import regroups games (the same
  flaw the `overrides` table has today).

## Consequences

- New tables `output_systems` and `output_games`, and
  `profiles.include_by_default`; `profiles.systems` is migrated and retired.
- Generators receive the output's game list; pruning removes unticked games.
- The app gains a pending-changes computation, an apply job, and undo.
- `PickVariant`/`ClearPick` no longer start a full Sync.
