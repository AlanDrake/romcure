# Project status

Current state, open gaps and backlog. Update at the end of every working
session. `HANDOFF.md` holds the stable decisions and working rules; the spec
(`rom-library-manager-spec.md`) holds requirements and per-milestone
checklists with what is verified. Past work is in the git history.

**Last updated:** 2026-10-25

## Where things are

Milestones M0–M5 are done and M6 is done except signing (see the spec
checklists for the verification level of each item). CI (vet, tests, Wails
build, smoke test on Windows/macOS/Linux, Playwright e2e on Linux) is green;
push only when needed (CLAUDE.md).

- **App:** Go core (`internal/*`), orchestration in `internal/app`, a thin
  Wails service in `cmd/romcure/service.go`, Svelte UI with Library, Games,
  Outputs, Settings and Help tabs, first-run setup, and a job progress strip
  with Cancel. 28 Playwright tests drive the server-mode build (the CHD-check
  one skips without chdman, so it never runs in CI's Linux job).
- **Identification:** checksums against Redump (discs), TOSEC (computers) and
  No-Intro (cartridges, libretro-database's copies), all downloaded on request.
  A DAT imported by hand replaces the downloaded No-Intro DAT of its system.
  Headered cartridge dumps match by their headerless CRC. GameCube/Wii discs
  that no DAT or file name identifies are named from their disc header, and
  PS1/PS2 discs from their serial (libretro serial lists). GBA, NDS and N64
  dumps that no checksum matches are named from the game code in their
  cartridge header, through the serial the No-Intro DAT lists, except files
  marked as hacks or translations. Inside an archive, members without one of the
  system's ROM extensions are not games. A C64 archive of several program files
  (a P00 or PRG set) is one game, linked as the archive. TOSEC disk sides and
  tapes are parts of one game with one `.m3u`. Damaged archives are skipped and
  listed, and the sync continues.
- **Games screen:** one row per game with its versions folded in, ticks per
  output, version choice, filters and search, and a resizable systems rail.
  Changes reach an output only on Apply. The backend is measured at 100k games
  (opt in with `ROMCURE_SCALE=1`).
- **Outputs:** RetroArch, ES-family, raw and SD-card export. Links where
  possible, otherwise copies. Edit, Check files and Repair. CHD/RVZ conversion
  with a content-keyed cache.
- **Live results on the owner's sample collection:** the No-Intro download verified
  NES 827/843, SNES 748/776, Mega Drive 708/709, SMS 284/370, Atari 2600
  482/482, N64 43/46 and PC Engine 132/664. PC Engine is low because that
  folder is a GoodTools-era set with "Sounds" files, PD demos and SuperGrafx
  titles; nobody has looked into it. The user confirmed that a PS1 CHD and a PSP
  CHD boot in RetroArch, and that ES-DE loads a generated tree. The UAC
  relaunch was confirmed on Windows.
- **M7 fixes, checked against copies of the user's data,** then confirmed
  working by the owner in their own app after a rebuild, Update DATs and Sync
  (2026-10-25):
  - GBA: went from 101 games (62 verified, 5 heuristic, 34 unidentified) to 97
    (62 verified, 33 named, 2 unidentified). The four stray archive members are
    gone, and 32 dumps are now named from their header.
  - C64: two multi-file programs went from 65 games and 51 games to 1 each.
    All 8 sides of a 4-disk game are selected and go in one 8-line `.m3u`.
  - The three N64 RARs that were recorded as plain files read as RARs again.
  - Migrations 0015 and 0016 take about 1 s on the user's 300 MB database.

## Known bugs and gaps

1. **Device export estimate** counts discs that will be converted at their
   uncompressed size, so it over-estimates. It has never been tested on a real
   FAT32/exFAT card.
2. **System picker** (`profile.KnownSystems`) lists only systems with a libretro
   DB name, so `atari800` and `apple2` (supported by TOSEC sync) can't be chosen.
3. **Batocera/RetroBat/RetroDECK folder names** come from documentation only.
   The ES-DE names were checked against a real install.
4. **Output name collisions:** two different multi-disc games with the same
   title still collide. When a namesake leaves, the coded file
   ("AC_DC [3fa91c]") is not removed, because pruning goes by game key, not by
   path.
5. **Cue sheets in archives:** a `.cue` whose text differs from Redump's is
   identified by name even when its tracks are verified. Loose sheets are
   promoted from their tracks; archived ones are not. Loose non-game files are
   skipped by a deny-list, which can miss odd types. Archive members are
   filtered by the system's ROM extensions, but only for the systems in
   `match/romext.go`.
6. **Scene-named 2-disc GameCube games** are not folded into one .m3u entry:
   disc 1's header carries no disc tag.
7. **Damaged files** are retried and reported again on every sync.
8. **CHD `game_key` changes** after "Check CHD files" do not carry over ticks
   or overrides. A system with no games still gets an empty RetroArch `.lpl`.
9. **Caches of old installs** stay in the roaming data folder. There is no
   migration; the user can pick a Cache folder in Settings.
10. **A slow migration shows no window** while it runs, because the window
    appears only after the library opens.
11. **libretro's No-Intro game names** drop some tags that stay in the ROM name
    ("(Aftermarket) (Unl)"). No-Intro's terms on DAT use need checking before a
    public release (spec §7).
12. **Wails template task `build:server` is broken** (it builds `.`). Use the
    command in CLAUDE.md.
13. **Multi-disc games with no complete release** still mix: when no release
    in the collection has every disc, the missing disc comes from another
    release (23 of 432 C64 games in the owner's set). For Redump names that
    means a different region, and the output then splits the game into two
    `.m3u` sets. Disc counts are only known from "of N" tags, so a 2-disc USA
    release loses to a 3-disc European one when Redump names don't say.
14. **Header names and hacks:** a hack keeps its base game's code. Files marked
    [h], [T+…], [tr …], (Hack) or (Translation) keep their own names. Unmarked
    hacks (a translation whose file name has no tag) are named and grouped as
    the base game and lose to a clean dump unless chosen on the Games screen.
15. **Serials come with the next DAT import:** "Update DATs" re-downloads the
    No-Intro DATs once for this. A DAT imported by hand has no serials until
    it is imported again.
16. **A C64 multi-file game is linked as its zip as is:** if that zip uses an
    old method (implode), RetroArch can't read it either, and it is not
    repacked.

## Never verified

- A real launch on macOS/Linux (CI only), including the 7zz path and the
  close-while-a-job-runs dialog.
- Declining the real UAC prompt (the in-app "Keep using copies" path is
  tested).
- An RVZ booting in Dolphin. The user never confirmed it.
- Parallel conversions with real chdman or dolphin-tool, and scratch space for
  N parallel jobs (each job checks only its own need). Killing a real
  dolphin-tool on Cancel (child processes).
- A real RetroArch playlist showing and launching games (RetroArch only started
  cleanly against a generated config).
- Removable-drive and RetroArch/ES-DE detection in setup on a machine that has
  them. The native Browse dialogs.
- Scrolling a 100k-game list in the real window, Shift-click range ticking, and
  the SD-card size meter and "doesn't fit" block (there is no e2e test for a
  device output).
- Encrypted RARs (reported as unreadable).
- NDS and N64 header naming on real dumps (synthetic tests only; the user's
  folders had none that needed it).
- A C64 multi-file zip (P00 set), or the `.m3u` of TOSEC disk sides, launched
  in RetroArch's VICE core or ES-DE.
- The recording of old zip methods on every zip in the library. It is filled
  in on the next scan and was checked on the N64 and GBA folders only. 7-Zip
  extracting the old-method C64 zips was checked by forcing two of them into a
  scratch output.
- The FAQ read by a user.

## Release tooling (owner decision: no compiling of third-party code)

- The tools are the upstream projects' own unmodified releases, pinned with
  SHA-256 in `tools/versions.txt` and fetched by `tools/fetch-tools.sh`:
  - chdman and DolphinTool for Windows x64/arm64.
  - 7-Zip for every platform.
  - On Linux/macOS, chdman and dolphin-tool come from the PATH; the projects
    publish no builds for them.

  `findTool` looks in `<exe>/tools`, then `tools/bin/<platform>`, then the PATH.
- The GPL source offer (Help, the notices, `SOURCES.txt`) names the upstream
  tag of each tool. Regenerate the notices with `go run ./tools/gen-notices`
  after a dependency change. A test enforces this for Go; npm is not checked.
- `tools/package.sh` and `.github/workflows/release.yml` build the Windows x64,
  Linux x64 and macOS arm64 archives. A manual run succeeded. Not verified:
  - a packaged build on a clean machine
  - the tag-triggered release step
  - Windows arm64 and macOS x64 packages

  The builds are unsigned, so SmartScreen and Gatekeeper will warn.

## Backlog (suggested order)

The owner's review of 2026-10-25 added milestones M7–M11 to the spec.

1. M7's follow-ups. The fixes themselves are done and are described above.
   Done: a multi-disc game is chosen as a whole release (see bug 13 for what
   remains). Still open: Mega Drive serials, and the PC Engine set.
2. M11's first item: "Find folders" on the Library tab, reusing setup's discovery.
3. M8: RetroArch playlists RetroArch reads. The owner chose option A (merge
   into RetroArch's own playlists). Done: the RetroArch in use is found
   (running process, usual places) and chosen per output, with a picker in
   setup, in the Add output form and on each RetroArch output's card. The
   playlist writer is in: Apply merges into the playlist folder of the chosen
   RetroArch and refuses while RetroArch is open. The owner ran it on their real
   RetroArch: systems appeared, games were added and removed, their own playlists
   kept working. A default core per system (ranked: opens the files, BIOS
   present, the user's own choice, then ES-DE's recommended order; option in
   Settings) is in and unlaunched. Open: dropping a system's entries when only that system is
   removed.
4. M9: one systems registry, then every system with a DAT source.
5. M10: arcade (FBNeo, MAME). Builds on M9.
6. A first release: run a packaged build on a clean machine, then push a v* tag. Then
   signing (needs the owner's certificates; spec M6), and No-Intro's terms and §7
   reviewed before the release is public. The owner may want this earlier.
