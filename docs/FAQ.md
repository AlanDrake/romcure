# RomCure FAQ

Short answers to common questions. If something here doesn't match what you see, or
your problem isn't covered, open an issue on
[GitHub](https://github.com/AlanDrake/romcure/issues) and attach the diagnostics file
from **Help → Save diagnostics…** (it contains the recent log and settings, with your
user name removed, and nothing about your games).

- [Basics](#basics)
- [Identifying games](#identifying-games)
- [Choosing games and versions](#choosing-games-and-versions)
- [Outputs, links and copies](#outputs-links-and-copies)
- [Frontends](#frontends)
- [Disc conversion](#disc-conversion)
- [SD cards and USB drives](#sd-cards-and-usb-drives)
- [Archives and damaged files](#archives-and-damaged-files)
- [Files, folders and troubleshooting](#files-folders-and-troubleshooting)

## Basics

**Will RomCure change or delete my games?**
No. Your game folders are only read. RomCure refuses to write inside them, and the
folders it creates (outputs) only contain links, copies and files it made itself.
Removing an output can delete those, never anything else.

**Does it download games?**
No. It only downloads game *databases* (lists of names and checksums) when you press
**Update DATs** on the Library tab (**Download databases** in the setup), and never
otherwise goes online.

**What is the difference between Sync and Apply?**
**Sync** (Library tab) reads your game folders, identifies the games, picks a version
of each and then updates every output. Run it after adding or removing games.
**Apply** (Games and Outputs tabs) only updates outputs from choices you already made
(which games, which versions); it doesn't read your folders, so it's quick.

**Can I keep using the app while it works?**
Yes. Long jobs run in the background with their progress at the top, and **Cancel**
stops them; whatever was finished is kept and the next run picks up from there. If you
close the window during a job, RomCure asks first.

**My games are spread over several folders.**
Add each folder on the Library tab and say which system it holds; several folders of
one system become one library. A game found in two folders is used from the folder you
added first.

## Identifying games

**What do "verified", "guessed" and "unidentified" mean?**
- *Verified*: the file's checksum is in a game database, so the name and version are
  certain.
- *Guessed* (heuristic): no database match, but the file name follows the usual
  convention (`Game Name (USA) (Rev 1)`), or the disc says what it is.
- *Unidentified*: neither. The game still works; it just keeps its file name and
  RomCure can't tell its versions apart.

**How do I get more games verified?**
Press **Update DATs** on the Library tab. Disc systems use Redump (PlayStation,
PS2, PSP, GameCube, Wii, Saturn, Dreamcast, Sega CD, PC Engine CD, 3DO), home computers
use TOSEC (Amiga, C64, ZX Spectrum, Atari ST, Amstrad CPC, MSX, Atari 8-bit, Apple II),
cartridge systems use No-Intro (NES, SNES, N64, Game Boy, GBA, DS, Mega Drive, Master
System, Game Gear, PC Engine, Atari 2600/7800/Lynx/Jaguar, Neo Geo Pocket, WonderSwan).
Then Sync. Disc games stored as `.chd` need **Check CHD files** first (it reads each
disc once, about half a minute per disc).

**Where do the cartridge databases come from? Can I use my own?**
No-Intro's site has no download an app can use, so **Update DATs** takes the copies
the [libretro-database](https://github.com/libretro/libretro-database) project keeps
(the same ones RetroArch uses). They can trail No-Intro by a few weeks and have no
parent/clone data. For the newest data, or a parent/clone DAT (which groups a game's
versions most accurately), download the DAT from
[datomatic.no-intro.org](https://datomatic.no-intro.org) (the zip is fine) and import it
on the Library tab under **Import a DAT file**; RomCure works out the system from the
DAT's name, or you choose it. It replaces the downloaded one, and **Update DATs** then
leaves that system alone. Then Sync. Headered NES/SNES/Lynx/Atari 7800 dumps match
either kind of DAT. Without a DAT these systems are identified by file name, which
works well for No-Intro-named sets.

**A disc shows up as a strange name like `scn-gzlt`.**
That is a scene release name. If no database and no file name tags identify the disc,
RomCure names it from the disc itself: GameCube and Wii discs carry their title,
PlayStation and PS2 discs a serial number that is looked up in a list fetched with
**Update DATs**.

**Some files are listed as unreadable.**
They are damaged or incomplete archives (or files RomCure can't open). They are skipped
and everything else syncs. The Library tab lists them with the reason; replace or
repair them and Sync again.

## Choosing games and versions

**Which version does RomCure pick?**
The one that best fits your region preference (Settings → Region preference): for
"USA first", a game released in USA, Europe and Japan gives you the USA version.
Verified dumps beat bad ones, final releases beat betas and demos, and newer revisions
win. The game's details panel (Games tab, click a game) shows every version and why
one was picked.

**How do I choose another version?**
Open the game's details on the Games tab and pick the version you want. That choice
applies to every output and survives database updates. "Back to automatic" undoes it.

**How do I choose which games an output gets?**
On the Games tab, pick the output at the top, then tick or untick games, whole systems
(left), or everything you filtered (search and filters at the top). Changes are saved
at once; the output's folder changes when you press **Apply**. **Undo** reverts the
last change. New games follow their system's tick, or the output's default.

**Can different outputs have different games?**
Yes, that is the point: everything on the PC, a hand-picked set on the handheld's SD
card. Only the *version* of a game is shared by all outputs.

## Outputs, links and copies

**What is an output?**
A folder arranged for one program: RetroArch, ES-DE, RetroDECK, Batocera, RetroBat,
plain folders, or an SD card / USB drive. You can have several. Each game in it is a
link to your file (or to the converted disc), so outputs take almost no space.

**Why does Windows ask to restart as administrator?**
Windows only lets administrators, or anyone with Developer Mode turned on, create
symbolic links. Say yes once, or turn on Developer Mode (Settings → For developers), or
choose "Use copies instead": outputs then hold real copies, which work everywhere but
use disk space. Settings → "When Windows needs administrator rights" changes the
choice later.

**What are hard links, and why are they off?**
A hard link is the same file under a second name: it costs no space and works where
symbolic links don't, but editing the output file (a save written into the ROM, a
patch) edits your original too. Turn them on only if you understand that
(Settings → Allow hard links). They also only work on the same drive.

**I moved or renamed my game folder and the output is broken.**
Remove the old folder on the Library tab, add the new one, and Sync. Outputs → **Check
files** lists what is broken; **Repair** removes those entries and recreates them.

**Can an output be on a network drive?**
No. Links and copies are unreliable on network shares, so RomCure refuses them as
outputs. Your *games* may be on a network drive.

**How do I remove an output?**
Outputs tab → Remove. You can choose to delete the links and copies it made (never
your own files) or leave them.

## Frontends

**RetroArch**
Make a RetroArch output. In RetroArch, set Settings → Directory → Playlists to the
output folder (the `.lpl` playlists are there), or point the File Browser at it. Games
appear by system with clean names; multi-disc games appear once, as an `.m3u`
playlist, with their discs in a hidden `.discs` folder.

**ES-DE, RetroDECK, Batocera, RetroBat**
Make an output of that kind in (or point the frontend's ROM directory at) its ROMs
folder. Folder names follow each frontend's conventions; ES-DE's have been checked
against a real install, the others come from their documentation.

**Which RetroArch cores read CHD and RVZ?**
CHD: the common PlayStation (Beetle PSX, PCSX ReARMed, SwanStation), Saturn, Sega CD,
PC Engine CD, Dreamcast (Flycast), PS2 (LRPS2/PCSX2) and PSP (PPSSPP) cores. RVZ:
Dolphin. The default CHD codecs are the ones every current core reads; only change
them (Settings) if you know your core supports more.

## Disc conversion

**Why are discs converted?**
CHD and RVZ are lossless, often much smaller, and a single file instead of a `.cue`
with many `.bin` tracks. RomCure converts only discs that need it (inside archives, or
`.iso`/`.cue`/`.gdi`), verifies each result, and keeps it in a cache, so each disc is
converted once. Your originals are untouched.

**Where do converted files go, and how big does it get?**
In the cache folder (Settings → Cache folder; Windows default
`%LocalAppData%\romcure`). It can grow to many GB: roughly the compressed size of
every converted disc that some output uses. You can move it in Settings; files already
converted keep being used where they are.

**It says there is not enough scratch space.**
Converting needs the full uncompressed image in the scratch folder first (a Wii disc
is about 9 GB, a PS2 DVD up to 8 GB). Point Settings → Scratch folder at a drive with
room. "Conversions at once" multiplies that need.

**Conversion is slow.**
A CD takes about a minute, a DVD several minutes, once. "Conversions at once" in
Settings runs several in parallel if you have the disk space.

**"chdman was not found" / "dolphin-tool was not found"**
Help → Tools shows what RomCure found. On Windows they ship in the `tools` folder next
to the program; on Linux and macOS install them (see the README) so they are on the
PATH. Without them, discs are linked as they are, or reported if a system needs
conversion.

## SD cards and USB drives

**How do I fill an SD card for a handheld?**
Add an output of kind "SD card / USB (copy)" on the card (setup offers it when a card
is plugged in), start it with no games, tick the games you want on the Games tab, and
Apply. The changes bar shows the size and whether it fits before anything is copied.

**Why copies?**
SD cards are usually FAT32 or exFAT, which can't hold links. RomCure copies, and the
next Apply only copies what changed.

**A game is too big for the card.**
FAT32 can't hold a file over 4 GB; such files are listed and skipped. Format the card
as exFAT if your device supports it.

## Archives and damaged files

**Which archives are supported?**
zip, 7z and RAR (RAR5, solid and multi-part `.part1.rar` sets; RomCure reads the
first part and finds the rest). A cartridge game alone in a zip is linked as the zip
(RetroArch and most frontends load zips); 7z and RAR games, and zips that hold more
than the game, are extracted once into the cache. Disc games are converted (see Disc
conversion).

**A password-protected archive doesn't work.**
Encrypted archives aren't supported; they are listed as unreadable.

## Files, folders and troubleshooting

**Where does RomCure keep its data?**
Settings and the library database: `%AppData%\romcure` on Windows,
`~/Library/Application Support/romcure` on macOS, `~/.config/romcure` on Linux.
Converted discs and extracted games: the cache folder (Settings). The data folder is
also shown on the Help tab.

**How do I start over?**
Close RomCure and delete the data folder (and the cache folder if you want to drop
converted discs too). Your games and outputs are not affected; remove outputs in the
app first if you want their links gone.

**How do I run the setup again?**
Help → Run setup again. Nothing already set up is removed.

**How do I report a problem?**
Help → Save diagnostics…, then open an issue on GitHub with the zip and what you did.
