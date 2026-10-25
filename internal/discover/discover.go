// Package discover helps first-run setup: it proposes which system a folder
// of games belongs to (from the folder name, then from the file types) and
// where outputs could go (installed frontends, removable drives). It only
// reads; nothing is written or changed. No Wails imports.
package discover

import (
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"romcure/internal/match"
	"romcure/internal/profile"
)

// Candidate is a folder that holds games, with the system it probably holds.
type Candidate struct {
	Folder     string
	Name       string // the folder's own name
	SystemID   string // "" when it could not be told
	SystemName string
	Games      int    // game files found inside (archives count as one)
	Reason     string // "folder name" or "file types"; "" when SystemID is ""
}

// maxFiles bounds how much of one folder is looked at.
const maxFiles = 20000

// aliases maps a normalized folder name to a system id. A name that merely
// contains a long alias also matches ("Sega Genesis (Mega Drive) games").
var aliases = map[string]string{
	"nes": "nes", "nintendoentertainmentsystem": "nes", "nintendones": "nes", "famicom": "nes",
	"snes": "snes", "supernintendo": "snes", "supernes": "snes", "superfamicom": "snes", "sfc": "snes",
	"supernintendoentertainmentsystem": "snes", "nintendosnes": "snes",
	"n64": "n64", "nintendo64": "n64",
	"gb": "gb", "gameboy": "gb", "nintendogb": "gb", "nintendogameboy": "gb",
	"gbc": "gbc", "gameboycolor": "gbc", "nintendogbc": "gbc", "nintendogameboycolor": "gbc",
	"gba": "gba", "gameboyadvance": "gba", "nintendogba": "gba", "nintendogameboyadvance": "gba",
	"nds": "nds", "nintendods": "nds", "ds": "nds",
	"gc": "gc", "gamecube": "gc", "ngc": "gc", "nintendogamecube": "gc",
	"wii": "wii", "nintendowii": "wii",
	"psx": "psx", "ps1": "psx", "psone": "psx", "playstation": "psx", "sonyplaystation": "psx", "playstation1": "psx",
	"ps2": "ps2", "playstation2": "ps2", "sonyplaystation2": "ps2",
	"psp": "psp", "playstationportable": "psp", "sonyplaystationportable": "psp",
	"genesis": "genesis", "megadrive": "genesis", "segamegadrive": "genesis", "segagenesis": "genesis", "md": "genesis",
	"segacd": "segacd", "megacd": "segacd", "segamegacd": "segacd",
	"saturn": "saturn", "segasaturn": "saturn",
	"dreamcast": "dreamcast", "segadreamcast": "dreamcast", "dc": "dreamcast",
	"gamegear": "gamegear", "segagamegear": "gamegear", "gg": "gamegear",
	"mastersystem": "sms", "segamastersystem": "sms", "sms": "sms",
	"pcengine": "pce", "turbografx": "pce", "turbografx16": "pce", "tg16": "pce", "pce": "pce",
	"pcenginecd": "pcecd", "turbografxcd": "pcecd", "pcecd": "pcecd",
	"neogeopocket": "ngp", "neogeopocketcolor": "ngpc",
	"wonderswan": "ws", "wonderswancolor": "wsc",
	"3do":       "3do",
	"atari2600": "atari2600", "2600": "atari2600", "atari7800": "atari7800", "7800": "atari7800",
	"atarilynx": "lynx", "lynx": "lynx", "atarijaguar": "jaguar", "jaguar": "jaguar",
	"msx": "msx", "msx2": "msx",
	"amiga": "amiga", "amiga500": "amiga", "amiga1200": "amiga", "commodoreamiga": "amiga",
	"c64": "c64", "commodore64": "c64",
	"zxspectrum": "zxspectrum", "spectrum": "zxspectrum", "sinclairzxspectrum": "zxspectrum",
	"atarist": "atarist", "amstradcpc": "amstradcpc", "cpc": "amstradcpc",
}

