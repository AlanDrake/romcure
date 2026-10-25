package discover

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
)

// Core is a libretro core chosen as a playlist's default: the file RetroArch
// loads, the name its .info file gives it (what a playlist records), and why
// it was chosen, in words for the sync report.
type Core struct {
	Path   string
	Name   string
	Reason string
}

// recommendedCores lists, per system, the RetroArch cores ES-DE offers, in
// ES-DE's order: the first is the one ES-DE launches by default, which is the
// recommendation we follow. Taken from the es_systems.xml of ES-DE 3.x. Names
// are the core file's name without "_libretro.<ext>". Cores not listed here but
// whose .info says they support the system can still be chosen, after these.
var recommendedCores = map[string][]string{
	"nes":        {"mesen", "mesen2", "nestopia", "fceumm", "quicknes", "rustynes"},
	"snes":       {"snes9x", "snes9x2010", "snes9x2005_plus", "bsnes", "bsnes_hd_beta", "bsnes-jg", "bsnes_mercury_accuracy", "mednafen_supafaust", "mesen-s", "mesen2"},
	"n64":        {"mupen64plus_next", "parallel_n64"},
	"gb":         {"gambatte", "sameboy", "gearboy", "tgbdual", "DoubleCherryGB", "irogb", "mesen-s", "mesen2", "bsnes", "mgba", "vbam", "skyemu"},
	"gbc":        {"gambatte", "sameboy", "gearboy", "tgbdual", "DoubleCherryGB", "irogb", "mesen-s", "mesen2", "bsnes", "mgba", "vbam", "skyemu"},
	"gba":        {"mgba", "vbam", "vba_next", "gpsp", "noods", "skyemu", "mesen2"},
	"nds":        {"melondsds", "melonds", "desmume", "desmume2015", "noods", "skyemu"},
	"gc":         {"dolphin"},
	"wii":        {"dolphin"},
	"psx":        {"mednafen_psx", "mednafen_psx_hw", "pcsx_rearmed", "swanstation"},
	"ps2":        {"pcsx2"},
	"psp":        {"ppsspp"},
	"genesis":    {"genesis_plus_gx", "genesis_plus_gx_wide", "picodrive", "blastem", "clownmdemu"},
	"segacd":     {"genesis_plus_gx", "genesis_plus_gx_wide", "picodrive", "blastem", "clownmdemu"},
	"saturn":     {"mednafen_saturn", "kronos", "yabasanshiro", "yabause"},
	"dreamcast":  {"flycast"},
	"gamegear":   {"genesis_plus_gx", "genesis_plus_gx_wide", "gearsystem", "smsplus", "picodrive", "blastem", "mesen2"},
	"sms":        {"genesis_plus_gx", "genesis_plus_gx_wide", "smsplus", "gearsystem", "picodrive", "blastem", "mesen2"},
	"pce":        {"mednafen_pce", "mednafen_pce_fast", "mednafen_supergrafx", "geargrafx", "mesen2"},
	"pcecd":      {"mednafen_pce", "mednafen_pce_fast", "mednafen_supergrafx", "geargrafx", "mesen2"},
	"ngp":        {"mednafen_ngp", "race"},
	"ngpc":       {"mednafen_ngp", "race"},
	"ws":         {"mednafen_wswan", "mesen2"},
	"wsc":        {"mednafen_wswan", "mesen2"},
	"3do":        {"opera"},
	"atari2600":  {"stella", "stella2014", "stella2023", "tia"},
	"atari7800":  {"prosystem", "mame"},
	"lynx":       {"handy", "mednafen_lynx", "gearlynx", "holani"},
	"jaguar":     {"virtualjaguar"},
	"msx":        {"bluemsx", "fmsx"},
	"amiga":      {"puae", "puae2021", "amiberry"},
	"c64":        {"vice_x64sc", "vice_x64", "vice_xscpu64", "vice_x128", "frodo"},
	"zxspectrum": {"fuse"},
	"atarist":    {"hatari", "hatari2014"},
	"amstradcpc": {"cap32", "crocods"},
	"atari800":   {"atari800"},
	"apple2":     {"applewin", "mame"},
}

// alwaysOpenable are content types RetroArch handles for every core: it
// unpacks archives itself when a core doesn't read them.
var alwaysOpenable = map[string]bool{"zip": true, "7z": true}

func coreExt() string {
	switch runtime.GOOS {
	case "windows":
		return ".dll"
	case "darwin":
		return ".dylib"
	}
	return ".so"
}

// coreInfo is what RetroArch's .info file says about one installed core.
type coreInfo struct {
	name         string          // file name without "_libretro.<ext>"
	path         string          // the core file
	display      string          // display_name, or name when there is no .info
	exts         map[string]bool // supported_extensions; nil = unknown (no .info)
	databases    map[string]bool // the libretro databases it is for
	experimental bool
	firmware     []string // BIOS files it can't run without, relative to the system folder
}

// CoreChooser picks the default core of each system's playlist in one
// RetroArch. Build it once per Apply: it reads every core's .info file and
// what the user's playlists and history already use.
type CoreChooser struct {
	in    RetroArchInstall
	cores map[string]coreInfo // by name
	used  map[string]map[string]int
}

// NewCoreChooser reads the installed cores of a RetroArch and which core the
// user already runs each kind of game with (playlist entries and the history,
// by database name).
func (in RetroArchInstall) NewCoreChooser() *CoreChooser {
	c := &CoreChooser{in: in, cores: map[string]coreInfo{}, used: map[string]map[string]int{}}
	files, _ := filepath.Glob(filepath.Join(in.Cores, "*_libretro"+coreExt()))
	for _, f := range files {
		name := strings.TrimSuffix(filepath.Base(f), "_libretro"+coreExt())
		c.cores[name] = readCoreInfo(name, f, filepath.Join(in.Info, name+"_libretro.info"))
	}
	lists, _ := filepath.Glob(filepath.Join(in.Playlists, "*.lpl"))
	for _, l := range lists {
		c.countUse(l)
	}
	return c
}

