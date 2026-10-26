#!/usr/bin/env bash
# Gathers the complete source of every GPL/LGPL program a release ships, at the
# exact versions pinned in tools/versions.txt ("source" lines), into
# dist/sources/, so each GitHub release carries them (spec §7: the safer way to
# meet the GPL's source obligation than a written offer alone).
#
#   chdman       MAME's own source release (mame<ver>s.exe, a self-extracting 7z)
#   7-Zip        7-Zip's own source archive
#   DolphinTool  Dolphin at the pinned tag's commit with its submodules, as a
#                tar.xz; Externals/Qt and Externals/FFmpeg-bin are left out:
#                they hold prebuilt Windows libraries, not source, and
#                DolphinTool uses neither.
#
# Usage: tools/fetch-sources.sh. Needs curl, git, tar and xz. Output: dist/sources/.
set -euo pipefail
cd "$(dirname "$0")/.."

out="dist/sources"
mkdir -p "$out"
sha() { if command -v sha256sum >/dev/null; then sha256sum "$1" | awk '{print $1}'; else shasum -a 256 "$1" | awk '{print $1}'; fi; }
field() { echo "$1" | sed -E "s/.* $2=([^ ]+).*/\1/"; }
pin() {
  local line
  line="$(grep -E "^$1 +[^ ]+ +source " tools/versions.txt || true)"
  [ -n "$line" ] || { echo "no $1 source pin in tools/versions.txt" >&2; exit 1; }
  echo "$line"
}

# Upstream source archives, checked against their pins.
for tool in chdman 7zip; do
  line="$(pin "$tool")"
  archive="$(field "$line" archive)"
  echo "Downloading $archive"
  curl -fsSL -o "$out/$archive" "$(field "$line" url)"
  [ "$(sha "$out/$archive")" = "$(field "$line" archive_sha256)" ] ||
    { echo "SHA-256 mismatch for $archive" >&2; exit 1; }
done

# Dolphin: the tag's commit and its submodules, without .git.
line="$(pin dolphin)"
version="$(echo "$line" | awk '{print $2}')"
commit="$(field "$line" commit)"
skip="$(field "$line" skip)"
work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT
src="$work/dolphin-$version"
git init -q "$src"
git -C "$src" remote add origin "$(field "$line" git)"
git -C "$src" fetch -q --depth 1 origin "$commit"
git -C "$src" checkout -q FETCH_HEAD
[ "$(git -C "$src" rev-parse HEAD)" = "$commit" ] || { echo "Dolphin: not at $commit" >&2; exit 1; }
want=()
for p in $(git -C "$src" config -f .gitmodules --get-regexp '\.path$' | awk '{print $2}'); do
  case ",$skip," in *",$p,"*) ;; *) want+=("$p") ;; esac
done
git -C "$src" submodule update -q --init --depth 1 --recursive -- "${want[@]}"
echo "Packing dolphin-$version-src.tar.xz"
tar --exclude=.git -C "$work" -cJf "$out/dolphin-$version-src.tar.xz" "dolphin-$version"

ls -l "$out"
