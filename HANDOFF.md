# Project Handoff — RomCure (ROM Library Manager)

**Live status, known bugs, backlog:** `docs/STATUS.md` (updated every session).
**Working notes for Claude Code (commands, test data, environment):** `CLAUDE.md`.
**Companion document:** `rom-library-manager-spec.md` (rev. 3) — the authoritative spec.
If this file and the spec disagree, the spec wins; flag the conflict to the owner.

---

## 1. What you're building (one paragraph)

A cross-platform desktop app (Windows/macOS/Linux) that takes a huge, read-only ROM
collection (No-Intro, Redump, TOSEC sets), identifies the games in it, picks one best
version per game (1G1R) according to user rules, and exposes the result to emulators
(RetroArch first) through symlinks or other zero-copy links. Disc images that need
it are converted to CHD/RVZ into a managed cache. It never modifies the source
collection. SD-card export for handhelds is a copy-based mode of the same pipeline.

---

## 2. Read this first

1. This file, top to bottom (≈10 min)
2. Spec §2 (architecture) and §2.2 (link tiers) — most bugs will come from these
3. Spec §3 milestone you're assigned to
4. Spec §5 (cross-cutting rules) — these are acceptance criteria for every task

---

## 3. Decisions already made — do not relitigate

Each decision lists what was rejected and why, so you don't re-propose it. If you have
new evidence that a decision is wrong, raise it with the owner; don't silently deviate.

| Decision | Rejected alternatives | Why |
|---|---|---|
| **Go + Wails v3 (pinned exact version)** | Tauri/Rust, .NET/Avalonia, Electron, Python/Qt, Wails v2 | Go stdlib covers symlinks, hashing, XML, zip; goroutines suit parallel hashing. v3 API is declared stable though still tagged beta; v2 is maintenance-only. |
| **Svelte + TypeScript** frontend | React | Owner preference. |
| **SQLite via `modernc.org/sqlite`** | CGO sqlite drivers | No CGO in the data layer simplifies builds. |
| **Canonical library = database records only** | A materialized `by-system/` link tree | A tree doubles links and creates link chains that hardlinks can't represent. Profiles link straight to the ultimate source. |
| **Link tiers: symlink → reflink → hardlink (opt-in) → copy** | Junctions; hardlink as default | Every link is a file link (junctions are directory-only). Hardlinks *are* the source file, so in-place writes would modify the source. |
| **Windows: launch unelevated; offer runtime UAC relaunch when a symlink is needed** | Manifest `requireAdministrator`; requiring Developer Mode | Manifest elevation makes "declined" kill the app with no fallback. Owner judged Dev Mode too much to ask of average users. |
| **Network-share destinations are blocked** | Allowing links/copies to NAS | Links are unreliable on SMB/NFS; owner decision. Network *sources* are fine. |
| **FAT32/exFAT only via explicit "Export to device" (copy)** | Treating it as a link failure | SD-card prep for handhelds is a primary use case; make it a feature. |
| **DAT files optional** | DAT-mandatory | Homebrew, hacks, uncovered systems; better first run. Three confidence tiers: DAT-verified / heuristic / unidentified. |
| **Tools bundled as the upstream projects' own unmodified releases, pinned by SHA-256; never compiled by us; PATH fallback on Linux/macOS** (owner, 2026-10-24) | Building MAME/Dolphin from source in CI (tried twice for MAME, failed on every OS); download-on-first-use | Compiling three OSes of MAME/Dolphin is a project of its own. MAME and Dolphin publish Windows builds only (7-Zip publishes all); elsewhere users install `mame-tools`/`rom-tools` and `dolphin-emu`. The GPL source offer then points at the upstream tag (`SOURCES.txt`, Help tab), pending legal review. |
| **Games are chosen per output; versions stay library-wide** | One include list for every output | An SD card cannot hold the PC library. `docs/adr/0001-per-output-game-selection.md`. |
| **Several source folders per system, read as one library** | One folder per system | Real collections are spread over folders; a game found twice is used from the folder added first. |
| **RAR is read through the bundled 7-Zip (subprocess), read-only** | A Go RAR library; shipping unRAR | No maintained Go reader handles RAR5/solid/multi-part; 7-Zip's license only forbids building a RAR *writer*. |
| **Bundled tools invoked only as subprocesses (`os/exec`)** | Linking MAME/Dolphin code via cgo | Keeps the app's own license independent of GPL (mere aggregation). |
| **`.m3u` with relative paths + hidden `.discs/` sibling folder** | Absolute paths; discs beside the m3u | Portability; avoids frontends listing each disc as a game. |
| **Symlinks relative on same volume, absolute across volumes** | Always absolute / always relative | Relative survives moving a drive; cross-volume relative is impossible on Windows. |
| **`game_key` = content-hash based** | Title/filename based | No-Intro/Redump rename entries; overrides must survive DAT updates. |

