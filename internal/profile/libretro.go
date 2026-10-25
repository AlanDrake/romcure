// Package profile generates per-consumer output layouts (RetroArch,
// ES-family, raw, device export) that link directly to ultimate sources.
// See spec §2, §2.4, §2.5.
package profile

import (
	"sort"
	"strings"
)

// libretroDBNames maps an internal system_id to the RetroArch/Libretro
// database name used for playlist (.lpl) folder/file naming and the
// db_name field within one. These intentionally match No-Intro/Redump DAT
// header <name> values, since libretro's own database is built from them.
//
// Deliberately partial: covers common systems, not the full libretro
// database (100+ entries). LibretroDBName reports ok=false for anything
// not listed here, so callers can fall back sensibly (e.g. the DAT's own
// header name, or systemID itself) rather than silently mislabeling.
var libretroDBNames = map[string]string{
	"nes":        "Nintendo - Nintendo Entertainment System",
	"snes":       "Nintendo - Super Nintendo Entertainment System",
	"n64":        "Nintendo - Nintendo 64",
	"gb":         "Nintendo - Game Boy",
	"gbc":        "Nintendo - Game Boy Color",
	"gba":        "Nintendo - Game Boy Advance",
	"nds":        "Nintendo - Nintendo DS",
	"gc":         "Nintendo - GameCube",
	"wii":        "Nintendo - Wii",
	"psx":        "Sony - PlayStation",
	"ps2":        "Sony - PlayStation 2",
	"psp":        "Sony - PlayStation Portable",
	"genesis":    "Sega - Mega Drive - Genesis",
	"segacd":     "Sega - Mega-CD - Sega CD",
	"saturn":     "Sega - Saturn",
	"dreamcast":  "Sega - Dreamcast",
	"gamegear":   "Sega - Game Gear",
	"sms":        "Sega - Master System - Mark III",
	"pce":        "NEC - PC Engine - TurboGrafx 16",
	"pcecd":      "NEC - PC Engine CD - TurboGrafx-CD",
	"ngp":        "SNK - Neo Geo Pocket",
	"ngpc":       "SNK - Neo Geo Pocket Color",
	"ws":         "Bandai - WonderSwan",
	"wsc":        "Bandai - WonderSwan Color",
	"3do":        "The 3DO Company - 3DO",
	"atari2600":  "Atari - 2600",
	"atari7800":  "Atari - 7800",
	"lynx":       "Atari - Lynx",
	"jaguar":     "Atari - Jaguar",
	"msx":        "Microsoft - MSX",
	"amiga":      "Commodore - Amiga",
	"c64":        "Commodore - 64",
	"zxspectrum": "Sinclair - ZX Spectrum",
	"atarist":    "Atari - ST",
	"amstradcpc": "Amstrad - CPC",
}

// discSystems identifies systems whose media is a disc image rather than
// a cartridge dump. Archive policy (spec §2.8) defers these to the
// transcode pipeline (M4) instead of passthrough/extract.
var discSystems = map[string]bool{
	"psx": true, "ps2": true, "psp": true, "gc": true, "wii": true,
	"segacd": true, "saturn": true, "dreamcast": true,
	"pcecd": true, "3do": true,
}

// LibretroDBName returns the RetroArch/Libretro database name for
// systemID, or ok=false if it isn't in the (intentionally partial) map.
func LibretroDBName(systemID string) (name string, ok bool) {
	name, ok = libretroDBNames[systemID]
	return name, ok
}

// IsDiscSystem reports whether systemID's media is a disc image.
func IsDiscSystem(systemID string) bool {
	return discSystems[systemID]
}

// SystemInfo names a system id for display.
type SystemInfo struct {
	ID   string
	Name string
	// Short is a compact label for lists ("SNES", "PS1").
	Short string
}

var shortNames = map[string]string{
	"psx": "PS1", "genesis": "MD", "segacd": "MCD", "saturn": "SS", "dreamcast": "DC", "gamegear": "GG",
	"sms": "SMS", "pce": "PCE", "pcecd": "PCE-CD", "atari2600": "2600", "atari7800": "7800", "zxspectrum": "ZX",
	"atarist": "ST", "amstradcpc": "CPC", "gc": "GCN", "ngp": "NGP", "ngpc": "NGPC", "ws": "WS", "wsc": "WSC",
}

// ShortName is a compact label for a system id.
func ShortName(id string) string {
	if s, ok := shortNames[id]; ok {
		return s
	}
	return strings.ToUpper(id)
}

// KnownSystems lists every system with a display name, sorted by name.
// Systems without a libretro database name are not offered in pickers.
func KnownSystems() []SystemInfo {
	out := make([]SystemInfo, 0, len(libretroDBNames))
	for id, name := range libretroDBNames {
		out = append(out, SystemInfo{ID: id, Name: name, Short: ShortName(id)})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}
