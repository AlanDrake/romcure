# Third-party executables

Bundled tools are invoked only as subprocesses (HANDOFF §3). Nothing here is
compiled: they are the upstream projects' own releases. Binaries live in
`tools/bin/<os>-<arch>/` and are **not committed** (see `.gitignore`); this
folder tracks `versions.txt` (pins with SHA-256), the fetch/package scripts and
`licenses/`.

`tools/fetch-tools.sh [platform]` downloads, verifies and unpacks them (CI and
the release workflow run it); `tools/package.sh` assembles a release archive.

| Tool | Version | Platforms | Source of the download |
|---|---|---|---|
| chdman | MAME 0.289 | windows-amd64, windows-arm64 | the `mame0289b_*.exe` release asset (a self-extracting archive; `chdman.exe` is taken out of it) |
| DolphinTool | Dolphin 2609 | windows-amd64, windows-arm64 | `dl.dolphin-emu.org/releases/2609/dolphin-2609-<arch>.7z` |
| 7-Zip | 26.05 | all | `github.com/ip7z/7zip/releases` |

chdman and DolphinTool are GPL-2.0-or-later; 7-Zip is LGPL. Linux and macOS have
no official chdman or dolphin-tool: RomCure finds them on the PATH there.

To update a pin: edit `versions.txt` (version, url, SHA-256) and run
`go run ./tools/gen-notices`.

## 7-Zip (reads RAR)

`tools/fetch-7zip.sh [platform]` downloads the official 7-Zip release pinned in
`versions.txt` (from https://github.com/ip7z/7zip/releases), checks its SHA-256,
and installs `7z.exe` + `7z.dll` (Windows) or `7zz` (Linux, macOS) into
`tools/bin/<os>-<arch>/` with its license. CI runs it before the tests. Unlike
chdman and dolphin-tool, the official binaries are what releases ship: 7-Zip is
LGPL, so the release only needs to carry its license text (`tools/licenses/`) and
a pointer to its source (the same release page; the app's Help screen says so).
7-Zip is used as a separate program (`internal/rararc`) and only to read: list a
RAR's headers and extract one file at a time.