// countUse adds the cores a playlist's entries name to the usage counts.
// Entries left on "DETECT" say nothing about the user's choice.
func (c *CoreChooser) countUse(path string) {
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	var pl struct {
		Items []struct {
			CorePath string `json:"core_path"`
			DBName   string `json:"db_name"`
		} `json:"items"`
	}
	if json.Unmarshal(data, &pl) != nil {
		return
	}
	for _, it := range pl.Items {
		if it.CorePath == "" || it.CorePath == "DETECT" || it.DBName == "" {
			continue
		}
		db := strings.TrimSuffix(it.DBName, ".lpl")
		name := strings.TrimSuffix(filepath.Base(strings.ReplaceAll(it.CorePath, `\`, "/")), "_libretro"+filepath.Ext(it.CorePath))
		if c.used[db] == nil {
			c.used[db] = map[string]int{}
		}
		c.used[db][name]++
	}
}

func readCoreInfo(name, path, infoPath string) coreInfo {
	ci := coreInfo{name: name, path: path, display: name}
	f, err := os.Open(infoPath)
	if err != nil {
		return ci
	}
	defer f.Close()
	kv := map[string]string{}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		if k, v, ok := strings.Cut(sc.Text(), "="); ok {
			kv[strings.TrimSpace(k)] = strings.Trim(strings.TrimSpace(v), `"`)
		}
	}
	if d := kv["display_name"]; d != "" {
		ci.display = d
	}
	ci.exts = splitSet(strings.ToLower(kv["supported_extensions"]))
	ci.databases = splitSet(kv["database"])
	ci.experimental = kv["is_experimental"] == "true"
	for i := 0; ; i++ {
		p, ok := kv["firmware"+strconv.Itoa(i)+"_path"]
		if !ok {
			break
		}
		if kv["firmware"+strconv.Itoa(i)+"_opt"] != "true" {
			ci.firmware = append(ci.firmware, p)
		}
	}
	return ci
}

func splitSet(s string) map[string]bool {
	m := map[string]bool{}
	for _, p := range strings.Split(s, "|") {
		if p = strings.TrimSpace(p); p != "" {
			m[p] = true
		}
	}
	return m
}

// opens reports whether the core reads every content type given. A core with
// no .info is given the benefit of the doubt.
func (ci coreInfo) opens(exts []string) bool {
	if ci.exts == nil {
		return true
	}
	for _, e := range exts {
		if !ci.exts[e] && !alwaysOpenable[e] {
			return false
		}
	}
	return true
}

// missingBIOS returns the first required BIOS file that isn't in the system
// folder ("" when none is missing).
func (c *CoreChooser) missingBIOS(ci coreInfo) string {
	for _, f := range ci.firmware {
		if _, err := os.Stat(filepath.Join(c.in.System, filepath.FromSlash(f))); err != nil {
			return f
		}
	}
	return ""
}

// Choose picks the default core for a system whose playlist (database name
// dbName) holds games of the given content types (lower-case extensions
// without the dot, e.g. "chd", "m3u"). Candidates are the installed cores
// recommended for the system and any other installed core whose .info names
// the database. They are ranked by, in order: can open the content, has its
// BIOS, is the core the user already runs these games with, the recommended
// order. A core that can't open the content is never chosen.
func (c *CoreChooser) Choose(systemID, dbName string, exts []string) (Core, bool) {
	rank := map[string]int{}
	for i, n := range recommendedCores[systemID] {
		rank[n] = i
	}
	type cand struct {
		ci     coreInfo
		rec    int // position in the recommended list; len(list) for others
		used   int
		bios   string
		opens  bool
		listed bool
	}
	var cs []cand
	for name, ci := range c.cores {
		r, listed := rank[name]
		if !listed {
			if !ci.databases[dbName] || ci.experimental {
				continue
			}
			r = len(recommendedCores[systemID])
		}
		cs = append(cs, cand{ci: ci, rec: r, used: c.used[dbName][name], bios: c.missingBIOS(ci), opens: ci.opens(exts), listed: listed})
	}
	sort.Slice(cs, func(i, j int) bool {
		a, b := cs[i], cs[j]
		if a.opens != b.opens {
			return a.opens
		}
		if (a.bios == "") != (b.bios == "") {
			return a.bios == ""
		}
		if a.used != b.used {
			return a.used > b.used
		}
		if a.rec != b.rec {
			return a.rec < b.rec
		}
		return a.ci.name < b.ci.name
	})
	if len(cs) == 0 || !cs[0].opens {
		return Core{}, false
	}
	best := cs[0]
	core := Core{Path: best.ci.path, Name: best.ci.display}
	switch {
	case len(cs) == 1:
		core.Reason = "the only installed core for these games"
	case best.used > 0:
		core.Reason = "the core you already play these games with"
	case best.listed && best.rec == 0:
		core.Reason = "the recommended core"
	default:
		core.Reason = "the best installed choice"
		// Say why a better-ranked core lost.
		for _, o := range cs[1:] {
			if o.listed && o.rec < best.rec {
				if !o.opens {
					core.Reason += "; " + o.ci.display + " can't open these files"
				} else if o.bios != "" {
					core.Reason += "; " + o.ci.display + " needs " + o.bios + ", which is not in RetroArch's system folder"
				}
				break
			}
		}
		if best.bios != "" {
			core.Reason += " (its BIOS " + best.bios + " is missing too)"
		}
	}
	return core, true
}
