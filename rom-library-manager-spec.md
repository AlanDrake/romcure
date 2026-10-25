# ROM Library Manager (RomCure) — Build Specification (rev. 3, 2026-10-24)

> **Rev. 3 changes (2026-10-24):** product name **RomCure**; games are chosen **per
> output** with a Games screen (§2.12, `docs/adr/0001-per-output-game-selection.md`);
> several source folders per system (§2.13); RAR archives through the bundled 7-Zip
> (§2.8); background jobs with progress and cancel (§2.14); first-run setup; third-party
> license screen. **Bundled tools are the upstream projects' own unmodified releases,
> not built from source** (§1, §6, §7: owner decision). Checklists below carry what
> is verified; `docs/STATUS.md` is the live status.

> **Rev. 2 changes:** positioning vs. igir (§0); Wails v3 decision (§1); canonical
> library moved from filesystem to DB-only (§2.1); link tiers reconciled with the UAC
> decision, junctions dropped, reflinks added, hardlink write-through risk (§2.2);
> removable-media/exFAT export mode (§2.4); archive passthrough policy (§2.8);
> 1G1R parent/clone data source (§2.9); transcode corrections (§2.10); stable game
> identity (§2.11); license findings filled in (§7). Milestones updated accordingly.

## 0. Project summary

A cross-platform (Windows/macOS/Linux) desktop application that curates large ROM sets
(No-Intro, Redump, TOSEC, etc.) into clean, deduplicated, emulator-ready libraries
**without duplicating data**. It scans and hash-matches a read-only source library
against DAT files (optional, see §2.6), applies region/language/revision selection
rules (1G1R), and exposes the result to target emulators/frontends via links — never
modifying the source set. Disc images that aren't directly runnable (or are better
compressed) are transcoded to CHD/RVZ/CSO into a separate managed cache.

**Positioning.** The closest existing tool is **igir** (GPL-3.0, Node CLI): it already
does DAT matching, 1G1R, hardlink/symlink/reflink output, multi-disc playlists, and
per-frontend output layouts. It has no GUI and does not transcode disc images to
CHD/RVZ/CSO. This app's differentiators are therefore:
1. A GUI with transparent, explainable sync reports (why each game was picked/skipped)
2. Integrated, cached, verified disc-image transcoding
3. Persistent, incremental re-sync with manual overrides that survive DAT updates
4. Multiple simultaneous output profiles from one library

Agents should study igir's documented behavior (1G1R preference semantics, frontend
folder tokens) as a reference for expected behavior, but **must not copy its code**
unless the project is itself GPL-3.0-compatible and that is a deliberate decision.

**Non-goals:**
- General-purpose file manager
- Downloading ROMs or DATs from anywhere the user doesn't explicitly point it to
- Scraping box art / metadata (may hand off to existing tools later)
- BIOS/firmware distribution

---

## 1. Tech stack (decided)

- **Language/runtime:** Go
- **App shell:** **Wails v3**, pinned to an exact version. As of Sep 2026 v3 is still
  tagged pre-release (pinned: v3.0.0-beta.30) but its maintainers state the API is stable. Rationale
  for v3 over v2: v2 is in maintenance, v3 is where fixes land, and migrating later
  costs more than starting there. Pin the version in `go.mod`, upgrade deliberately,
  and keep all business logic in plain Go packages with no Wails imports so the shell
  is swappable.
- **Linux note:** target WebKitGTK **4.1** (Ubuntu 24.04+ and current distros no longer
  ship 4.0). Verify the AppImage/package on at least Ubuntu LTS, Fedora, and SteamOS
  (Steam Deck desktop mode — a key audience).
- **Frontend:** TypeScript + Svelte
- **Local DB:** SQLite via `modernc.org/sqlite` (pure Go, no CGO)
- **Hashing:** stdlib `hash/crc32`, `crypto/sha1`, `crypto/md5`
- **DAT parsing:** stdlib `encoding/xml` (Logiqx XML); also support ClrMamePro text
  format, which some DATs still ship in
- **Archives:** stdlib `archive/zip`; `github.com/bodgit/sevenzip` for 7z (pure Go); RAR through the bundled official 7-Zip as a subprocess (LGPL; read-only, extract-only)
- **Reflinks:** `unix.Clonefile` (macOS APFS), `FICLONE` ioctl (Linux Btrfs/XFS),
  `FSCTL_DUPLICATE_EXTENTS_TO_FILE` (Windows ReFS / Dev Drive) via `golang.org/x/sys`
- **External tools (invoked only as subprocesses via `os/exec`):**
  - `chdman` (MAME) — PS1/PS2/Saturn/Dreamcast/PC-Engine CD/PSP → CHD
  - `dolphin-tool` (Dolphin) — GC/Wii → RVZ
  - `7-Zip` (`7z.exe`+`7z.dll`, or `7zz`) — reads RAR archives (§2.8)
  - `maxcso` — PSP → CSO (optional, see §2.10; not built)
  - **Where they come from (rev. 3, owner decision: nothing third-party is compiled):**
    the projects' own unmodified releases, pinned by version and SHA-256 in
    `tools/versions.txt` and fetched by `tools/fetch-tools.sh`. Windows ships all
    three (MAME's and Dolphin's Windows releases, x64 and arm64). Linux and macOS
    ship 7-Zip only: MAME and Dolphin publish no chdman / dolphin-tool for them, so
    there the app finds them on the PATH (`mame-tools` or `rom-tools`, `dolphin-emu`).
    Lookup order: `<exe>/tools`, the repository's `tools/bin/<platform>`, the PATH.
- **CI/packaging:** GitHub Actions matrix (windows/macos/ubuntu), native build per OS
  (`ci.yml`: vet, tests, build, Playwright end-to-end on Linux, launch smoke test).
  `release.yml` builds, fetches the pinned tools, and packages a portable archive per
  OS with `tools/package.sh` (app, `tools/`, licenses, notices, `SOURCES.txt`); a
  `v*` tag also publishes a GitHub release.

