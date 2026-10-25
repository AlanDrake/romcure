package discover

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakeRetroArch makes a RetroArch folder with the given cores; info maps a core
// to its .info text ("" = no .info file).
func fakeRetroArch(t *testing.T, info map[string]string) RetroArchInstall {
	t.Helper()
	root := t.TempDir()
	in := RetroArchInstall{Dir: root, Cores: filepath.Join(root, "cores"), Info: filepath.Join(root, "info"),
		System: filepath.Join(root, "system"), Playlists: filepath.Join(root, "playlists")}
	for _, d := range []string{in.Cores, in.Info, in.System, in.Playlists} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for name, text := range info {
		touch(t, filepath.Join(in.Cores, name+"_libretro"+coreExt()))
		if text != "" {
			if err := os.WriteFile(filepath.Join(in.Info, name+"_libretro.info"), []byte(text), 0o644); err != nil {
				t.Fatal(err)
			}
		}
	}
	return in
}

func info(display, exts, db string, extra ...string) string {
	return "display_name = \"" + display + "\"\nsupported_extensions = \"" + exts + "\"\ndatabase = \"" + db + "\"\n" + strings.Join(extra, "\n") + "\n"
}

const (
	gbaDB = "Nintendo - Game Boy Advance"
	psxDB = "Sony - PlayStation"
)

func choose(t *testing.T, in RetroArchInstall, sys, db string, exts ...string) Core {
	t.Helper()
	c, ok := in.NewCoreChooser().Choose(sys, db, exts)
	if !ok {
		t.Fatalf("no core chosen for %s", sys)
	}
	return c
}

func TestTheRecommendedCoreWinsAmongSeveral(t *testing.T) {
	in := fakeRetroArch(t, map[string]string{
		"gpsp": info("Nintendo - Game Boy Advance (gpSP)", "gba|bin", gbaDB),
		"mgba": info("Nintendo - Game Boy Advance (mGBA)", "gba|gb|gbc", gbaDB),
	})
	c := choose(t, in, "gba", gbaDB, "gba")
	if c.Name != "Nintendo - Game Boy Advance (mGBA)" || c.Reason != "the recommended core" {
		t.Errorf("got %+v", c)
	}
	if filepath.Base(c.Path) != "mgba_libretro"+coreExt() {
		t.Errorf("path %s", c.Path)
	}
}

func TestTheOnlyInstalledCoreIsUsedEvenIfNotRecommendedFirst(t *testing.T) {
	in := fakeRetroArch(t, map[string]string{"gpsp": ""}) // no .info: name from the file
	if c := choose(t, in, "gba", gbaDB, "gba"); c.Name != "gpsp" || c.Reason != "the only installed core for these games" {
		t.Errorf("got %+v", c)
	}
}

func TestACoreThatCantOpenTheFilesLosesAndIsNamedInTheReason(t *testing.T) {
	in := fakeRetroArch(t, map[string]string{
		"mednafen_psx": info("Sony - PlayStation (Beetle PSX)", "cue|toc|m3u|ccd|exe|pbp", psxDB),
		"pcsx_rearmed": info("Sony - PlayStation (PCSX ReARMed)", "bin|cue|img|mdf|pbp|toc|cbn|m3u|chd|iso|exe", psxDB),
	})
	c := choose(t, in, "psx", psxDB, "chd", "m3u")
	if c.Name != "Sony - PlayStation (PCSX ReARMed)" || !strings.Contains(c.Reason, "Beetle PSX) can't open these files") {
		t.Errorf("got %+v", c)
	}
	// Zipped games open with any core: RetroArch unpacks them itself.
	if c := choose(t, in, "psx", psxDB, "zip", "cue"); c.Name != "Sony - PlayStation (Beetle PSX)" {
		t.Errorf("zip: got %+v", c)
	}
	// When nothing can open them, no default is set.
	only := fakeRetroArch(t, map[string]string{"mednafen_psx": info("Beetle PSX", "cue", psxDB)})
	if _, ok := only.NewCoreChooser().Choose("psx", psxDB, []string{"chd"}); ok {
		t.Error("a core that can't open the files was chosen")
	}
}