---

## 4. Current state

- Repository: https://github.com/AlanDrake/romcure (default branch `master`; CI runs on
  pushes to `master` and on pull requests; all three OS jobs were green on 2026-10-24).
- Code: M0-M5 done and the Games screen, setup, RAR, release packaging on top; see the
  spec's checklists and `docs/STATUS.md` (live: what is verified, known gaps, backlog).
- Product name: **RomCure**; binary `romcure.exe`; Go module `romcure`.
- License for the app itself: **GPL-3.0** (the LICENSE file on the GitHub repo). Bundled
  GPL tools are still separate executables run as subprocesses; if releases ship them,
  the source/notice obligations in the spec's §7 still apply to those binaries.

---

## 5. Suggested repository layout

```
/cmd/romcure/            Wails v3 app entry point (thin)
/internal/scan/            directory walk, hash cache, zip/7z/RAR header CRC reading
/internal/rararc/          RAR through the bundled 7-Zip (list, extract); rartest builds test RARs
/internal/dat/             Logiqx XML + ClrMamePro text parsers, P/C detection
/internal/match/           hash → DAT matching, confidence tiers
/internal/rules/           grouping (P/C, clone lists, title normalization), 1G1R
/internal/library/         canonical game records, overrides, game_key
/internal/plan/            per-profile desired state, diff vs link_state
/internal/link/            link primitives + capability detection, per-OS files
/internal/fsinfo/          filesystem type, volume identity, free space (per-OS)
/internal/elevate/         Windows UAC relaunch (no-op on other OSes)
/internal/transcode/       chdman/dolphin-tool wrappers, Convert pipeline, cache
/internal/profile/         output generators: options, layout, linkgame, retroarch, esfamily, raw, device
/internal/store/           SQLite schema, migrations, queries
/internal/app/             orchestration: sources, outputs, sync, Games screen logic, progress reporter
/internal/catalog/         in-memory game index behind the Games screen
/internal/settings/ discover/ support/ notices/ applog/ proc/   settings, first-run discovery, diagnostics bundle, license data, log, quiet subprocesses
/frontend/                 Svelte + TS
/tools/                    versions.txt (pins), fetch-tools.sh, fetch-7zip.sh, package.sh, gen-notices, licenses/
/.github/workflows/        ci.yml (every push), release.yml (portable archives; v* tag = release)
/testdata/                 tiny synthetic ROMs, DATs, archives (no real ROMs!)
/docs/                     spec, this file, ADRs
```

Rule: nothing under `/internal` imports Wails. The UI layer calls into these packages
through one thin service layer in `/cmd/romcure`.

Per-OS code uses build-tagged files (`link_windows.go`, `link_darwin.go`,
`link_linux.go`) — no runtime `GOOS` branching for syscalls.

---

## 6. First tasks (M0 breakdown, in order)

All 7 implemented; see spec.md's M0 checklist for exactly what's verified vs. not
(UAC decline path needs a human click, Windows reflink untested against real
ReFS/Dev Drive hardware).

1. ✅ Scaffold repo, Go module, Wails v3 (pinned) + Svelte template; window opens on all
   3 OSes in CI.
2. ✅ `/internal/store`: migration runner + schema v1 (tables listed in spec M0).
3. ✅ `/internal/fsinfo`: filesystem type (NTFS/APFS/ext4/Btrfs/XFS/ReFS/FAT32/exFAT/
   remote), volume identity (same-volume check), free space. Table-driven tests.
4. ✅ `/internal/link`: `Symlink(rel|abs)`, `Reflink`, `Hardlink`, `Copy`; plus
   `Capabilities(src, dst)` returning which tiers are available and why not.
5. ✅ `/internal/elevate` (Windows): symlink probe, Dev Mode registry check, ShellExecute
   `runas` relaunch, detection of declined UAC (relaunch returns error) → caller falls
   back. Test manually on a standard non-admin account; document the result.