---

## 2. Core architecture

```
[Source ROM Library]  (read-only, never modified)
        │
        ▼
  Scanner/Hasher  ──►  Hash Cache (SQLite)
   (reads CRC from zip/7z headers where possible)
        │
        ▼
  DAT Matcher  ◄── DAT files  +  parent/clone data (§2.9)
        │
        ▼
  Rule Engine (1G1R, overrides)  ──►  Canonical library = DB records (§2.1)
        │
        ▼
  Link Planner ── per game per profile: passthrough | extract | transcode
        │                              │
        ▼                              ▼
        │                     Transcode (background job)
        │                      Extract → Convert → Verify
        │                              │
        │                              ▼
        │                     Transcode Cache (content-addressed)
        └──────────────┬───────────────┘
                       ▼
              Link Executor (tiers, §2.2)
                       ▼
              Profile outputs (RetroArch / ES-family / raw / device export)
```

### 2.1 Canonical library is a DB concept, not a folder tree (changed in rev. 2)

Rev. 1 materialized a `by-system/<game-id>/` link tree and then linked profiles to it.
That doubles the link count, creates link chains (profile → canonical → source) that
hardlinks can't represent and that break if either layer moves, and gives users a
folder they might point an emulator at by mistake. Instead:

- The canonical library lives in SQLite: one record per selected game, with its system,
  identity, chosen variant, and its **ultimate source** (raw file, archive, or
  transcode-cache entry).
- Each profile links **directly to the ultimate source**. No intermediate tree.

```
<app_data_root>/
  library.db
  transcode-cache/          (content-addressed, see §2.10)
  scratch/                  (temporary extraction space; user-relocatable)
<profile_output_root>/       (chosen by user per profile, e.g. RetroArch roms dir)
```

`<system-id>` remains a fixed internal vocabulary (`nes`, `snes`, `psx`, `ps2`, `gc`, …);
display names and per-frontend folder names are mapping tables.

### 2.2 Link Executor tiers (reconciled with the UAC decision)

Decided per link request, in this order:

1. **Symlink** — when the process can create them (elevated via the runtime UAC
   relaunch, Dev Mode on, or always on macOS/Linux). Preferred: works cross-volume,
   visibly a link, breakage is detectable, emulator can't write through to the source
   by accident on most setups.
2. **Reflink / clone** — when source and destination are on the same CoW-capable
   volume (APFS, Btrfs, XFS, ReFS/Dev Drive). Zero extra space initially, and it's an
   independent file: writes never reach the source. On macOS this is a first-class
   option worth offering even though symlinks work.
3. **Hardlink** — same volume only. **Caveat:** a hardlink *is* the source file; any
   program that writes in place (some emulators patch ROMs, some tools "fix" headers or
   rename-in-place) modifies the source, violating §5. Use only when the user has
   explicitly opted in after a one-line warning, and never for files a target is known
   to write to.
4. **Copy** — always available; real disk usage; used automatically for export to
   removable media (§2.4).

Junctions are **dropped**: every link the app creates is a file link (folders in
profile outputs are real directories), and junctions only apply to directories.

Symlink paths: **relative** when source and target are on the same volume (survives
moving a drive/mount point together), **absolute** otherwise (relative links across
volumes are impossible on Windows and fragile elsewhere). Record which in `link_state`.

### 2.3 Elevation model (Windows) — unchanged

App launches unelevated. On first link request that needs a symlink and can't get one,
offer a runtime relaunch elevated (UAC). Decline/unavailable admin → fall through to
the next tier for that session; remember the choice. Native folder pickers only
(UIPI blocks drag-and-drop into elevated windows). Symlinks created by the elevated
process are usable by the normal user afterwards.

### 2.4 Destination filesystem rules (extended in rev. 2)

Detect the destination's filesystem type, not just whether it's remote:

| Destination | Behavior |
|---|---|
| Local NTFS/APFS/ext4/Btrfs/XFS/ReFS | Normal link tiers (§2.2) |
| Network share (UNC, SMB/NFS mount, remote mapped drive) | **Blocked** as profile output, with explanation (unchanged decision) |
| FAT32 / exFAT (SD cards, USB sticks) | No links possible on these filesystems → offered only as an **"Export to device" profile** that copies, with up-front size estimate and free-space check |

Rationale: preparing a curated 1G1R set on an SD card for a handheld (Steam Deck SD,
Anbernic/Miyoo/Retroid devices, MiSTer) is one of the most common reasons people want
this tool. Treating it as a deliberate copy-based export rather than a link-failure
fallback makes it a feature instead of a confusing degradation. Device export reuses
the same profile generators and `link_state` (with `link_type = 'copy'`), so re-export
is incremental (only changed files copied).

FAT32 also has a 4 GiB file limit — the export planner must flag games whose output
file exceeds it (common for PS2/Wii ISOs; CHD/RVZ usually fit).

### 2.5 Multi-disc and multi-file games

- `.m3u` per multi-disc game, **relative paths only**, discs in sibling `.discs/`
  (dot-prefixed + Windows hidden attribute). Regenerated per profile.
- **BIN/CUE sets are multi-file even for one disc:** the `.cue` references `.bin`
  files by name, so every member file must be linked with its exact original name
  into the same directory. Never link the `.cue` alone. (Converting to CHD removes
  this problem, which is a further reason to prefer CHD output.)
- Same applies to GDI + tracks (Dreamcast).

### 2.6 Operation without DAT files — unchanged

DATs are optional. Three confidence tiers per file (DAT-verified / heuristic-only /
unidentified), carried through sync report and profiles. Unmatched files can be
passed through 1:1 (opt-in, off by default).

### 2.7 `link_state` schema (updated: no canonical scope)