func TestACoreMissingItsBIOSLosesUntilTheBIOSIsThere(t *testing.T) {
	bios := []string{"firmware_count = 1", `firmware0_path = "scph5501.bin"`, `firmware0_opt = "false"`}
	in := fakeRetroArch(t, map[string]string{
		"mednafen_psx": info("Sony - PlayStation (Beetle PSX)", "cue|chd", psxDB, bios...),
		"pcsx_rearmed": info("Sony - PlayStation (PCSX ReARMed)", "cue|chd", psxDB),
	})
	c := choose(t, in, "psx", psxDB, "chd")
	if c.Name != "Sony - PlayStation (PCSX ReARMed)" || !strings.Contains(c.Reason, "needs scph5501.bin") {
		t.Errorf("without BIOS: %+v", c)
	}
	touch(t, filepath.Join(in.System, "scph5501.bin"))
	if c := choose(t, in, "psx", psxDB, "chd"); c.Name != "Sony - PlayStation (Beetle PSX)" {
		t.Errorf("with BIOS: %+v", c)
	}
	// An optional BIOS doesn't count.
	opt := fakeRetroArch(t, map[string]string{
		"mednafen_psx": info("Beetle PSX", "chd", psxDB, "firmware_count = 1", `firmware0_path = "x.bin"`, `firmware0_opt = "true"`),
		"pcsx_rearmed": info("PCSX ReARMed", "chd", psxDB),
	})
	if c := choose(t, opt, "psx", psxDB, "chd"); c.Name != "Beetle PSX" {
		t.Errorf("optional BIOS: %+v", c)
	}
}

func TestTheCoreTheUserAlreadyPlaysWithWins(t *testing.T) {
	in := fakeRetroArch(t, map[string]string{
		"mednafen_psx": info("Sony - PlayStation (Beetle PSX)", "chd", psxDB),
		"swanstation":  info("Sony - PlayStation (SwanStation)", "chd", psxDB),
	})
	// The user's own playlist runs two PlayStation games with SwanStation.
	pl := `{"items": [
	  {"path": "/a.chd", "core_path": "/ra/cores/swanstation_libretro` + coreExt() + `", "db_name": "Sony - PlayStation.lpl"},
	  {"path": "/b.chd", "core_path": "/ra/cores/swanstation_libretro` + coreExt() + `", "db_name": "Sony - PlayStation.lpl"},
	  {"path": "/c.chd", "core_path": "DETECT", "db_name": "Sony - PlayStation.lpl"},
	  {"path": "/d.gba", "core_path": "/ra/cores/mednafen_psx_libretro` + coreExt() + `", "db_name": "Nintendo - Game Boy Advance.lpl"}
	]}`
	if err := os.WriteFile(filepath.Join(in.Playlists, "content_history.lpl"), []byte(pl), 0o644); err != nil {
		t.Fatal(err)
	}
	if c := choose(t, in, "psx", psxDB, "chd"); c.Name != "Sony - PlayStation (SwanStation)" || c.Reason != "the core you already play these games with" {
		t.Errorf("got %+v", c)
	}
}

func TestAnUnlistedCoreForTheSystemCountsButAnExperimentalOneDoesNot(t *testing.T) {
	in := fakeRetroArch(t, map[string]string{
		"newgba": info("Nintendo - Game Boy Advance (NewGBA)", "gba", gbaDB),
		"labgba": info("Nintendo - Game Boy Advance (Lab)", "gba", gbaDB, `is_experimental = "true"`),
		"snes9x": info("Nintendo - SNES (Snes9x)", "sfc", "Nintendo - Super Nintendo Entertainment System"),
	})
	if c := choose(t, in, "gba", gbaDB, "gba"); c.Name != "Nintendo - Game Boy Advance (NewGBA)" || c.Reason != "the only installed core for these games" {
		t.Errorf("got %+v", c)
	}
	// A listed core still beats it.
	touch(t, filepath.Join(in.Cores, "mgba_libretro"+coreExt()))
	if c := choose(t, in, "gba", gbaDB, "gba"); c.Name != "mgba" {
		t.Errorf("with mgba: %+v", c)
	}
}

func TestParseRetroArchConfigReadsTheInfoAndSystemFolders(t *testing.T) {
	root := t.TempDir()
	in, err := ParseRetroArchConfig(writeCfg(t, root, "libretro_info_path = \":\\info\"\nsystem_directory = \"\"\n"))
	if err != nil || in.Info != filepath.Join(root, "info") || in.System != filepath.Join(root, "system") {
		t.Fatalf("%+v %v", in, err)
	}
}
