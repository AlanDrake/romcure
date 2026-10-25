# UX roadmap

Status: proposal, 2026-10-24. The order and outline of the user-facing work
after the Games screen. Each item gets its own plan when it's picked up; the
Games screen already has one (`docs/game-selection-plan.md`).

## Rules for every screen

- Words from the user's side: outputs, games, versions, "your choice". Link
  tiers, game keys and grouping sources sit one click away (a drawer or a
  Details line), never in the main view.
- Every automatic decision says why, next to where it shows (spec §5).
- Choices save at once. Changes on disk happen on an explicit action that
  shows what it will do first (Apply, Sync, Export).
- Bulk actions get Undo, not confirmation dialogs. Only irreversible disk
  actions confirm (deleting an output's links, clearing the cache), and they
  confirm inside the page.
- One background job at a time, visible on every tab.

## 1. Games screen: planned

`docs/game-selection-plan.md`, phases 0–4.

## 2. Outputs tab

Pairs with phase 4 of the Games plan.

- One card per output: kind, folder, file system, games included, last
  applied, state (up to date / N changes waiting / problems), and actions
  Apply, Choose games…, Open folder, Edit, Remove.
- Edit the name and folder. A new folder means regenerating there and offering
  to remove the old links.
- Remove asks "Also delete the links this output made? Your games are not
  touched." (default yes) and always deletes the output's `link_state` rows.
  This fixes STATUS bug 15.
- A problems list per output in plain words (broken link, source missing,
  file over FAT32's 4 GB, conversion failed), with Repair (`plan.Repair`
  exists; no UI yet).

## 3. Jobs and progress

- A status strip on every tab while a job runs: what it's doing, n of N, the
  current item, elapsed time, Cancel.
- Backend: progress events gain Done/Total; conversions report percent
  (chdman prints it).
- When a job ends: a toast with the outcome and a link to details (sync
  report, problems).
- Cancel must stop a running conversion (STATUS bug 2) before this ships.

## 4. First-run setup (spec M6 onboarding)

A short wizard that writes the same records as the normal screens and can be
reopened from Help.

1. **Welcome:** what the app does in three lines; "Your files are only read,
   never changed."
2. **Where your games are:** pick one parent folder (or several). The app
   proposes a system for each subfolder from its name (aliases such as
   "snes", "Super Nintendo", "SFC", No-Intro/Redump/libretro names) and from
   file extensions (.sfc/.smc → SNES, .gba → GBA; .cue/.chd/.iso need a
   choice). A table to confirm or change each row; folders it can't place are
   listed, not dropped. Today each folder needs its system picked from a long
   list.
3. **Preferred versions:** region preset with a concrete example ("Super Mario
   World: you'll get the USA version").
4. **Where games should go:** detect RetroArch (default install paths and its
   config), ES-DE (portable folder or ~/ES-DE) and removable drives
   (FAT32/exFAT), and propose an output. SD cards start with no games
   (Games plan, decision 5).
5. **Links on Windows:** explain once, with the actual state. Developer Mode on:
   nothing to do. Off: "Windows will ask for administrator rights once", or
   "copies will be used". Needs the elevation wiring (STATUS bug 1).
6. **Game databases (optional, online):** one button, skippable.
7. **First sync** with progress, ending on the Games screen.

## 5. Library tab

- Source rows with system, folder, games found, last scan, warnings ("12 files
  not recognized" → opens Games filtered to unknown files).
- Sync report numbers link to the Games screen with filters set.
- Storage panel: sources, cache and outputs sizes, Clear cache
  (`MeasureStorage` and `Cache.Clear` exist; spec M4 item without UI).

## 6. Settings and Help

- Group settings: Versions (region preset, later a custom order), Conversions
  (parallelism, codecs, scratch folder), Links (hard links, Windows
  elevation), Data (cache location once STATUS bug 3 is fixed).
- Third-Party Licenses screen and source-offer page (spec §7) under Help.

## 7. Polish (low priority)

- Short system names everywhere (add `Short` to `profile.SystemInfo`: NES,
  SNES, PS1…); the Games screen needs them first.
- A light theme that follows the OS (colors are already tokens).
- Product name (still the placeholder).

## Suggested order

1. Games screen, phases 0–3.
2. Outputs tab together with Games phase 4.
3. Jobs and progress (after STATUS bug 2).
4. First-run setup (after STATUS bug 1).
5. Library tab, then Settings and Help, then polish.

STATUS bug 3 (caches in the roaming profile) must be fixed before any public
build, whatever the order above.