```
link_state
  id                 PK
  profile_id         text   (every row belongs to a profile, incl. device exports)
  system_id          text
  game_key           text   (stable identity, §2.11)
  target_path        text   (relative to the profile output root)
  source_kind        enum: 'raw_file' | 'archive' | 'archive_member_extracted'
                           | 'transcode_cache'
  source_path        text   (absolute)
  source_hash        text   (content hash at link time — drift detection)
  link_type          enum: 'symlink_rel' | 'symlink_abs' | 'reflink' | 'hardlink' | 'copy'
  is_manual_override bool
  disc_index         nullable int
  link_status        enum: 'ok' | 'broken' | 'stale' | 'pending'
  created_at         timestamp
  last_verified_at   timestamp
```

Diff algorithm unchanged from rev. 1, except it runs per profile only. Manual
overrides live on the canonical game record (DB), not per link, so an override
applies to all profiles at once.

### 2.8 Archive policy (new in rev. 2)

Most No-Intro cartridge sets are distributed as one `.zip` (or `.7z`) per game. The
app can't link to a file *inside* an archive, so each profile × system needs an
archive policy:

| Policy | Meaning | Typical use |
|---|---|---|
| **Passthrough** | Link the archive itself, renamed to the clean game name | RetroArch with cores that load zip; cartridge systems |
| **Extract** | Extract the single ROM to the transcode cache and link that | Standalone emulators that can't read archives; 7z for targets lacking 7z support |
| **Transcode** | Extract → convert → verify → cache | Disc images (§2.10) |

- Default: passthrough for `.zip` on cartridge systems in the RetroArch profile;
  extract for `.7z` unless the target is known to support it; transcode for discs.
- Passthrough only applies when the archive contains exactly the matched ROM (plus
  nothing else). Multi-ROM archives always extract.
- RAR (`.rar`, RAR5, solid, multi-part sets read from their first volume) is read
  through the bundled 7-Zip as a subprocess (`internal/rararc`): `7z l -slt` lists
  members and CRCs, `7z e -so` extracts one file. A RAR always extracts (never
  passthrough). Without 7-Zip a RAR is listed as unreadable and the rest syncs.
- Scanning reads member CRC32s from zip central directory / 7z / RAR headers instead of
  decompressing — this is the main lever for scan speed on large sets. Full-content
  hashing (SHA1) is only needed for verification or when a DAT lacks CRC.

### 2.9 1G1R parent/clone data (new in rev. 2 — corrects rev. 1)

Rev. 1 assumed DATs "group variants by game." That's only partly true:
- **No-Intro**: the standard DATs don't encode parent/clone; the separately offered
  **Parent/Clone XML** DATs do (`cloneof`). The app should detect which kind it got and
  tell the user if they've loaded a non-P/C DAT.
- **Redump**: DATs carry no parent/clone relationships at all.
- **TOSEC**: no P/C either.

So 1G1R needs a second data source when P/C is missing:
1. Use P/C relationships from the DAT when present (preferred).
2. Otherwise support importing **Retool clone lists** (community-maintained JSON
   mapping titles across regions — check its license before bundling vs. asking the
   user to download them).
3. Otherwise fall back to title-normalization grouping (strip region/language/revision
   tags), labeled heuristic in the UI exactly like §2.6.

The Rule Engine must record *which* of these three established each grouping, and the
sync report shows it.

### 2.10 Transcode details (corrected in rev. 2)

- **chdman mode depends on media**: `createcd` for CD systems (PS1, Saturn, Sega CD,
  PCE-CD, Dreamcast GDI); **`createdvd` for DVD/UMD systems (PS2 DVD titles, PSP)**.
  PS2 has a few CD-based titles — choose by detected media, not by system alone.
- **PSP**: PPSSPP fully supports CHD since 1.17 (created with `createdvd`), so CHD can
  be the default PSP target and CSO becomes optional for older/other targets. Keep
  maxcso in M5 only if a target profile actually needs CSO.
- **Codec pinning**: newer chdman versions support codecs (e.g. zstd) that older
  emulator cores may not decode. Pass explicit codec flags per profile rather than
  relying on chdman defaults, and include them in the cache key.
- **Cache key**: hash of the *uncompressed disc content* (track/ISO hashes from the
  DAT match), not of the source archive — so a re-zipped or re-7z'd source doesn't
  trigger a needless re-transcode. Key = content hash + format + tool version + flags.
- **Verification**: `chdman verify` / `dolphin-tool verify` after every conversion.
  Additionally, confirm the CHD's embedded raw-data SHA1 against the DAT where possible,
  so the output is proven to represent the verified dump.
- **Scratch space**: free-space check = largest uncompressed disc in the batch × number
  of parallel jobs. Default parallelism 1 for DVD-sized images.

### 2.11 Stable game identity (new in rev. 2)

`game_key` must survive DAT updates. No-Intro and Redump periodically **rename**
entries, so a key derived from the title would orphan overrides and links. Use:
- DAT-matched: hash of the sorted ROM/track content hashes of the selected variant,
  plus a separate "group key" for the 1G1R group (P/C parent's content hash, or
  clone-list id).
- Heuristic/unidentified: content hash of the file.
Display names are attributes that are allowed to change; keys are not.

### 2.12 Game selection per output (new in rev. 3)

Which *games* an output gets is the user's choice (ADR 0001). A game's state on an
output is the first that exists of: the tick on the game, the tick on its system,
the output's default; so new games follow their system or the default. The *version*
of a game stays library-wide (overrides, §2.1). Ticks are keyed by the variant's
`game_key` and saved at once; output folders change only on **Apply** (regenerates
outputs from the database without scanning) or **Sync**. The Games screen
(`internal/catalog`: in-memory index, search, filters, windowed list, 100k games)
shows state per output, a changes bar (pending add/replace/remove, size, SD-card
fit, Undo) and a details drawer (version choice and why). Outputs have a card on the
Outputs tab (rename, move, check and repair, remove with or without deleting links).

### 2.13 Several folders per system (new in rev. 3)

A system may have several source folders. They are scanned one by one and identified
together (`library.SyncRoots`): grouping and 1G1R work across all of them, and the
same content in two folders is one game, used from the folder added first. Folders
may not overlap each other or an output. Removing a folder forgets its games at once.

