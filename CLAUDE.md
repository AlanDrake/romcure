# Working notes for Claude Code

Read first: `HANDOFF.md` (decisions, rules), `docs/STATUS.md` (current state,
known bugs, backlog), `rom-library-manager-spec.md` (requirements + checklists).
At the end of a session, update `docs/STATUS.md` and the spec checklists with
what was done and how it was verified (say plainly when something wasn't).

Machine-specific notes (tool paths, scratch disks, sample ROM folders, emulator
installs) are in `CLAUDE.local.md`, which is gitignored:

@CLAUDE.local.md

## Personal information: never committed

Nothing that identifies the owner goes into a committed file or a commit
message: no machine paths (drive letters, user folders, install folders), no
names of files or folders from the owner's collection (ROM, archive or member
names, game titles tied to their library), no emails, user names or device
names. Tests and examples use synthetic names ("Game (USA).zip", "LOADER.P00",
"Quest of Ages (1988)(Publisher)(Side A)"); notes on real-data checks give
counts, not names ("on a copy of the owner's GBA set: 32 of 34 named"). Tell
sub-agents this rule in their brief. Machine details belong in
`CLAUDE.local.md`.

## Commands

Go may not be on the PATH: `.claude/settings.local.json` (gitignored) can set
`BASH_ENV` to a script that adds Go, `wails3` and `PLAYWRIGHT_BROWSERS_PATH=0`
for Bash in this repo (see `CLAUDE.local.md`).

- Go checks: `go vet ./... && go test ./internal/...`
  (point `TEMP`/`TMP` at a fast scratch disk if there is one)
- Low memory: add `-p 2`.
- Cross-OS compile check (no Linux/macOS here; WSL can't start):
  `GOOS=linux go vet ./internal/...` and `GOOS=darwin ...`
- Frontend types: `cd frontend && npm run check`
- Bindings (gitignored, CI regenerates): `wails3 generate bindings -clean=true -ts -i ./...`
- Desktop build: `wails3 task build` → `bin/romcure.exe`
- E2E (headless Chromium against the app in Wails server mode):
  `cd frontend && npx playwright test`. Screenshots: `page.screenshot` into
  `frontend/test-results/` (gitignored) and view them with Read.
- Server-mode binary by hand: `go build -tags server,production -o bin/romcure-server.exe ./cmd/romcure`
  (the template's `task build:server` is broken), run with `WAILS_SERVER_PORT`.
- Running the real app without touching the user's data: set `APPDATA`
  and `LOCALAPPDATA` (Windows) to a scratch folder before starting it (new installs
  keep their big caches under `LOCALAPPDATA`).
- GitHub: `GH_TOKEN` (repo-scoped) comes from `.claude/settings.local.json`;
  use the REST API (`curl -H "Authorization: Bearer $GH_TOKEN" https://api.github.com/repos/AlanDrake/romcure/...`).
  Default branch is `master`; commit there as usual. **Don't push every
  commit** — each push to `master` re-runs the full 3-OS CI matrix, and
  CI minutes are tight. Keep committing locally and push only when it's
  actually needed: the user asks, a session's work is done, or something
  else needs the pushed state (e.g. a tag/release). Say so when leaving
  commits unpushed, so it's clear GitHub isn't current.

## Environment and test data

- Where the scratch disks, the sample ROM folders (read-only: never write
  there) and the emulator installs are: `CLAUDE.local.md`.
- A portable ES-DE install's `portable.txt` ignores `--home`, so run a copy
  without it to avoid touching the user's config.
- Tool binaries (chdman, DolphinTool, 7-Zip) in `tools/bin/windows-amd64/` (gitignored)
  come from `bash tools/fetch-tools.sh`: the official releases pinned in
  `tools/versions.txt`, nothing compiled. Release archives: `tools/package.sh`
  and `.github/workflows/release.yml`.
- Launch RetroArch/Dolphin with isolated configs (`--config` / `-u`) so the
  user's settings and playlists are untouched.

## Gotchas

- Large bash heredocs containing quotes or backslashes sometimes fail to
  parse; write files with the Write tool and edit with Edit instead.
- Wails: the template stylesheet made the whole page a drag region (fixed);
  keep `--wails-draggable: no-drag` on the page.
- Linux runners need the `gtk3` build tag for Wails (set in CI).
- macOS/Linux fail on things Windows tolerates (missing dirs, the root mount);
  run the cross-OS vet and watch CI after OS-sensitive changes.
