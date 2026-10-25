# Plan: choosing which games go to each output

Status: designed 2026-10-24, nothing implemented. Owner decisions are in
`docs/adr/0001-per-output-game-selection.md`. A clickable mockup with made-up
data is in `docs/mockups/games-screen.html` (open it in a browser; also
published privately for the owner at
https://claude.ai/artifact/WQ51qaLqjhAYrQxKBcSE9D). It is the visual and
behavioral reference for phase 3: the selection rules, undo, pending counts
and size meter in its script are a working model of what the backend must do.

## Goal

On the Games screen the user searches by name, filters by system and a few
facets, and ticks which games go to each output. It must stay fast at 100k
games (full TOSEC sets get there), explain every inclusion and every version
pick (spec §5), and never rescan source folders because of a tick.

## Decisions

1. **A tick means "this game is on this output".** Each output keeps its own
   ticks. The version choice (region/revision) stays library-wide, as spec
   §2.1/§2.7 require: changing it affects every output.
2. **Choices save at once; output folders change on Apply.** Ticks and version
   picks are stored immediately. Folders change only when the user presses
   Apply (or runs Sync). Apply regenerates outputs from the database and does
   not rescan source folders. Version picks stop starting a full Sync (today
   `Service.PickVariant` and `ClearPick` call `StartSync`).
3. **Inheritance decides new games.** A game's state on an output is the first
   that exists of: the user's tick on that game, the user's tick on its
   system, the output's default. Clicking a system box sets the system and
   clears the game ticks inside it; clicking "All systems" sets the default
   and clears everything below. So:
   - PC output (default include): new games appear automatically, except in
     systems the user unticked; games the user unticked stay out.
   - SD card that started empty, with GBA ticked and 40 SNES games picked by
     hand: new GBA games are added, new SNES games are not.
   There is no separate "add new games" setting. The drawer and the rail
   footer state the rule in words for the current output.
4. **Only exceptions are stored.** Ticking a game back to the state it would
   inherit deletes its row.
5. **New outputs:** "SD card / USB (copy)" starts empty (default exclude); the
   user ticks what fits. Other kinds start with everything. The add-output form
   shows "Start with: all games / no games" with the kind's default selected.
6. **Existing outputs migrate unchanged:** default include. A non-empty
   `profiles.systems` becomes default exclude plus an include row per listed
   system (same result as today). After migration nothing reads
   `profiles.systems`.

## Screen

```
┌─────────┬──────────────────────────────────────────────────────────────────┐
│ nav     │ Games  11,400 games            Choosing for [Miyoo SD ▾] G:\Roms │
│         │┌──────────────────┐┌───────────────────────────────────────────┐ │
│         ││▣ All     1.2k/11k││ [⌕ Search games                        /] │ │
│         ││☑ GB       820/820││ [All|Included|Not included] (Several      │ │
│         ││☑ GBA    1.4k/1.4k││  versions) (Your choices) (Not verified)  │ │
│         ││▣ SNES    24/2,240││ ▣ Game  2,240 shown · 24 included  Ver Size│ │
│         ││☐ PS1      3/2,520││ ☑ Chrono Trigger        USA        ✓ 4 MB │ │
│         ││ …                ││ ☐ Crystal Knights II    Japan · 2v ✓ 1 MB │ │
│         ││ New games are    ││ …(virtualized, one line per game)…        │ │
│         ││ added to GB, GBA ││                          ┌drawer─────────┐│ │
│         │└──────────────────┘└──────────────────────────┴───────────────┘┘ │
│         │ +5 to add · 1 version change   [meter] 18 GB of 59 GB  Undo Apply│
└─────────┴──────────────────────────────────────────────────────────────────┘
```

Regions:
- **Header:** "Choosing for [output ▾]" plus the output's folder and kind.
  With one output, plain text "Games in <name>". With no outputs, no boxes and
  a hint to add one; browsing and version picks still work.
- **Systems rail (~216 px):** "All systems", then one row per system with a
  source: tri-state box, short name, long name (muted, truncated),
  "included/total". Clicking the name filters the list to that system
  (Ctrl/Cmd-click adds more); clicking the box includes or drops the whole
  system on this output. Footer: one sentence on what happens to new games on
  this output, generated from the state.
- **Toolbar:** search box; Show [All | Included | Not included]; toggles
  Several versions, Your choices (a tick of yours on this output or a version
  you picked), Not verified; "Clear filters" while any is on.
- **List header:** tri-state box for everything the filter shows, "2,240 shown
  · 24 included", and when a search has hits outside the selected systems,
  "12 more in other systems" (click to search all). Sortable Game / System /
  Size.
- **Rows (fixed 34 px, virtualized):** box (with a small dot when the tick is
  the user's own rather than inherited) · name with search hits highlighted ·
  chips ("3 versions", "your version", "4 discs", "new") · system short name
  (hidden when one system is shown) · picked version tags · identification
  icon (✓ verified, ≈ by name, ? unknown; tooltip in words) · size (≈ when a
  disc will be converted).
- **Drawer (~380 px, over the list's right side):** "On <output>" box with the
  reason ("Ticked by you.", "Included because GBA is ticked. New GBA games are
  added too.", "Left out: Miyoo SD only takes the games and systems you
  tick.") and "Follow the <system> setting instead" when the tick is the
  user's; the other outputs with their own boxes; "Version — used on every
  output" as radio buttons with the automatic pick labelled and the selection
  reason; "Back to automatic"; identification, grouping method, source file,
  output path.
- **Changes bar (bottom, always shown):** for the current output "+12 to add ·
  −3 to remove · 1 version change" or "Miyoo SD is up to date"; for device
  outputs a size meter: selected size, free space after Apply, card size and
  file system; amber above 90 %, red above 100 % with "Doesn't fit: … Untick
  X to apply" and Apply disabled. When other outputs have changes too
  (version picks affect all): "Also changed: PC RetroArch · Apply all".
  Buttons: Undo, Apply to <output>.
- **Toasts** after bulk actions, with Undo.

Interactions:
- Search: debounced ~120 ms; ignores case, accents and punctuation; words in
  any order; matches the game name and every version's title (DAT name or
  file name). "zelda past" finds "Legend of Zelda, The - A Link to the Past";
  "pokemon" finds "Pokémon".
- Header box: includes every shown game unless all are included, else drops
  them all. It acts on everything the filter matches, not only the rows on
  screen; its label carries the count.
- Shift-click a row box (or Shift+Space): applies that row's new state to the
  range since the last row ticked.
- Under Show Included / Not included, a row the user just ticked stays in
  place until the filter changes (no rows jumping away mid-review).
- Keyboard: ↑/↓ move, Space ticks, Enter opens the drawer, / focuses search,
  Esc clears the search or closes the drawer.
- Every bulk action (header box, system box, All systems, range) gets an Undo
  toast; no confirmation dialogs. The bar's Undo undoes the last action of any
  kind, including version picks.
- Version change in the drawer: stored at once; the row updates at once; every
  output that includes the game shows a pending version change.
- **Multi-disc games are one row** ("Legend of Dragoon · 4 discs"). Today
  each disc is its own group (`library.Sync` puts the disc number in the
  grouping title), so the browse layer folds disc groups of one system by the
  `ParseDiscTag` base title, the same way `profile.partitionByDisc` builds
  `.m3u` sets. A tick applies to every disc. Picking a version for a set picks
  the version with the same region/language/revision on every disc; a disc
  without a match keeps its own pick and the drawer says so.
- Apply runs a background job ("apply") with progress in the bar; the list
  stays usable; ticks made meanwhile become new pending changes.
- Empty states: no outputs (hint); no games ("Run Sync on the Library tab");
  nothing matches ("No games match "xyz" in SNES." with Search all systems /
  Clear filters).
- Windows under 900 px: rail turns into a horizontal strip, System and Version
  columns hide, drawer takes the full width.

## Data model (migration 0008)

```sql
ALTER TABLE profiles ADD COLUMN include_by_default INTEGER NOT NULL DEFAULT 1;

-- A system-level tick on one output (absent = follow include_by_default).
CREATE TABLE output_systems (
    profile_id INTEGER NOT NULL REFERENCES profiles (id) ON DELETE CASCADE,
    system_id  TEXT NOT NULL,
    included   INTEGER NOT NULL,
    PRIMARY KEY (profile_id, system_id)
);

-- A game-level tick on one output, stored per variant game_key (content
-- based, spec §2.11) so it survives DAT updates that rename or regroup games.
CREATE TABLE output_games (
    profile_id INTEGER NOT NULL REFERENCES profiles (id) ON DELETE CASCADE,
    system_id  TEXT NOT NULL,
    game_key   TEXT NOT NULL,
    included   INTEGER NOT NULL,
    updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    PRIMARY KEY (profile_id, system_id, game_key)
);
```

- Ticking a game writes one row per variant key of its group (of every disc
  group for a multi-disc set). A group's decision is the newest row among its
  variants' keys, so a group that a DAT import splits or merges keeps the
  user's choice; on conflicting rows the newest wins.
- Resolution: game row, then `output_systems` row, then `include_by_default`.
- Foreign keys are enforced (`PRAGMA foreign_keys = ON` in `store.Open`), so
  deleting a profile drops its ticks.
- **Overrides have the same weakness today:** `overrides` is keyed by
  `group_id`, so a DAT import that changes group keys (title groups becoming
  P/C groups) orphans them. Fix in phase 1: when `library.Sync` builds a
  group, also accept an override whose `override_game_key` is one of the
  group's candidates, and move it to the new group id.

## Backend

- **Browse index** (proposed package `internal/catalog`, no Wails): built from
  the DB after Sync/Apply and refreshed on version picks. One compact record
  per row (disc sets folded): group ids, system, display name, normalized
  search text (name + every variant title, lowercased, accents folded,
  punctuation as spaces), version count, picked tags, confidence, overridden,
  size, variant keys. Ticks resolve on the fly from the decision rows (small).
  Filtering, sorting and paging run in memory. Simpler than equivalent SQL and
  fast enough; 100k records is roughly 30 MB.
- **Size per row:** source size for passthrough (`files.size`), member size for
  extraction (`archive_members.size`), cached CHD/RVZ size when converted,
  otherwise the uncompressed size flagged as an estimate.
- **Service API** (thin wrappers over `internal/app`):

```go
type GameFilter struct {
	OutputID   int64    // 0 = no output context (no boxes)
	Systems    []string // empty = all
	Query      string
	Show       string   // "all" | "included" | "excluded"
	Several    bool     // only games with more than one version
	Choices    bool     // only games with a tick of yours or a version pick
	Unverified bool     // only games not verified by a DAT
	Sort       string   // "name" | "system" | "size"
	Desc       bool
}
type GameRow struct {
	Key            string // stable row id (group id, or "set:<system>:<base title>")
	SystemID, Name string
	Version        string // picked variant's tags
	Versions, Discs int
	Confidence     string
	VersionPicked  bool // override set
	Included       bool
	Explicit       bool // the tick is the user's, not inherited
	Size           int64
	SizeEstimated  bool
}
type GamePage struct {
	Total, Included int
	OtherSystems    int // search hits outside Systems
	Rows            []GameRow
}
func (s *Service) Games(f GameFilter, offset, limit int) (GamePage, error)
func (s *Service) SystemCounts(outputID int64) ([]SystemCount, error) // per system: total, included, state; plus the rule sentence
func (s *Service) Game(key string, outputID int64) (GameDetail, error)  // versions, reasons, discs, per-output inclusion, paths

type Target struct { // exactly one is set
	All      bool
	SystemID string
	Keys     []string
	Filter   *GameFilter // everything the filter matches
}
func (s *Service) SetIncluded(outputID int64, t Target, included bool) (Change, error) // Change{Token, Affected, Message}
func (s *Service) FollowSystem(outputID int64, key string) error // drop the user's tick on one game
func (s *Service) Undo(token string) error
func (s *Service) Pending(outputID int64) (Pending, error) // Add, Remove, Replace, bytes, device free space, Blockers
func (s *Service) StartApply(outputIDs []int64) error      // job kind "apply"
```

- **Generation:** `generateLayout` and `PlanDeviceExport` take the output's
  included games (e.g. a `Games []store.SelectedGame` field filled by
  `app.generate`) instead of calling `db.SelectedGames(systemID)` themselves.
  `pruneOrphans` then removes unticked games' outputs with no other change.
  `app.Sync`/Apply must also run generation for systems that have no included
  games but still have `link_state` rows on that output, so dropping a whole
  system removes its links, and delete its RetroArch `.lpl` (not tracked in
  `link_state` today).
- **Apply:** for each chosen output with pending changes, generate every
  affected system, then verify. No scan, no `library.Sync`.
- **PickVariant:** store the override, set `games.is_selected` for that group
  directly (the override wins anyway), refresh the catalog row; no job.
  **ClearPick:** run `library.Sync` for that one system (it reads the DB and
  cue sheets, not the folder tree) to get the automatic pick back, then
  refresh the catalog. If that is slow on a 10k-game system, store the
  missing rule inputs (variant penalty; beta/proto/demo separately) on `games`
  and reselect one group.
- **Pending:** compare the output's included, selected games with its
  `link_state` rows by game_key: added (no rows), removed (rows, not
  included), replaced (same output name, different key; one change). Bytes
  for device outputs; blockers from `PlanDeviceExport` (free space, FAT32
  4 GiB).
- **Undo:** `SetIncluded` keeps the previous rows of everything it touches
  (game rows, system rows, default) in an in-memory ring of the last 20
  changes and returns a token. Version picks get tokens too. Undo doesn't need
  to survive a restart.

## Frontend

Split `App.svelte` (843 lines) before adding the screen:

```
src/App.svelte                 shell: nav, tabs, messages
src/lib/state.svelte.ts        shared state (State, job, progress, notices)
src/lib/VirtualList.svelte     fixed-height windowed list, no new dependency
src/lib/Toast.svelte
src/views/Library.svelte  Outputs.svelte  Settings.svelte  Help.svelte
src/views/games/Games.svelte        header, output picker, layout
src/views/games/SystemRail.svelte   tri-state systems
src/views/games/GameList.svelte     toolbar, header row, rows
src/views/games/GameDrawer.svelte
src/views/games/ChangesBar.svelte   pending, size meter, Undo, Apply
```

- The list fetches pages of 200 rows from `Games()` as they scroll into view,
  caches them by page, shows placeholder rows while loading, and resets on a
  filter change.
- Lift `main { max-width: 880px }` for this screen; it needs the full width.
- Keep the app's words: "your choice", "picked", "Back to automatic".

## Performance targets

On a synthetic DB of 100k groups across 30 systems, generated by a Go test
helper (no real ROMs, HANDOFF rule):
- `Games()` first page < 150 ms; a search keystroke to rows < 100 ms.
- `SystemCounts` < 100 ms; `Pending` < 200 ms.
- `SetIncluded` on a filter of 50k games < 1 s.
- Catalog rebuild after Sync < 2 s.
- The list scrolls without dropped frames (fixed row height, no layout reads
  while rendering).

## Phases

Each phase ends with `go vet ./... && go test ./internal/...`, the cross-OS
vet, `npm run check`, Playwright, and a STATUS.md update.

0. **Split the frontend** with no behavior change. Exit: e2e unchanged and
   green.
1. **Selection model.** Migration, resolution, generators use it, pruning of
   unticked games and dropped systems, device export counts only ticked games,
   override lookup by content key, and the RemoveProfile cleanup (STATUS bug
   15). Tests:
   - inheritance table (default × system × game);
   - a file added after a sync follows its system/default;
   - ticks and overrides survive a DAT import that turns title groups into
     P/C groups;
   - unticking a game removes its link; dropping a system removes its links
     and its `.lpl`;
   - Apply twice: zero filesystem writes the second time (idempotency rule);
   - removing an output deletes its `link_state` rows (and, if asked, its
     links); a new output never inherits them.
2. **Browse API:** catalog, `Games`/`SystemCounts`/`Game`, `SetIncluded` for
   every target kind, `FollowSystem`, `Undo`, `Pending`, the apply job, the
   quick `PickVariant`/`ClearPick`. Tests for each filter (word order, accents,
   punctuation, version titles), disc-set folding, undo, pending counts;
   benchmarks for the targets above.
3. **Games screen** per the mockup. Playwright: search narrows; system name
   filters; system box includes all; tick, the bar shows +1, Apply, and the
   output folder holds exactly the ticked games; header box under a filter;
   Shift-click range; Undo toast; switching output shows that output's ticks;
   a version change marks every output; device size meter and the "doesn't
   fit" block. Screenshots at 1100×740 and at 700 px wide, checked by eye.
4. **Outputs tab and links into the screen** (see `docs/ux-roadmap.md` §3):
   per output, included/total, pending changes, size and free space for
   devices, "Choose games…" (opens Games on that output), Apply; "Start with:
   all games / no games" when adding; Remove asks whether to delete the links
   the output made (never the user's games). Sync report numbers link to the
   Games screen with filters set.
5. **Later, unscheduled:** paste a list of names to tick (fuzzy match, review
   before applying); "same games as <output>"; region, language and type
   facets; automatic rules for unlicensed/pirate/aftermarket/BIOS with
   reasons; a compare view with one column per output.

## Risks and open points

- A DAT import that splits a group gives both halves the old tick; merging
  groups with different ticks keeps the newest. Acceptable, and the drawer
  shows which tick applied.
- "Check CHD files" changes a CHD's game_key (its content hash becomes the
  track hash), which drops its ticks and override. When the check updates a
  file, carry its decisions from the old key to the new one (same file_id).
- Output name collisions (STATUS bug 4) get likelier once people curate; fix
  them in or before phase 1.
- Device sizes for unconverted discs are upper bounds (STATUS bug 1); the
  meter labels them "≈" and the blocker must not fire on an estimate alone
  without saying so.
