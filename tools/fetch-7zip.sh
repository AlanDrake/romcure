#!/usr/bin/env bash
# Downloads the official 7-Zip release pinned in tools/versions.txt, checks its
# SHA-256, and puts the program that reads RAR archives into
# tools/bin/<os>-<arch>/ (7z.exe + 7z.dll on Windows, 7zz elsewhere), together
# with 7-Zip's license text. RomCure runs it only as a separate program.
#
# Usage: tools/fetch-7zip.sh [windows-amd64|linux-amd64|linux-arm64|darwin-amd64|darwin-arm64]
# (default: this machine). Needs curl, and on Windows an installed 7z to unpack
# the installer.
set -euo pipefail
cd "$(dirname "$0")/.."

platform="${1:-}"
if [ -z "$platform" ]; then
  case "$(uname -s)" in
    Linux) os=linux ;; Darwin) os=darwin ;; *) os=windows ;;
  esac
  case "$(uname -m)" in
    x86_64 | AMD64) arch=amd64 ;; arm64 | aarch64) arch=arm64 ;; *) arch=amd64 ;;
  esac
  platform="$os-$arch"
fi

line="$(grep -E "^7zip +[^ ]+ +$platform " tools/versions.txt || true)"
[ -n "$line" ] || { echo "no 7zip pin for $platform in tools/versions.txt" >&2; exit 1; }
version="$(echo "$line" | awk '{print $2}')"
archive="$(echo "$line" | sed -E 's/.* archive=([^ ]+).*/\1/')"
want="$(echo "$line" | sed -E 's/.* archive_sha256=([^ ]+).*/\1/')"

out="tools/bin/$platform"
mkdir -p "$out"
work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT

echo "Downloading $archive (7-Zip $version)"
curl -fsSL -o "$work/$archive" "https://github.com/ip7z/7zip/releases/download/$version/$archive"

if command -v sha256sum >/dev/null; then got="$(sha256sum "$work/$archive" | awk '{print $1}')"
else got="$(shasum -a 256 "$work/$archive" | awk '{print $1}')"; fi
if [ "$got" != "$want" ]; then
  echo "SHA-256 mismatch for $archive: got $got, pinned $want" >&2
  exit 1
fi

case "$platform" in
  windows-*)
    sz=""
    for c in 7z 7zz "/c/Program Files/7-Zip/7z.exe" "/c/Program Files (x86)/7-Zip/7z.exe"; do
      if command -v "$c" >/dev/null 2>&1; then sz="$c"; break; fi
    done
    [ -n "$sz" ] || { echo "7-Zip is needed to unpack the installer" >&2; exit 1; }
    "$sz" x -y "-o$work/x" "$work/$archive" 7z.exe 7z.dll License.txt >/dev/null
    cp "$work/x/7z.exe" "$work/x/7z.dll" "$out/"
    cp "$work/x/License.txt" "$out/7zip-License.txt"
    ;;
  *)
    mkdir "$work/x"
    tar -xJf "$work/$archive" -C "$work/x"
    cp "$work/x/7zz" "$out/7zz"
    chmod +x "$out/7zz"
    cp "$work/x/License.txt" "$out/7zip-License.txt"
    ;;
esac
echo "7-Zip $version installed in $out"
