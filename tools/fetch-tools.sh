#!/usr/bin/env bash
# Downloads the official, unmodified builds of the programs RomCure runs, checks
# each against the SHA-256 pinned in tools/versions.txt, and puts them in
# tools/bin/<os>-<arch>/ (where the app and tools/package.sh look for them).
#
#   chdman       MAME's official Windows release (GPL-2.0+)    windows only
#   DolphinTool  Dolphin's official Windows release (GPL-2.0+) windows only
#   7-Zip        official release (LGPL), reads RAR           all platforms
#
# Nothing is compiled. The upstream projects publish no chdman for Linux/macOS
# and no dolphin-tool for macOS/Linux: there RomCure finds them on the PATH
# (mame-tools / rom-tools, dolphin-emu).
#
# Usage: tools/fetch-tools.sh [windows-amd64|windows-arm64|linux-amd64|linux-arm64|darwin-amd64|darwin-arm64]
# (default: this machine). Needs curl, and 7z on Windows to unpack.
set -euo pipefail
cd "$(dirname "$0")/.."

platform="${1:-}"
if [ -z "$platform" ]; then
  case "$(uname -s)" in Linux) os=linux ;; Darwin) os=darwin ;; *) os=windows ;; esac
  case "$(uname -m)" in x86_64 | AMD64) arch=amd64 ;; arm64 | aarch64) arch=arm64 ;; *) arch=amd64 ;; esac
  platform="$os-$arch"
fi
out="tools/bin/$platform"
mkdir -p "$out"

bash tools/fetch-7zip.sh "$platform"

case "$platform" in windows-*) ;; *)
  echo "No official chdman or dolphin-tool exists for $platform: install them (mame-tools or rom-tools; dolphin-emu) and RomCure will find them on the PATH."
  exit 0 ;;
esac

sz=""
for c in 7z 7zz "/c/Program Files/7-Zip/7z.exe" "/c/Program Files (x86)/7-Zip/7z.exe" "$out/7z.exe"; do
  if command -v "$c" >/dev/null 2>&1 || [ -x "$c" ]; then sz="$c"; break; fi
done
[ -n "$sz" ] || { echo "7-Zip is needed to unpack the downloads" >&2; exit 1; }

sha() { if command -v sha256sum >/dev/null; then sha256sum "$1" | awk '{print $1}'; else shasum -a 256 "$1" | awk '{print $1}'; fi; }
work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT

# fetch <tool> <inner file to extract>: download the pinned archive, verify, extract.
fetch() {
  local tool="$1" inner="$2" line version archive url want
  line="$(grep -E "^$tool +[^ ]+ +$platform " tools/versions.txt || true)"
  [ -n "$line" ] || { echo "no $tool pin for $platform in tools/versions.txt" >&2; exit 1; }
  version="$(echo "$line" | awk '{print $2}')"
  archive="$(echo "$line" | sed -E 's/.* archive=([^ ]+).*/\1/')"
  url="$(echo "$line" | sed -E 's/.* url=([^ ]+).*/\1/')"
  want="$(echo "$line" | sed -E 's/.* archive_sha256=([^ ]+).*/\1/')"
  echo "Downloading $archive ($tool $version)"
  curl -fsSL -o "$work/$archive" "$url"
  [ "$(sha "$work/$archive")" = "$want" ] || { echo "SHA-256 mismatch for $archive (pinned $want)" >&2; exit 1; }
  "$sz" e -y -r "-o$work/x-$tool" "$work/$archive" "$inner" >/dev/null
  cp -p "$work/x-$tool/$inner" "$out/"
  echo "$tool $version installed in $out"
}

fetch chdman chdman.exe
fetch dolphin DolphinTool.exe
