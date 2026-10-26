# RomCure

**Turn a big, messy ROM collection into clean, emulator-ready libraries — without
copying your games or touching the originals.**

RomCure reads your game folders, works out which game each file is, keeps one good
version of every game (for example the USA one), and makes them appear in RetroArch,
ES-DE and other frontends through links, so a curated library takes almost no extra
disk space. Disc images are converted to smaller formats (CHD, RVZ) on the way. Your
source folders are only ever read, never changed.

> **Status:** early, unsigned builds. Windows is the most tested platform; macOS and
> Linux build and pass the test suite in CI but have seen little real use.

## What it does

- **Identifies your games** by checksum against Redump (discs), TOSEC (home
  computers) and No-Intro (cartridges, from libretro-database's copy) databases,
  downloaded when you ask, or DATs you import, and by file name otherwise
  (No-Intro-style names such as `Super Metroid (Japan, USA) (En,Ja).sfc` work well).
  Headered cartridge dumps (NES, Famicom Disk System, Atari 7800, Lynx, SNES copier
  headers) and scene-named discs (named from the GameCube/Wii disc header or the
  PlayStation serial) are handled.
- **One game, one version (1G1R):** keeps one version of each game according to your
  region preference (USA, Europe, Japan or World first), and tells you why. Pick
  another version of any game in two clicks.
- **Choose which games go where:** each output (RetroArch on the PC, an SD card for a
  handheld...) has its own selection. Search, filter, tick systems or single games,
  undo, then press Apply.
- **Links, not copies:** outputs are symbolic links (or reflinks, or hard links if you
  allow them) to your files. Where links are impossible (an SD card formatted FAT32 or
  exFAT), it copies, and checks first that everything fits.
- **Outputs for:** RetroArch (with playlists), ES-DE, RetroDECK, Batocera, RetroBat,
  plain folders, and SD card / USB export. Multi-disc games get one `.m3u` entry.
- **Disc conversion:** PlayStation, PS2, PSP, Saturn, Sega CD, Dreamcast, PC Engine CD
  and 3DO discs become CHD; GameCube and Wii become RVZ. Conversion is verified before
  use and cached, so it happens once.
- **Archives:** reads zip, 7z and RAR (including RAR5 and multi-part sets) without
  unpacking them to scan; extracts only what an output needs. A damaged archive is
  reported and skipped, never stops a sync.
- **Several folders per system**, a first-run setup that finds your game folders and
  installed frontends, cancel and progress for every long job, a diagnostics export
  that leaves out your library and user name.

## Download and install

Get the archive for your system from the
[Releases page](https://github.com/AlanDrake/romcure/releases), unpack it anywhere and
run `romcure` (`romcure.exe` on Windows). Nothing is installed; keep the `tools`
folder next to the program.

| | Included programs | You may need |
|---|---|---|
| **Windows** (x64) | chdman, DolphinTool, 7-Zip | Nothing. Windows asks once whether RomCure may restart as administrator to make links (or turn on Developer Mode); saying no is fine: outputs are then copied. |
| **Linux** (x64) | 7-Zip | `chdman` (package `mame-tools`) for CHD, `dolphin-tool` (package `dolphin-emu`) for RVZ; RomCure finds them on the PATH. |
| **macOS** (Apple Silicon) | 7-Zip | `chdman` (Homebrew `rom-tools`); dolphin-tool is not published for macOS. |

The builds are not signed yet, so Windows SmartScreen and macOS Gatekeeper warn the
first time you open them.

## Quick start

1. Run RomCure. The setup asks for the folder with your games; if it holds one folder
   per system, they are all found at once (check the system it guessed for each).
2. Choose your region preference.
3. Add an output: a found RetroArch or ES-DE folder, an SD card, or a folder of your
   choice. Choose whether it starts with all games or none.
4. Optionally press **Download databases** (Redump, TOSEC and No-Intro, plus PlayStation
   serial lists) for exact identification.
5. Press **Start first sync**. Then use the **Games** tab to choose what each output
   gets and press **Apply**.

Questions and problems: see the [FAQ](docs/FAQ.md).

## Building from source

You need Go 1.27, Node.js 22 and the Wails v3 CLI (`v3.0.0-beta.30`); on Linux also
GTK 3 and WebKitGTK 4.1 development packages.

```sh
go install github.com/wailsapp/wails/v3/cmd/wails3@v3.0.0-beta.30
wails3 task build                 # -> bin/romcure (add EXTRA_TAGS=gtk3 on Linux)
bash tools/fetch-tools.sh         # official chdman / DolphinTool / 7-Zip, SHA-256 checked
go test ./internal/...            # core tests (no window needed)
cd frontend && npx playwright test   # end-to-end tests against the app in server mode
```

`tools/package.sh <platform> <version>` builds a release archive;
`.github/workflows/release.yml` does it for all platforms. The design and its decisions
are in [`rom-library-manager-spec.md`](rom-library-manager-spec.md) and
[`HANDOFF.md`](HANDOFF.md); the live status is [`docs/STATUS.md`](docs/STATUS.md).

## License

RomCure is free software under the [GNU GPL v3](LICENSE). It runs chdman (MAME),
DolphinTool (Dolphin) and 7-Zip as separate programs, unmodified official releases
under their own licenses; see [`THIRD_PARTY_NOTICES.txt`](THIRD_PARTY_NOTICES.txt) or
Help → Third-party licenses in the app. RomCure does not include or download games.
Each GitHub release also carries the complete source of those three programs at the
exact versions it ships (`tools/fetch-sources.sh`).

Game databases (No-Intro, Redump, TOSEC, libretro-database) are not included: RomCure
downloads them when you ask, from libretro-database and Redump, or reads DATs you
import. They remain their authors' work.

## Trademarks

RetroArch, ES-DE, EmulationStation, Batocera, RetroBat, RetroDECK, MAME, Dolphin,
PPSSPP, No-Intro, Redump, TOSEC, PlayStation, Nintendo, Sega and the other names of
programs, projects and systems mentioned here and in the app belong to their owners.
They are used only to say what RomCure works with. RomCure is not affiliated with or
endorsed by any of them.