// extensions maps a file extension that belongs to one system to it.
var extensions = map[string]string{
	".nes": "nes", ".sfc": "snes", ".smc": "snes", ".n64": "n64", ".z64": "n64", ".v64": "n64",
	".gb": "gb", ".gbc": "gbc", ".gba": "gba", ".nds": "nds", ".gcm": "gc", ".rvz": "gc", ".gcz": "gc", ".wbfs": "wii",
	".gen": "genesis", ".md": "genesis", ".smd": "genesis", ".sms": "sms", ".gg": "gamegear",
	".pce": "pce", ".ngp": "ngp", ".ws": "ws", ".wsc": "wsc", ".a26": "atari2600", ".a78": "atari7800",
	".lnx": "lynx", ".j64": "jaguar", ".adf": "amiga", ".d64": "c64", ".t64": "c64", ".prg": "c64",
	".tzx": "zxspectrum", ".tap": "zxspectrum", ".z80": "zxspectrum", ".sna": "zxspectrum",
	".cso": "psp", ".pbp": "psp",
}

func normalize(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// SystemFromName proposes a system for a folder name ("" = no idea).
func SystemFromName(name string) string {
	n := normalize(name)
	if id, ok := aliases[n]; ok {
		return id
	}
	best, bestLen := "", 0
	for alias, id := range aliases {
		if len(alias) >= 6 && len(alias) > bestLen && strings.Contains(n, alias) {
			best, bestLen = id, len(alias)
		}
	}
	return best
}

type listing struct {
	games int
	exts  map[string]int
}

// look counts the game files under dir (stopping at maxFiles).
func look(dir, systemHint string) listing {
	l := listing{exts: map[string]int{}}
	filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if path != dir && strings.HasPrefix(d.Name(), ".") {
				return fs.SkipDir
			}
			return nil
		}
		if match.IsSidecar(systemHint, d.Name()) {
			return nil
		}
		l.games++
		l.exts[strings.ToLower(filepath.Ext(d.Name()))]++
		if l.games >= maxFiles {
			return fs.SkipAll
		}
		return nil
	})
	return l
}

// systemFromExtensions returns the system the files belong to, when the
// recognizable ones agree (80%) and make up most of the folder (half): a few
// stray .adf files in a folder of thousands of other files say nothing.
func systemFromExtensions(exts map[string]int) string {
	votes := map[string]int{}
	total, all := 0, 0
	for e, n := range exts {
		all += n
		if id, ok := extensions[e]; ok {
			votes[id] += n
			total += n
		}
	}
	if total*2 < all {
		return ""
	}
	for id, n := range votes {
		if total > 0 && n*100/total >= 80 {
			return id
		}
	}
	return ""
}

func systemName(id string) string {
	for _, s := range profile.KnownSystems() {
		if s.ID == id {
			return s.Name
		}
	}
	return id
}

// Folders looks at parent and the folders inside it (down to three levels)
// and lists those that hold games, with a proposed system for each. A folder
// that cannot be placed is still listed (SystemID empty) when it is a direct
// child of parent, so nothing is silently dropped.
func Folders(parent string) ([]Candidate, error) {
	if st, err := os.Stat(parent); err != nil {
		return nil, err
	} else if !st.IsDir() {
		return nil, &os.PathError{Op: "discover", Path: parent, Err: os.ErrInvalid}
	}
	var out []Candidate
	var visit func(dir string, depth int) bool
	visit = func(dir string, depth int) bool {
		name := filepath.Base(dir)
		id := SystemFromName(name)
		l := look(dir, id)
		if l.games == 0 {
			return false
		}
		reason := "folder name"
		if id == "" {
			id, reason = systemFromExtensions(l.exts), "file types"
		}
		if id != "" {
			out = append(out, Candidate{Folder: dir, Name: name, SystemID: id, SystemName: systemName(id), Games: l.games, Reason: reason})
			return true
		}
		found := false
		if depth < 3 {
			entries, _ := os.ReadDir(dir)
			for _, e := range entries {
				if e.IsDir() && !strings.HasPrefix(e.Name(), ".") {
					if visit(filepath.Join(dir, e.Name()), depth+1) {
						found = true
					}
				}
			}
		}
		if !found && depth <= 1 {
			out = append(out, Candidate{Folder: dir, Name: name, Games: l.games})
			return true
		}
		return found
	}
	visit(parent, 0)
	sort.SliceStable(out, func(i, j int) bool { return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name) })
	return out, nil
}
