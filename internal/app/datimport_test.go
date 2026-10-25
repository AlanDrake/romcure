package app

import (
	"archive/zip"
	"context"
	"fmt"
	"hash/crc32"
	"os"
	"path/filepath"
	"testing"

	"romcure/internal/store"
)

func noIntroDAT(name string, games map[string][]byte) string {
	s := `<?xml version="1.0"?>` + "\n<datafile><header><name>" + name + "</name></header>\n"
	for g, data := range games {
		s += fmt.Sprintf(`<game name=%q><rom name=%q size="%d" crc="%08x"/></game>`+"\n", g, g+".nes", len(data), crc32.ChecksumIEEE(data))
	}
	return s + "</datafile>\n"
}

func TestSystemOfDAT(t *testing.T) {
	for name, want := range map[string]string{
		"Nintendo - Nintendo Entertainment System (Headered) (Parent-Clone)": "nes",
		"Nintendo - Game Boy Color (Parent-Clone)":                           "gbc",
		"Nintendo - Game Boy":                                                "gb",
		"Sega - Mega Drive - Genesis (Parent-Clone)":                         "genesis",
		"Sony - PlayStation 2":                                               "ps2",
		"Sony - PlayStation":                                                 "psx",
	} {
		if got, ok := SystemOfDAT(name); !ok || got != want {
			t.Errorf("%q -> %q %v, want %q", name, got, ok, want)
		}
	}
	if got, ok := SystemOfDAT("My Homebrew Collection"); ok {
		t.Errorf("unknown DAT -> %q", got)
	}
}

// A No-Intro DAT imported by hand (zipped, as No-Intro hands it out) verifies
// the cartridge games at the next sync, and a newer import replaces it.
func TestImportDATByHandThenSyncVerifies(t *testing.T) {
	a, src := selectionApp(t) // Alpha (USA), Alpha (Europe), Beta (USA), Gamma (USA)
	addOutput(t, a, store.Profile{Name: "Plain"})
	dir := t.TempDir()
	zpath := filepath.Join(dir, "No-Intro NES.zip")
	zf, _ := os.Create(zpath)
	zw := zip.NewWriter(zf)
	w, _ := zw.Create("Nintendo - Nintendo Entertainment System (Headered) (20261001-000000).dat")
	w.Write([]byte(noIntroDAT("Nintendo - Nintendo Entertainment System (Headered)", map[string][]byte{"Beta (USA)": []byte("beta")})))
	zw.Close()
	zf.Close()

	res, err := a.ImportDAT(zpath, "")
	if err != nil {
		t.Fatal(err)
	}
	if res.SystemID != "nes" || res.Games != 1 {
		t.Errorf("result = %+v", res)
	}
	if _, err := a.Sync(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
	var conf string
	a.DB.QueryRow(`SELECT confidence FROM games WHERE ultimate_source_ref = ?`, filepath.Join(src, "Beta (USA).nes")).Scan(&conf)
	if conf != "dat_verified" {
		t.Errorf("Beta after the import: %q", conf)
	}

	// A second import for the same system replaces the first.
	plain := filepath.Join(dir, "newer.dat")
	os.WriteFile(plain, []byte(noIntroDAT("Nintendo - Nintendo Entertainment System", map[string][]byte{"Gamma (USA)": []byte("gamma")})), 0o644)
	if _, err := a.ImportDAT(plain, "nes"); err != nil {
		t.Fatal(err)
	}
	var n int
	a.DB.QueryRow(`SELECT COUNT(*) FROM dats WHERE system_id = 'nes'`).Scan(&n)
	if n != 1 {
		t.Errorf("%d DATs for nes, want the newer one only", n)
	}
}

func TestImportDATRefusesWhatItCantUse(t *testing.T) {
	a := newApp(t)
	dir := t.TempDir()
	unknown := filepath.Join(dir, "x.dat")
	os.WriteFile(unknown, []byte(noIntroDAT("Some Homebrew", map[string][]byte{"A": []byte("a")})), 0o644)
	if _, err := a.ImportDAT(unknown, ""); err == nil {
		t.Error("an unknown system must be asked for")
	}
	if _, err := a.ImportDAT(unknown, "nes"); err != nil {
		t.Errorf("with the system chosen it imports: %v", err)
	}
	notDAT := filepath.Join(dir, "readme.txt")
	os.WriteFile(notDAT, []byte("hello"), 0o644)
	if _, err := a.ImportDAT(notDAT, "nes"); err == nil {
		t.Error("a file that is not a DAT must be refused")
	}
}

// Regression: a plain (non parent/clone) DAT must not turn every verified
// version into a game of its own; 1G1R still keeps one Alpha.
func TestAPlainDATStillGivesOneVersionPerGame(t *testing.T) {
	a, _ := selectionApp(t) // Alpha (USA) "alpha", Alpha (Europe) "alpha-eu", Beta, Gamma
	_, out := addOutput(t, a, store.Profile{Name: "Plain"})
	path := filepath.Join(t.TempDir(), "nes.dat")
	os.WriteFile(path, []byte(noIntroDAT("Nintendo - Nintendo Entertainment System",
		map[string][]byte{"Alpha (USA)": []byte("alpha"), "Alpha (Europe)": []byte("alpha-eu")})), 0o644)
	if _, err := a.ImportDAT(path, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Sync(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
	// Verified games are named after the DAT entry; there must be one Alpha.
	if got := outputs(t, out); !equal(got, []string{"Alpha (USA).nes", "Beta.nes", "Gamma.nes"}) {
		t.Errorf("output = %v, want one Alpha", got)
	}
}
