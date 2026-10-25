# Plan: Redump DAT + SBI auto-download

Status: steps 1-6 implemented. internal/redump, SBI linking, multi-track fix, and opt-in CHD verification (internal/chdverify, ~25 s per 700 MB disc) are done and tested; verified end to end with 4 real Redump-exact FF8 Italy CHDs. RetroArch logged `[+sbi]` and `LibCrypt detected`; user confirmed gameplay and m3u disc swapping. No UI yet. Fits spec M5 "Redump + TOSEC DATs".

## What redump.info offers (probed 2026-10-23)

- `https://redump.info/downloads` is a static HTML table: one row per system
  with a version timestamp and links `/datfile/<SLUG>`, `/cues/<SLUG>` and, for
  only MAC, PC and PSX, `/sbi/<SLUG>`. About 100 slugs (PSX, PS2, PSP, SS, DC,
  MCD, PCE, 3DO, GC, WII, XBOX, ...).
- Each link answers with a direct `application/zip` and a
  `Content-Disposition` filename that carries the entry count and a timestamp:
  `Sony - PlayStation - Datfile (10982) (2026-10-23 10-18-40).zip`. No auth,
  no API key. `robots.txt` disallows only `/api/`, `/discs?`, login/edit paths.
- The datfile zip holds one Logiqx XML `.dat` (PSX: 13 MB, 10 982 games), with
  a `<header><version>` timestamp, and per game `id`, `category`, `serial` and
  one `<rom>` per file: the `.cue` plus every `.bin` track, each with size,
  crc, md5 and sha1. There is **no** `cloneof` anywhere (confirms spec §2.9).
- SBI zips (PSX: 235 files) hold `<game name>.sbi` named exactly like the DAT
  game name. They carry the LibCrypt subchannel data some emulators need.
- The HTML table, not an API, is the only listing, so slugs/versions must be
  scraped. Keep the parser tolerant and fail loudly if the layout changes.

## Design

New package `internal/redump` (no Wails, per HANDOFF rule):

1. **Catalog** `FetchCatalog(ctx)`: GET `/downloads`, parse rows into
   `{Slug, Name, Version, HasCues, HasSBI}`. Pure HTML parsing, tested against
   a saved fixture in `testdata/`.
2. **Slug to system_id map** (like `profile/libretro.go`): PSX to `psx`, PS2 to
   `ps2`, PSP to `psp`, SS to `saturn`, DC to `dreamcast`, etc. Partial on
   purpose; unmapped slugs are listed but not offered as auto-sync.
3. **Downloader** `Fetch(ctx, slug, kind)`: conditional and polite.
   - Skip if the catalog version equals the stored one (new column
     `dats.remote_version`, `dats.remote_slug`; migration `0002`).
   - Fixed User-Agent `romcure/<ver>`, one request at a time, no retries
     faster than 30 s, a size cap, and the zip read via `archive/zip` into a
     temp file on the cache volume, never inside a source root.
   - Atomic publish into `<appdata>/dats/redump/<slug>/`, keep the previous
     version until the new one imports successfully.
4. **Import** reuses `dat.ParseFile` + `dat.Import` unchanged (Logiqx XML, no
   P/C flag, so grouping falls back to title normalization as §2.9 says).
   Re-importing a newer DAT must preserve overrides (already tested in M2).
5. **SBI store** `<appdata>/sbi/psx/<DAT game name>.sbi` (only extracted from
   the zip, names sanitized, zip-slip checked).
6. **Profile use of SBI**: when a selected game has an SBI whose name equals
   the matched DAT game name, link/copy it next to the game file with the
   same basename as the linked game (`Game.sbi` beside `Game.chd`
   or `.cue`), also per disc inside `.discs/`. Recorded in `link_state` with a
   new `source_kind` value (`sbi`), and reported ("SBI added: LibCrypt").
7. **Settings/UX hooks** (service layer later): "Check for DAT updates" button,
   per-system toggle, last checked / version shown. Network is only touched on
   an explicit user action or an opt-in schedule, never silently at startup.

## Matching gaps to settle first

- **Multi-file games.** Redump entries are cue + N bins. `match.MatchFile`
  works per file; the game is "verified" only if *all* of its roms are present.
  Check how M1 aggregates this, and add a test with a 2-track disc.
- **CHD sources.** Many users (including the owner's sample set) already
  hold `.chd`. A CHD header has only the whole-disc raw SHA1, which is not a
  Redump hash. Options: (a) treat CHDs as heuristic tier only, (b) on demand
  `chdman extractcd` to scratch and hash the tracks (accurate, slow, uses the
  scratch dir). Recommend (a) by default plus an opt-in "verify against
  Redump" job through the background job system.
- **Content hash for `game_key`** (§2.11) should use the sorted track hashes,
  which this DAT format provides.

## Steps

1. Migration + `internal/redump` catalog parser and slug map (tests on fixture).
2. Downloader with version check, atomic extract, zip-slip tests, `httptest`
   server for all network tests (no real network in CI, per HANDOFF).
3. Wire import through to `dat.Import`; test the update path with renamed
   entries.
4. SBI extraction + profile linking + tests (PSX, with and without SBI).
5. Multi-track verified-match tests; decide the CHD policy.
6. Manual check on this machine: download the real PSX DAT and SBI zip to `R:`,
   import, and run a LibCrypt title in RetroArch with the SBI beside it.

## Open decisions

- Redump asks for no specific terms on the downloads page, but their site is
  volunteer-run: confirm we are fine bundling nothing and only fetching on user
  request, with attribution/link in the UI. (No DAT or SBI is committed to
  this repo.)
- CHD policy above (heuristic vs opt-in track verification).
- Which systems to map in the first pass (suggest PSX, PS2, PSP, Saturn,
  Dreamcast, Sega CD, PC Engine CD, GC, Wii).