### 2.14 Jobs, progress and cancel (new in rev. 3)

One background job at a time (sync, apply, DAT update, CHD check). Progress is a
step counter (`n/N`) plus the stage, system, output and the file or game being
processed, thinned out before reaching the UI. Cancel stops the job at the next
game and kills a running chdman / dolphin-tool; a file that cannot be read (damaged
archive) is reported and skipped, never fatal. Closing the window while a job runs
asks first. Converted discs and extracted files live in a cache that the user may
move (default: the OS cache folder for new installs).

---

## 3. Roadmap & milestones

### M0 — Foundations
- [x] Wails v3 (pinned) + Go scaffold; CI matrix builds green on 3 OSes
      _(CI runs on every push: vet, tests, build, launch smoke test on all three, Playwright end-to-end on Linux; green as of 2026-10-24)_
- [x] Core packages free of Wails imports (`scan`, `dat`, `rules`, `plan`, `link`,
      `transcode`, `profile`), each with unit tests
      _(all implemented and tested; only `cmd/romcure` imports Wails)_
- [x] SQLite schema v1: `files`, `archive_members`, `dat_entries`, `games`,
      `game_groups`, `rules`, `overrides`, `link_state`, `transcode_cache`
- [x] Link primitives: symlink (rel/abs), reflink (per-OS), hardlink, copy
- [x] Capability detection: symlink probe, Dev Mode check, same-volume check,
      **filesystem-type detection** (remote / FAT32 / exFAT / CoW-capable)
- [ ] Runtime UAC relaunch flow with decline → fallback proven on a standard account
      _(relaunch confirmed live 2026-10-25 on a standard account: approved the real UAC
      prompt, the app relaunched elevated with symlinks working. Decline detection is
      implemented (ErrUACDeclined) but still needs a live human UAC *decline* to click
      through — unverified. Probe/detection side confirmed internally consistent on this
      standard, non-admin account both before and after enabling Developer Mode:
      SymlinkProbe/IsDevModeEnabled/IsElevated read false/false/false, then
      true/true/false — IsElevated correctly stays false since Dev Mode grants
      symlinks without admin)_
- [x] Structured logging to a user-accessible file

**Exit:** each link tier created and correctly chosen on all 3 OSes; FAT32/exFAT and
network destinations correctly classified.
_(Partially met. On Windows with Developer Mode on: symlink, hardlink, and copy all
verified for real; reflink still skips (this machine's NTFS volume isn't CoW-capable,
unrelated to Dev Mode). Linux/macOS builds are cross-compiled and vet-clean from this
machine but never executed. FAT32/exFAT/remote classification is unit-tested at the
string-mapping level only — not against a real FAT32/exFAT volume or network share.)_

### M1 — Scan, hash, match
- [x] Recursive scanner with progress, cancel, resume
- [x] Hash cache keyed by (path, size, mtime)
- [x] Read member CRC32 from zip/7z headers without decompression
- [x] DAT import: Logiqx XML + ClrMamePro text; detect P/C vs non-P/C No-Intro DATs
      _(rev. 3: also by hand from the Library tab, a .dat/.xml or a zip holding one, system detected from its name)_