6. ✅ Tier selection function: given capabilities + user prefs (hardlink opt-in), return
   tier + human-readable reason. This reason string is shown in sync reports later.
7. ✅ Structured logging to a file in the app data dir.

**M0 is done when** spec M0 exit criteria pass on all three OSes.

---

## 7. Working rules for agents

- **Never write to a source root.** Add a guard in `/internal/link` and
  `/internal/transcode` that refuses any write path inside a registered source root,
  and test it. Treat a violation as a release-blocking bug.
- **No real ROMs in the repo or CI.** Use synthetic files with known CRCs and
  hand-written mini DATs in `/testdata`. Do not download ROMs or DATs in tests.
- **Every user-visible fallback carries a reason** (why copy instead of link, why a
  game was excluded, which grouping method was used). No silent degradations.
- **Idempotency test is mandatory** for anything touching outputs: run twice, assert
  zero filesystem writes on the second run.
- **Cross-OS parity:** a checklist item isn't done until behavior on Windows, macOS
  and Linux is stated and tested (CI or a written manual test note).
- **Pin everything:** Wails version, tool versions, codec flags. Version bumps are
  separate, deliberate changes.
- **Don't copy igir's code** (GPL-3.0). Reading its docs for behavior reference is fine.
- **Record decisions:** any non-trivial choice not covered by the spec gets a short ADR
  in `/docs/adr/NNNN-title.md` (context, decision, alternatives).
- Commits: small, one concern each; message says what and why.

---

## 8. Known risks / things likely to bite

| Risk | Where | Mitigation |
|---|---|---|
| Windows elevation edge cases (no admin account, UAC declined, elevated process can't see mapped drives) | M0 | Test on a standard account; mapped drive letters are per-session — elevated process may not see the user's mapped drives, another reason to block network destinations |
| WebKitGTK version differences on Linux | M0 | Target 4.1; test Ubuntu LTS, Fedora, SteamOS |
| Wails v3 is still pre-release | all | Pin version; keep `/internal` Wails-free |
| Scan speed on huge sets | M1 | Read CRCs from archive headers; don't decompress unless needed |
| DATs without parent/clone data (Redump, TOSEC, plain No-Intro) | M2 | Clone lists / title normalization fallback, labeled as such |
| DAT renames orphaning overrides | M2 | content-hash `game_key`; test with a renamed DAT |
| BIN/CUE linked without all tracks | M3 | Link all member files with original names |
| CHD codec incompatibility with older cores | M4 | Explicit codec flags per profile |
| PS2/PSP CHD made with `createcd` | M4 | Choose `createdvd` by detected media |
| Unsigned bundled tool binaries blocked on macOS | M6 | Sign + notarize every helper binary |
| GPL source-offer obligations | M4+ | Official unmodified binaries + `SOURCES.txt` + written offer (Help tab); legal review pending (spec §7) |
| FAT32 4 GiB limit | M5 | Planner flags oversized outputs |

---

## 9. Open questions for the owner

- [x] Product name: RomCure
- [x] Repository: github.com/AlanDrake/romcure. License: GPL-3.0
- [ ] Legal review of the GPL/LGPL source-offer wording (spec §7)
- [ ] Retool clone lists: bundle (license permitting) or require user download?
- [ ] Hardlink tier: expose at all in v1, or hide behind an advanced setting?
- [ ] Code-signing accounts (Windows certificate, Apple Developer ID) — who owns them
      and when they'll be available (needed by M6, lead time can be weeks)
- [ ] Minimum OS versions (proposal: Windows 10 22H2+, macOS 13+, Ubuntu 22.04+)

---

## 10. Glossary

- **DAT** — XML/text catalog of known-good dumps with hashes (No-Intro, Redump, TOSEC).
- **P/C (parent/clone)** — DAT relationship grouping regional/revision variants of one game.
- **1G1R** — "one game, one ROM": keep a single preferred variant per game.
- **Clone list** — community mapping (e.g. Retool) supplying P/C groupings missing from DATs.
- **Profile** — one generated output layout for one consumer (RetroArch, ES-family, raw, device export).
- **Ultimate source** — the real file a profile links to: raw file, archive, or transcode-cache entry.
- **Reflink** — copy-on-write clone (APFS, Btrfs, XFS, ReFS): instant, independent copy.
- **CHD / RVZ / CSO** — compressed disc formats (MAME/Dolphin/PSP ecosystems).
- **m3u** — playlist grouping a multi-disc game's discs for disc swapping.
