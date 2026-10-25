package discover

import (
	"bufio"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// Core is a libretro core that is installed: the file RetroArch loads and the
// name its .info file gives it, which is what a playlist records.
type Core struct {
	Path string
	Name string
}

// suggestedCores lists, per system, the libretro cores worth defaulting to, best
// first: the one most people use for that system, then alternatives. The first
// that is installed wins. Names are the core file's name without
// "_libretro.<ext>".
var suggestedCores = map[string][]string{
	"nes":        {"mesen", "nestopia", "fceumm", "quicknes"},
	"snes":       {"snes9x", "bsnes", "bsnes_mercury_balanced", "snes9x2010"},
	"n64":        {"mupen64plus_next", "parallel_n64"},
	"gb":         {"gambatte", "sameboy", "mgba"},
	"gbc":        {"gambatte", "sameboy", "mgba"},
	"gba":        {"mgba", "gpsp", "vba_next"},
	"nds":        {"melonds", "desmume", "melondsds"},
	"gc":         {"dolphin"},
	"wii":        {"dolphin"},
	"psx":        {"swanstation", "pcsx_rearmed", "mednafen_psx_hw", "mednafen_psx"},
	"ps2":        {"pcsx2", "lrps2"},
	"psp":        {"ppsspp"},
	"genesis":    {"genesis_plus_gx", "picodrive", "blastem"},
	"segacd":     {"genesis_plus_gx", "picodrive"},
	"saturn":     {"mednafen_saturn", "kronos", "yabause"},
	"dreamcast":  {"flycast"},
	"gamegear":   {"genesis_plus_gx", "gearsystem", "picodrive"},
	"sms":        {"genesis_plus_gx", "smsplus", "gearsystem", "picodrive"},
	"pce":        {"mednafen_pce_fast", "mednafen_pce"},
	"pcecd":      {"mednafen_pce_fast", "mednafen_pce"},
	"ngp":        {"mednafen_ngp"},
	"ngpc":       {"mednafen_ngp"},
	"ws":         {"mednafen_wswan"},
	"wsc":        {"mednafen_wswan"},
	"3do":        {"opera"},
	"atari2600":  {"stella", "stella2014"},
	"atari7800":  {"prosystem"},
	"lynx":       {"handy", "mednafen_lynx"},
	"jaguar":     {"virtualjaguar"},
	"msx":        {"bluemsx", "fmsx"},
	"amiga":      {"puae"},
	"c64":        {"vice_x64sc", "vice_x64"},
	"zxspectrum": {"fuse"},
	"atarist":    {"hatari"},
	"amstradcpc": {"cap32", "crocods"},
}

func coreExt() string {
	switch runtime.GOOS {
	case "windows":
		return ".dll"
	case "darwin":
		return ".dylib"
	}
	return ".so"
}

// SuggestedCore returns the preferred installed core for a system in this
// RetroArch, if any: the first of the system's suggested cores that is in the
// cores folder, with the display name its .info file gives (the core's file
// name when there is none).
func (in RetroArchInstall) SuggestedCore(systemID string) (Core, bool) {
	for _, name := range suggestedCores[systemID] {
		path := filepath.Join(in.Cores, name+"_libretro"+coreExt())
		if st, err := os.Stat(path); err != nil || st.IsDir() {
			continue
		}
		display := infoDisplayName(filepath.Join(in.Info, name+"_libretro.info"))
		if display == "" {
			display = name
		}
		return Core{Path: path, Name: display}, true
	}
	return Core{}, false
}

// infoDisplayName reads display_name from a core's .info file ("" if absent).
func infoDisplayName(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		if k, v, ok := strings.Cut(sc.Text(), "="); ok && strings.TrimSpace(k) == "display_name" {
			return strings.Trim(strings.TrimSpace(v), `"`)
		}
	}
	return ""
}