- [x] No-Intro DATs for cartridge systems downloaded on request from libretro-database's
      copies (`internal/nointro`; a DAT imported by hand takes precedence)
      _(verified live: 7 systems in 2.6 s, unchanged re-check 42 ms; Sync verified 3,224 of
      the owner's sample ROMs; see docs/STATUS.md 2026-10-25)_
- [x] Matching with three confidence tiers (§2.6), incl. zero-DAT operation
- [x] Library browser: per-system counts, matched/heuristic/unidentified lists
- [x] RAR archives (§2.8) and several folders per system (§2.13)
      _(verified on the user's real RARs, incl. a 1.4 GB GameCube RAR converted to RVZ)_
- [x] A damaged archive or unreadable file is reported and skipped; the sync continues
- [x] Headered cartridge dumps (NES, FDS, Atari 7800, Lynx, SNES copier) match the
      headerless CRCs No-Intro lists (`internal/romheader`, the clrmamepro header-skipper rules)
- [x] GameCube/Wii discs that no DAT or file name identifies are named from their disc
      header ("Title (Region)", `internal/discid`)
- [x] PlayStation/PS2 discs likewise, from the serial in SYSTEM.CNF and libretro-database's
      serial lists (CC BY-SA 4.0, downloaded with the DATs, never shipped)

**Exit:** a real No-Intro cartridge set (zipped) scans and matches in minutes, not
hours; a folder with no DAT still yields a browsable library.
_(Partially met. All of the above tested against hand-written synthetic fixtures
(small raw files, a Go-generated zip, a real 7z built with actual 7-Zip since
bodgit/sevenzip can't write one, synthetic mini DAT text/XML) — per the "no real
ROMs/DATs" testing rule, nothing has been scanned/matched against an actual
No-Intro set, so the "minutes, not hours" performance claim is unverified. The
zero-DAT browsable-library path is verified. Scanner tested directly only on
Windows; cross-compiled and vet-clean on Linux/macOS from this machine but never
executed there.)_

### M2 — Rule engine & canonical library (DB)
- [ ] Grouping from P/C DAT, Retool clone list import, or title normalization (§2.9),
      with the source of each grouping recorded
      _(DAT P/C and title-normalization both done, with source recorded. Retool
      clone-list import has no importer yet — `rules.GroupingCloneList` exists as a
      type so wiring it in later needs no schema/call-site change, just the
      importer and one more branch in `rules.Group`)_
- [x] 1G1R preferences: region, language, revision, verified, exclude beta/proto/demo
      _(region/language are ordered priority lists; revision is a single
      PreferLatestRevision toggle rather than a priority list — DAT revision
      strings are open-ended, not a small fixed vocabulary like regions — that
      prefers the higher number extracted from strings like "Rev 1"/"v1.1";
      "Rev A"-style non-numeric revisions always lose to a numbered one)_
- [x] Manual overrides on canonical game records
- [x] Stable `game_key` / group key (§2.11); test by renaming DAT entries between runs
- [x] Sync report: selected / excluded / unmatched / overridden, with reasons

**Exit:** re-running with no changes is a no-op; loading a newer DAT with renamed
entries preserves overrides.
_(Met and directly tested — see TestSyncIsIdempotent and
TestSyncManualOverrideSurvivesDATRename in internal/library/sync_test.go. Tested
against synthetic fixtures only, same caveat as M1.)_

### M3 — RetroArch profile
- [x] system-id → libretro database name mapping
      _(~30 common systems, intentionally partial — see internal/profile/libretro.go)_
- [x] Archive policy (§2.8) with RetroArch defaults
- [x] Profile generator linking directly to sources; `.lpl` generation
- [x] Multi-file (cue/bin, gdi) linking; `.m3u` + `.discs/`
- [ ] End-to-end test in real RetroArch on all 3 OSes
      _(Partial, Windows only. Generated a real profile (synthetic NES ROM) with the
      actual code path and pointed an existing RetroArch install at it via an
      isolated `--config`, so the user's real settings/playlists/history were never
      touched. RetroArch started cleanly against that config with zero errors in its
      verbose log, including loading from our custom playlist_directory. Did **not**
      visually confirm the game actually appears correctly in the in-app playlist
      list. macOS/Linux RetroArch verification not attempted at all — no access to
      those OSes from this machine.)_

**Exit:** RetroArch launches single-disc, multi-disc, and zipped cartridge games from
the generated profile.
_(Not verified — see above. All four link/playlist code paths (raw, zip passthrough,
zip/7z extract, multi-disc .m3u) are tested against synthetic fixtures, but nothing
has actually been launched inside RetroArch.)_

### M4 — CHD transcoding
- [x] Ship `chdman` as MAME's own unmodified release (rev. 3; no source build)
      _(Windows x64/arm64 pinned and fetched with a hash check; Linux/macOS use the PATH. Compiling MAME's tools in CI was tried twice and dropped.)_
- [ ] License compliance for chdman completed (§7)
      _(notices, in-app license text and source pointers done; legal review pending)_
- [x] Extract-to-scratch with free-space check
      _(`CheckScratchSpace` plus `Job.Prepare`; the RetroArch profile extracts archived discs into scratch and converts them)_
- [x] createcd/createdvd chosen by media; explicit codec flags per profile
      _(media inferred from image size by `DetectMedia`; PSP forced to createdvd by system; no DAT-aware override)_
- [x] Conversions run in the background job with progress parsing and cancel; optional parallel pre-warm (`Conversions at once`)
      _(rev. 3: the separate pause/resume queue was removed as unused; Cancel kills the running tool, pausing was not needed)_
- [x] Content-keyed transcode cache; verify + SHA1 check before linking
      _(tested with a fake chdman, plus real chdman 0.289 on Windows; the RetroArch profile links the verified CHD from the cache. Booted and played without problems (user-confirmed) in RetroArch pcsx_rearmed on Windows from the symlinked cache CHD of a PS1 game; PSP: a 7z'd ISO became a createdvd CHD and also ran in RetroArch PPSSPP, user-confirmed. macOS/Linux unverified)_
- [x] Storage: cache folder choice (setup and Settings), scratch folder, `Cache.Clear`
      _(a size panel (`MeasureStorage`) exists in the backend but has no UI)_

**Exit:** 7z'd PS1 BIN/CUE and PSP ISO both become verified CHDs, linked into the
RetroArch profile, and boot.

### M5 — More targets & formats
- [x] ES-family profile (EmulationStation-DE, Batocera, RetroDECK, RetroBat folder maps)
      _(ES-DE folder names checked against a real ES-DE 3.5 install, and a generated tree was loaded by that ES-DE and the user confirmed a PSP game under PSP and a 4-disc game (one entry for its m3u) under PlayStation. Batocera/RetroBat/RetroDECK maps come from documentation only, never run against those frontends.)_
- [x] Raw-export profile
      _(`GenerateRaw`: folder per system id, clean names, m3u sets; tested)_
- [x] **Export-to-device profile** (FAT32/exFAT copy, incremental, 4 GiB check)
      _(`PlanDeviceExport`/`ExportToDevice`: remote blocked, free-space and size estimate, FAT32 limit enforced, size-based incremental copy; tested on NTFS and a RAM disk only, never on a real FAT32/exFAT card, and transcoded discs are estimated at their uncompressed size)_
- [x] Redump + TOSEC DATs
      _(Redump half: `internal/redump` catalog/download/sync + SBI linking done, see docs/redump-integration-plan.md; TOSEC: `internal/tosec` finds the latest release, downloads the ~100 MB Complete pack once, imports the DATs of one system (optionally by category), tested against a fake server and live; naming layer parses country/language/version/flags and ranks clean over modified. On the real TOSEC 2025-03-13 Amiga Games DATs, 52 of 67 ADF files in the owner's Amiga folder verified, 59 games linked into an ES-DE tree, none launched. Only computers are mapped (amiga, c64, zxspectrum, atarist, amstradcpc, msx, atari800, apple2); no UI)_
- [ ] RVZ via `dolphin-tool` (+ §7 compliance)
      _(Conversion verified on a real Wii WBFS and on a GameCube image from a RAR (lossless SHA1 check). dolphin-tool is Dolphin's own Windows release (2609); macOS/Linux need it on the PATH. Still open: a launch check of an RVZ in Dolphin (never confirmed), legal review of §7.)_
- [ ] CSO via `maxcso` only if a target needs it (+ §7 compliance)
- [x] Region preset templates
      _(`rules.Presets`: usa, europe, japan, world. Required fixing region/language parsing first: multi-region tags like "(USA, Europe)" and "(En,Fr,De)" were previously ignored and ranked last. Preferences are not yet persisted or exposed in any UI.)_

**Exit:** one library feeds RetroArch, an ES-family profile, and an SD-card export
simultaneously.

### M6 — Release hardening
- [x] Link verification pass (broken/stale detection) from `link_state`
      _(`plan.VerifyProfile` + `plan.Repair`: checks every recorded output for missing/dangling/redirected links, stale copies and sources whose hash changed after a rescan; Repair removes only flagged outputs inside the output root and the next sync recreates them. Unit tested on Windows; no UI.)_
- [x] Onboarding explaining link types, elevation, export mode
      _(first-run setup screen, 2026-10-24; see docs/STATUS.md for what is verified)_
- [x] Settings: scratch dir, parallelism, codec flags, elevation memory
      _(`internal/settings`: validated JSON, closed codec allowlist, scratch dir kept out of source roots and network locations, corrupt files preserved as .bad. Read by the app since 2026-10-24: parallelism, CHD codecs and elevation memory are used; see docs/STATUS.md.)_
- [ ] Signing: Windows Authenticode; **macOS Developer ID signing + notarization of the
      app *and every bundled tool binary*** (unsigned helper binaries are blocked by
      Gatekeeper when spawned)
- [x] Third-Party Licenses screen + source-offer page (§7)
      _(Help tab, generated data, 2026-10-24; release attachments of the GPL tool sources still to do, see docs/STATUS.md)_
- [x] Log bundle export
      _(`internal/support.ExportBundle`: redacted log tail + settings + system info, never the database; Help tab "Save diagnostics")_
- [x] User docs/FAQ
      _(`README.md` for GitHub, `docs/FAQ.md`, linked from the Help tab)_
- [x] Portable release archives per OS with the pinned tools, notices and SOURCES.txt
      _(`release.yml` + `tools/package.sh`; a manual run produced the three archives; the tag-triggered release step and a launch on a clean machine are unverified)_

**Exit:** signed builds on 3 OSes; new user reaches a working RetroArch library or SD
export using in-app guidance alone.

> **M7–M11 (added 2026-10-25)** come from the owner's review of a live sync of
> their collection. M7 fixes what that sync got wrong; M8–M11 are new work. Suggested order:
> M7, then M11's first item, M8 once the owner has chosen an option, M9, M10.

### M7 — Identification fixes from the owner's review
- [x] GBA, NDS and N64 dumps that no DAT checksum matches are named from their cartridge
      header: its game code ("ABCE") is looked up in the `serial` that libretro's No-Intro
      DATs list per game
      _(`discid/cartridge.go` checks each header's checksum; `dat_entries.serial` is
      migration 0015. Files marked as hacks or translations keep their own names. Checked
      on a copy of the owner's GBA folder: 32 of the 34 unmatched dumps are named, with
      2 left unidentified. NDS and N64 are tested on synthetic headers only.)_
- [x] Archive members that aren't games (patches, scene notes, extensionless files next to
      a ROM) are dropped, using per-system ROM extensions
      _(`match/romext.go`. The 4 stray members in the owner's GBA set are gone.)_
- [x] A C64 program made of several files (a PC64 `.P00` set, a PRG set) is one game: the
      archive
      _(`match/bundle.go`, zip and 7z only. On a copy of the owner's C64 set, two such
      archives went from 65 games and 51 games to 1 each, and the RetroArch output
      links the zip under the game's name. None has been launched in VICE.)_
- [x] TOSEC disk sides and tapes ("(Side B)", "(Disk 2 of 4 Side A)", "(Tape 1 of 2)") are
      discs of one game, linked together with one `.m3u`
      _(`library.ParseDiscTag`; sets are keyed without dump flags (`library.SetTitle`).
      For a 4-disk, 8-side game in the owner's set, all 8 sides are selected and go in
      one 8-line .m3u. It has not been launched.)_
- [x] A file whose kind changed since its last scan is scanned again
      _(on a copy of the owner's database, the 3 N64 RARs recorded as plain files read
      as RARs again)_
- [x] Zips that use old methods (implode, shrink) are extracted with 7-Zip and never passed
      through (Go and RetroArch can't read them; 23 of the 5,464 C64 zips)
      _(the method is recorded at scan time in `files.zip_method`, migration 0016; zips
      scanned earlier are filled in on the next scan from their central directory. Two
      of them, forced into a scratch output, extracted with 7-Zip; without 7-Zip the
      error names the method. A C64 multi-file zip that uses an old method is still
      linked as it is.)_
- [ ] Multi-disc games are chosen as a whole release (every disc from the same dump and
      region), not one disc at a time. Today a set can mix regions or cracks.
- [ ] Mega Drive serials: the header's "GM 00001009-00" against the DAT's several formats
      ("MK-1079", "T-48073-00", "00054503-00")
- [ ] PC Engine's low match rate (132 of 664) explained: the folder is a GoodTools-era set
      with "Sounds" files, PD demos and SuperGrafx titles

**Exit:** on the owner's folders, the unmatched GBA dumps get their No-Intro names, a
C64 game made of many files is one game, and every side of a multi-disk game is in one
playlist.
_(Met on copies of the owner's database and DATs, with a scratch output, then confirmed
working by the owner in their own app after a rebuild, Update DATs and Sync
(2026-10-25). None of these games has been launched.)_

### M8 — RetroArch playlists that RetroArch reads
The RetroArch output writes `<database name>.lpl` at its own root, and setup tells the
user to point RetroArch's playlist folder at it. That swaps out every playlist the user
already has (a RetroArch that has scanned games has its own playlists under these same
names, such as "Nintendo - Game Boy Advance.lpl"), so in practice the playlists go unused.
**The owner chose A (2026-10-25).** The options were:
- **A. Merge into RetroArch's playlists folder (recommended).** Read `retroarch.cfg`
  (`playlist_directory`, `libretro_directory`; `:\` means the RetroArch folder) and
  update `<database name>.lpl` there: entries whose path is in the output are ours and
  are rewritten; every other entry, and any core the user picked in RetroArch, is kept.
  The names match RetroArch's own, so system icons and thumbnails work. RetroArch rewrites
  playlists while it runs, so Apply must check that it is closed. `retroarch.cfg` itself
  is never edited (`config_save_on_exit` would undo it).
- **B. Separate "RomCure - <system>.lpl" files** in the same folder: nothing of the
  user's is touched, but each gets a generic icon and sits beside the user's own list for
  the same system.
- **C. No playlists: RetroArch scans the output** (Import Content → Scan Directory, or a
  Manual Scan, which RetroArch can refresh later). Nothing is written outside the output,
  but RetroArch's checksum scan misses the dumps RomCure named from headers or file
  names, and the user rescans after each Apply.
- **D. Keep today's behavior** and document it better.

With any option: a default core per system when a known one is installed (mGBA for GBA,
FBNeo for arcade), and portable paths (`base_content_directory`) for SD-card exports that
RetroArch on a handheld will read.
- [ ] Find the RetroArch install in use. Candidates, each needing a real `retroarch.cfg`:
      the folder of a running RetroArch (finds portable installs anywhere); the usual
      places (Windows: `%APPDATA%`, default folders, Steam, installer registry entry,
      shortcuts that point at `retroarch.exe`; macOS; Linux, Flatpak and Snap). When
      several are found, the running one wins, then the most recently changed
      `retroarch.cfg`; list them with one plain line each. Browse (exe, `.app` or
      `retroarch.cfg`) is always there, and the choice is kept per output. The playlist
      folder is resolved from `playlist_directory` (blank or "default" = `<config
      folder>/playlists`, `:\` = the RetroArch folder), shown plainly, and checked again
      at every Apply. A shortcut that starts RetroArch with `--config` is not covered.
      _(Done: `discover.ParseRetroArchConfig` and `discover.RetroArchInstalls` read the
      config from the usual places and rank by last change; unit-tested on synthetic
      configs, vet-checked for Linux and macOS. Not done: running-process detection,
      the registry and shortcut lookups, Browse, the setup and output UI. Not run
      against a real install yet.)_
- [ ] Playlist writer for the chosen option: no write when nothing changed; our entries
      removed when an output or system is removed
- [ ] Default core per system when installed
- [ ] Apply checks whether RetroArch is running

**Exit:** the owner launches a game from a RomCure playlist in RetroArch on Windows.

### M9 — One systems registry; the systems RetroArch and ES-DE support
What the app knows about a system lives in about eight hand-written maps (libretro names,
short names, folder aliases, file extensions, ES-family folders, Redump slugs, TOSEC
names, disc systems) covering about 35 systems. libretro-database has 92 No-Intro DATs;
ES-DE 3.x knows 195 systems.
- [ ] One registry (`internal/systems`): id, names, media class (cartridge, disc,
      computer, arcade), libretro database name, DAT source, ES-DE and Batocera folders,
      ROM extensions, header rules. Every package reads it, which also fixes STATUS bug 2
      (atari800 and apple2 can't be chosen).
- [ ] Every system with a DAT source: from No-Intro e.g. 32X, SG-1000, Virtual Boy,
      Pokémon Mini, Famicom Disk System, Satellaview, SuperGrafx, ColecoVision,
      Intellivision, Vectrex, Atari 5200 and 8-bit, Odyssey², MSX2, VIC-20, Plus/4; from
      Redump e.g. Neo Geo CD, PC-FX, CD-i, Jaguar CD, CD32, CDTV, Xbox, PC-98
- [ ] ES-DE folder names checked against `es_systems.xml` of a real install; Batocera's
      against a real install or marked as unchecked
- [ ] Systems with no DAT source (DOS, ScummVM, ports) can be added as unverified folders

**Exit:** every system ES-DE and RetroArch support that has a DAT source can be chosen,
synced and output.

### M10 — Arcade: FBNeo and MAME
Arcade sets don't fit the cartridge model. A game is a zip named by its short name
(`sf2.zip`) holding many ROM chips; a clone may need its parent's zip; many games need a
BIOS zip (`neogeo.zip`) or device files; each emulator version expects its own sets.
Emulators find games by file name, so outputs keep the short names.
- [ ] Arcade DATs from libretro-database: FBNeo (`metadat/fbneo-split`), MAME 2003-Plus
      and MAME (`metadat/mame`); a DAT imported by hand for any other MAME version
- [ ] Sets matched by short name and member checksums: complete, incomplete (which ROMs
      are missing), or for another version; split, merged and non-merged sets understood
- [ ] Parent, BIOS and device files linked beside the game; MAME CHDs in a folder per set
- [ ] One game per parent/clone family from the DAT, with filters for BIOS, bootlegs,
      mahjong/casino and mechanical games
- [ ] Outputs: original file names; labels from the DAT's descriptions; a RetroArch
      playlist with the FBNeo or MAME core; ES-DE's `arcade`, `fbneo`, `mame` and
      `neogeo` folders

**Exit:** the owner's arcade and Neo Geo sets are matched against FBNeo with
their BIOS, and a game launches in RetroArch's FBNeo core.

### M11 — Adding ROM folders
- [ ] **"Find folders" on the Library tab:** setup's look-inside-this-folder step
      (`discover.Folders`), as a component both screens share: pick a top folder, review
      the system proposed for each subfolder, add the ticked ones
- [ ] Remember those top folders and offer the new subfolders found at the next sync
- [ ] Better guesses: zip contents (member extensions), a sample of checksums against
      downloaded DATs ("31 of 40 files are in the GBA DAT"), every frontend's folder names
      (`megadrive`, `pcengine`, `wswan`) from the M9 registry, and the kind of set
      (No-Intro, Redump, TOSEC, GoodTools, MAME)
- [ ] Per folder: change its system, see its counts (verified, unverified), and a plain
      reason when two folders overlap
- [ ] Mixed folders (GB and GBC games together) sorted into their systems by extension or
      header (needs design)
- [ ] A whole set in one archive (a computer collection in one 7z, a zipped No-Intro set) read as a folder
      (later: a solid 7z is slow to read file by file)

**Exit:** a user points at one top folder and every system folder in it is added, from
the Library tab as well as from setup.

---

## 4. Backlog (unscheduled)
- PBP and WUA output
- Config-driven custom profile specs (JSON/YAML)
- CLI mode / scheduled re-sync
- Scraper hand-off
- BIOS detection/validation
- Download-on-demand for tool binaries
- Retool clone-list import (`rules.GroupingCloneList` exists as a type only)
- Game Boy / SNES / Master System dumps no checksum matches: their headers carry a title
  but the DATs list no serial (M7 covers GBA, NDS, N64)
- Show the title being identified during the Identify step of a sync
- chdman / dolphin-tool builds for Linux and macOS (only if upstream or the owner supplies them)

---

## 5. Cross-cutting requirements
- **Never modify or delete anything under a source root.** Hardlink tier is opt-in
  specifically because it can violate this (§2.2).
- **Idempotency:** unchanged inputs → zero filesystem writes.
- **Transparency:** every skip, fallback, grouping method, and copy is visible.
- **Resumability:** scans and transcode batches survive app restarts.
- **Cross-platform parity:** behavior stated and tested on all 3 OSes before a checklist
  item closes.

---

## 6. Decisions (locked)
- Frontend: Svelte
- Shell: Wails v3, pinned (rev. 2)
- Canonical library: DB-only, profiles link directly to sources (rev. 2)
- Link tiers: symlink → reflink → hardlink (opt-in) → copy; no junctions (rev. 2)
- Network destinations blocked; FAT32/exFAT only via export-to-device (rev. 2)
- Bundled tools: the upstream projects' own unmodified releases, pinned by SHA-256,
  invoked as subprocesses; never compiled by us; PATH fallback on Linux/macOS
  (rev. 3, owner, 2026-10-24; replaces "built from pinned source in CI")
- Product name RomCure; repository `AlanDrake/romcure`; app license GPL-3.0 (rev. 3)
- Games are chosen per output, versions library-wide (ADR 0001, rev. 3)
- Several source folders per system (rev. 3)
- RAR support through the bundled 7-Zip, read-only (rev. 3)
- `link_state`: §2.7

---

## 7. Third-party license compliance — findings

*Not legal advice; have the final release reviewed by someone qualified, especially
the GPL source-offer mechanics.*

| Tool | License (upstream repo) | Obligations when bundling the binary |
|---|---|---|
| `chdman` (MAME) | MAME as a whole: **GPL-2.0-or-later**; >90% of files individually BSD-3-Clause | Treat the binary as GPL-2.0+: ship license text, attribution, and complete corresponding source for the exact version built |
| `dolphin-tool` (Dolphin) | **GPL-2.0-or-later** | Same as above |
| `7-Zip` (RAR reading) | **LGPL-2.1+**, with the **unRAR restriction** on the RAR code (may not be used to build a RAR-compatible archiver), BSD-2/3 for small parts | Ship 7-Zip's `License.txt` with the binary (required for binary redistribution); say that RomCure only reads RAR; source is published at github.com/ip7z/7zip |
| libretro-database serial lists (data, downloaded on request, not shipped) | **CC BY-SA 4.0** | Attribution in the notices; unmodified; share-alike only matters if we redistribute or adapt the lists, which we don't |
| libretro-database copies of the No-Intro DATs (data, downloaded on request from GitHub, not shipped) | Repo **CC BY-SA 4.0**; the DATs are No-Intro's work | Attribution to libretro-database and No-Intro in the notices; unmodified; we fetch from libretro, never from No-Intro's site. **Open:** check No-Intro's own terms on DAT use before a public release |
| `maxcso` | **ISC**; bundles LGPL (7-Zip/p7zip code), Apache-2.0 (Zopfli), MIT (libdeflate, libuv), BSD (lz4), zlib | Include ISC notice + each bundled library's license text; LGPL portion: provide source for that component too |
| `bodgit/sevenzip` (Go lib, compiled into the app) | Verify in repo before M1 | If permissive (expected), notice in licenses screen |
| Retool clone lists (if bundled) | Verify before M2 | Decide bundle vs. user-supplied based on license |

**Practical obligations derived from the above:**
- [x] **Source availability (rev. 3 approach):** the shipped binaries are the projects'
      own unmodified releases, so every archive carries `SOURCES.txt` naming the upstream
      release of each tool, and the Help tab and the notices say the same and offer to provide
      the source on request (a written offer). Rev. 2 advised attaching the exact source
      tarballs to each release instead, because upstream can move or delete tags: that
      remains the safer option if a lawyer prefers it (MAME publishes `mame0289s.exe`;
      Dolphin's tag needs its submodules). **Needs the legal review noted above.**
- [ ] **Your app's own license is not forced to GPL** by invoking these tools as
      separate executables via `os/exec` — this is generally treated as mere
      aggregation. Keep it that way: never link MAME/Dolphin code into the Go binary
      (e.g. via cgo or libchdr-from-MAME), or the analysis changes. Note: libchdr
      (standalone CHD reader) is BSD-3 if CHD *reading* in-process is ever needed.
- [x] **Attribution:** in-app Third-Party Licenses screen listing every bundled
      component, version, license, full license text, and source link; also a
      `THIRD_PARTY_NOTICES` file in each installer.
- [ ] **Trademarks:** refer to "MAME", "Dolphin", "RetroArch", "PPSSPP" nominatively
      (to say what the app works with); no logos, no implication of endorsement.
- [x] **Per-release check:** notices are generated from the pins (`go run ./tools/gen-notices`);
      a Go test fails if a Go module in `go.mod` has no notice or a stale version.
      The tool pins feeding the notices are not yet checked by a test.
- [ ] Repeat this table's verification for every new bundled component (M5 tools).
