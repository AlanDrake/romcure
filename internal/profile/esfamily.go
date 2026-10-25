package profile

import (
	"fmt"

	"romcure/internal/store"
)

// Frontend is an EmulationStation-family frontend whose ROM folders are
// named per system (spec M5). They all read <roms>/<system folder>/, with
// .m3u files for multi-disc games, but disagree on the folder names.
type Frontend string

const (
	FrontendESDE      Frontend = "esde"
	FrontendRetroDECK Frontend = "retrodeck"
	FrontendBatocera  Frontend = "batocera"
	FrontendRetroBat  Frontend = "retrobat"
)

// folderMaps maps app system id → ROM folder name. ES-DE's column was
// checked against the es_systems.xml of an ES-DE 3.x install; RetroDECK
// bundles ES-DE and shares its names. Batocera's (also used by RetroBat,
// which follows it) comes from Batocera's documented system names and has
// NOT been checked against a real install.
var esdeFolders = map[string]string{
	"nes": "nes", "snes": "snes", "n64": "n64", "gb": "gb", "gbc": "gbc", "gba": "gba", "nds": "nds",
	"gc": "gc", "wii": "wii", "psx": "psx", "ps2": "ps2", "psp": "psp",
	"genesis": "genesis", "segacd": "segacd", "saturn": "saturn", "dreamcast": "dreamcast",
	"gamegear": "gamegear", "sms": "mastersystem", "pce": "pcengine", "pcecd": "pcenginecd",
	"ngp": "ngp", "ngpc": "ngpc", "ws": "wonderswan", "wsc": "wonderswancolor",
	"3do": "3do", "atari2600": "atari2600", "atari7800": "atari7800",
	"lynx": "atarilynx", "jaguar": "atarijaguar", "msx": "msx",
	"amiga": "amiga", "c64": "c64", "zxspectrum": "zxspectrum", "atarist": "atarist",
	"amstradcpc": "amstradcpc", "atari800": "atari800", "apple2": "apple2",
}

var batoceraFolders = map[string]string{
	"nes": "nes", "snes": "snes", "n64": "n64", "gb": "gb", "gbc": "gbc", "gba": "gba", "nds": "nds",
	"gc": "gamecube", "wii": "wii", "psx": "psx", "ps2": "ps2", "psp": "psp",
	"genesis": "megadrive", "segacd": "segacd", "saturn": "saturn", "dreamcast": "dreamcast",
	"gamegear": "gamegear", "sms": "mastersystem", "pce": "pcengine", "pcecd": "pcenginecd",
	"ngp": "ngp", "ngpc": "ngpc", "ws": "wswan", "wsc": "wswanc",
	"3do": "3do", "atari2600": "atari2600", "atari7800": "atari7800",
	"lynx": "lynx", "jaguar": "jaguar", "msx": "msx1",
	"amiga": "amiga500", "c64": "c64", "zxspectrum": "zxspectrum", "atarist": "atarist",
	"amstradcpc": "amstradcpc", "atari800": "atari800", "apple2": "apple2",
}

func (f Frontend) folders() (map[string]string, bool) {
	switch f {
	case FrontendESDE, FrontendRetroDECK:
		return esdeFolders, true
	case FrontendBatocera, FrontendRetroBat:
		return batoceraFolders, true
	}
	return nil, false
}

// FolderName returns the frontend's ROM folder for systemID.
func (f Frontend) FolderName(systemID string) (string, bool) {
	m, ok := f.folders()
	if !ok {
		return "", false
	}
	name, ok := m[systemID]
	return name, ok
}

// GenerateESFamily generates (or incrementally updates) an
// EmulationStation-family profile for systemID under opts.OutputRoot (the
// frontend's ROM root). Unlike RetroArch there is no playlist file: the
// frontend discovers games by scanning the folder. A system with no folder
// mapping for the frontend falls back to its own id, and the report says so.
func GenerateESFamily(db *store.DB, systemID string, fe Frontend, opts Options) (Report, error) {
	if _, ok := fe.folders(); !ok {
		return Report{}, fmt.Errorf("unknown frontend %q", fe)
	}
	if opts.ProfileID == "" {
		opts.ProfileID = string(fe)
	}
	folder, mapped := fe.FolderName(systemID)
	if !mapped {
		folder = systemID
	}
	report, _, err := generateLayout(db, systemID, folder, folder, opts)
	if !mapped {
		report.Notes = append(report.Notes, fmt.Sprintf("no %s folder is known for system %q; used %q — rename it or add a mapping", fe, systemID, folder))
	}
	return report, err
}
