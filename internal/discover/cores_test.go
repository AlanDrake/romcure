package discover

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSuggestedCorePrefersTheFirstInstalledAndNamesItFromItsInfo(t *testing.T) {
	root := t.TempDir()
	cores, info := filepath.Join(root, "cores"), filepath.Join(root, "info")
	for _, f := range []string{
		filepath.Join(cores, "gpsp_libretro"+coreExt()), // second choice for GBA
		filepath.Join(cores, "vba_next_libretro"+coreExt()),
	} {
		touch(t, f)
	}
	in := RetroArchInstall{Cores: cores, Info: info}

	c, ok := in.SuggestedCore("gba")
	if !ok || c.Path != filepath.Join(cores, "gpsp_libretro"+coreExt()) || c.Name != "gpsp" {
		t.Fatalf("without mgba or an info file: %+v %v", c, ok)
	}

	touch(t, filepath.Join(cores, "mgba_libretro"+coreExt()))
	if err := os.MkdirAll(info, 0o755); err != nil {
		t.Fatal(err)
	}
	infoText := "authors = \"x\"\ndisplay_name = \"Nintendo - Game Boy Advance (mGBA)\"\n"
	if err := os.WriteFile(filepath.Join(info, "mgba_libretro.info"), []byte(infoText), 0o644); err != nil {
		t.Fatal(err)
	}
	c, ok = in.SuggestedCore("gba")
	if !ok || c.Name != "Nintendo - Game Boy Advance (mGBA)" || filepath.Base(c.Path) != "mgba_libretro"+coreExt() {
		t.Fatalf("with mgba: %+v %v", c, ok)
	}

	if _, ok := in.SuggestedCore("snes"); ok {
		t.Error("no SNES core is installed")
	}
	if _, ok := in.SuggestedCore("nosuchsystem"); ok {
		t.Error("unknown system")
	}
}

func TestParseRetroArchConfigReadsTheInfoFolder(t *testing.T) {
	root := t.TempDir()
	in, err := ParseRetroArchConfig(writeCfg(t, root, "libretro_info_path = \":\\info\"\n"))
	if err != nil || in.Info != filepath.Join(root, "info") {
		t.Fatalf("%+v %v", in, err)
	}
}
