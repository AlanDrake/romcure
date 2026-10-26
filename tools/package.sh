#!/usr/bin/env bash
# Assembles a portable release folder and archive for one platform:
#
#   romcure-<version>-<platform>/
#     romcure(.exe)              the app (from bin/)
#     tools/                     chdman, dolphin-tool, 7-Zip (from tools/bin/<platform>/)
#     LICENSE, THIRD_PARTY_NOTICES.txt, README.txt
#     SOURCES.txt                where the source of each shipped tool is
#
# Usage: tools/package.sh <platform> <version>   e.g. windows-amd64 v0.1.0
# Run after `wails3 task build` and tools/fetch-tools.sh. Output: dist/.
# The app finds tools/ next to its executable (cmd/romcure findTools).
set -euo pipefail
cd "$(dirname "$0")/.."

platform="${1:?platform, e.g. windows-amd64}"
version="${2:?version, e.g. v0.1.0}"
name="romcure-$version-$platform"
exe="romcure"
case "$platform" in windows-*) exe="romcure.exe" ;; esac

[ -f "bin/$exe" ] || { echo "bin/$exe not found: build the app first" >&2; exit 1; }

out="dist/$name"
rm -rf "$out"
mkdir -p "$out/tools"
cp "bin/$exe" "$out/"
cp LICENSE THIRD_PARTY_NOTICES.txt "$out/"

# Every converter built or fetched for this platform, with its license text.
if [ -d "tools/bin/$platform" ]; then
  for f in tools/bin/"$platform"/*; do
    [ -f "$f" ] && cp "$f" "$out/tools/"
  done
fi
cp tools/licenses/7zip-License.txt tools/licenses/LGPL-2.1.txt "$out/tools/" 2>/dev/null || true

# The shipped tools are the upstream projects' own releases; name where the
# source of each exact version is (GPL/LGPL source offer; also in the app's Help).
{
  echo "Programs shipped in tools/ are unmodified official releases. Their source:"
  for t in chdman dolphin 7zip; do
    line="$(grep -E "^$t +[^ ]+ +$platform " tools/versions.txt || true)"
    [ -n "$line" ] || continue
    v="$(echo "$line" | awk '{print $2}')"
    case "$t" in
      chdman)  echo "  chdman (MAME) $v: https://github.com/mamedev/mame/releases/tag/$v" ;;
      dolphin) echo "  DolphinTool (Dolphin) $v: https://github.com/dolphin-emu/dolphin/releases/tag/$v" ;;
      7zip)    echo "  7-Zip $v: https://github.com/ip7z/7zip/releases/tag/$v" ;;
    esac
  done
  echo "The complete source of each, at these exact versions, is also attached to the"
  echo "RomCure release this archive comes from: https://github.com/AlanDrake/romcure/releases"
  echo "(Dolphin's without Externals/Qt and Externals/FFmpeg-bin, which hold prebuilt"
  echo "Windows libraries that DolphinTool does not use)."
  echo "If you cannot find a source archive, ask at https://github.com/AlanDrake/romcure/issues."
} > "$out/SOURCES.txt"

cat > "$out/README.txt" <<EOF
RomCure $version ($platform)

Run $exe. Everything is in this folder; nothing is installed.
The tools/ folder holds the programs RomCure runs for you (chdman, dolphin-tool
and 7-Zip). Keep it next to $exe. On Linux and macOS, chdman and dolphin-tool
come from your system instead: install mame-tools (or rom-tools) and dolphin-emu.

Licenses: LICENSE (RomCure, GPL-3.0) and THIRD_PARTY_NOTICES.txt.
Source: https://github.com/AlanDrake/romcure
Where to get the source of each tool: SOURCES.txt.
EOF

mkdir -p dist
case "$platform" in
  windows-*)
    (cd dist && if command -v 7z >/dev/null 2>&1; then 7z a -tzip -bso0 "$name.zip" "$name"
     else powershell -NoProfile -Command "Compress-Archive -Force -Path '$name' -DestinationPath '$name.zip'"; fi)
    echo "dist/$name.zip" ;;
  *)
    tar -czf "dist/$name.tar.gz" -C dist "$name"
    echo "dist/$name.tar.gz" ;;
esac
