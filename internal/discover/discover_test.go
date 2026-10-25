package discover

import (
	"os"
	"path/filepath"
	"testing"
)

func touch(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestSystemFromName(t *testing.T) {
	for name, want := range map[string]string{
		"SNES": "snes", "Super Nintendo": "snes", "Nintendo - NES": "nes", "Nintendo - GBA": "gba",
		"Sega Genesis (Mega Drive) games": "genesis", "PSX": "psx", "PlayStation 2": "ps2", "PS2": "ps2",
		"Nintedo GameCube": "gc", "TurboGrafx-16 roms": "pce", "Atari 2600": "atari2600", "ZX Spectrum": "zxspectrum",
		"Amiga 500": "amiga", "Nintendo 64": "n64", "MSX": "msx",
		"Nintendo Switch": "", "Xbox 360": "", "My stuff": "", "Old downloads": "", "NeoGeo AES": "",
	} {
		if got := SystemFromName(name); got != want {
			t.Errorf("SystemFromName(%q) = %q, want %q", name, got, want)
		}
	}
}

func TestFoldersProposesSystemsByNameThenByFileTypes(t *testing.T) {
	root := t.TempDir()
	touch(t, filepath.Join(root, "Super Nintendo", "Mario (USA).zip"))
	touch(t, filepath.Join(root, "Super Nintendo", "readme.txt"))
	touch(t, filepath.Join(root, "stuff", "a.gba"))
	touch(t, filepath.Join(root, "stuff", "b.gba"))
	touch(t, filepath.Join(root, "mixed", "x.gba"))
	touch(t, filepath.Join(root, "mixed", "y.nes"))
	touch(t, filepath.Join(root, "empty", "notes.txt"))
	touch(t, filepath.Join(root, "Collection", "Nested", "PSX", "disc.chd"))

	got, err := Folders(root)
	if err != nil {
		t.Fatal(err)
	}
	by := map[string]Candidate{}
	for _, c := range got {
		by[c.Name] = c
	}
	if c := by["Super Nintendo"]; c.SystemID != "snes" || c.Reason != "folder name" || c.Games != 1 {
		t.Errorf("Super Nintendo: %+v (the .txt is not a game)", c)
	}
	if c := by["stuff"]; c.SystemID != "gba" || c.Reason != "file types" {
		t.Errorf("stuff: %+v", c)
	}
	if c, ok := by["mixed"]; !ok || c.SystemID != "" {
		t.Errorf("mixed types must be listed unplaced, got %+v ok=%v", c, ok)
	}
	if _, ok := by["empty"]; ok {
		t.Error("a folder without games must not be listed")
	}
	if c := by["PSX"]; c.SystemID != "psx" {
		t.Errorf("nested PSX folder not found: %+v", by)
	}
	if _, ok := by["Collection"]; ok {
		t.Error("the unplaced wrapper must not be listed when its subfolders were placed")
	}
}

func TestFoldersAcceptsAFolderThatHoldsGamesDirectly(t *testing.T) {
	root := t.TempDir()
	touch(t, filepath.Join(root, "a.nes"))
	touch(t, filepath.Join(root, "b.nes"))
	got, err := Folders(root)
	if err != nil || len(got) != 1 || got[0].SystemID != "nes" || got[0].Games != 2 {
		t.Fatalf("got %+v err %v", got, err)
	}
}

func TestFoldersRejectsAFile(t *testing.T) {
	f := filepath.Join(t.TempDir(), "f")
	touch(t, f)
	if _, err := Folders(f); err == nil {
		t.Error("a file is not a folder")
	}
}

func TestAFewRecognizableFilesDoNotDecideAFolderOfOtherThings(t *testing.T) {
	root := t.TempDir()
	touch(t, filepath.Join(root, "Misc", "a.adf"))
	touch(t, filepath.Join(root, "Misc", "b.adf"))
	for _, n := range []string{"1.com", "2.dat1", "3.img", "4.bin", "5.iso", "6.dsk"} {
		touch(t, filepath.Join(root, "Misc", n))
	}
	got, _ := Folders(root)
	if len(got) != 1 || got[0].SystemID != "" {
		t.Errorf("got %+v, want one unplaced folder", got)
	}
}
