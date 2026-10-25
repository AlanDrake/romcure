package profile

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// The ES-DE folder map must name systems that ES-DE actually defines. This
// reads a real install's resources/systems/<os>/es_systems.xml when
// ROMCURE_ESDE_SYSTEMS points at one (skips otherwise).
func TestESDEFolderNamesExistInRealInstall(t *testing.T) {
	path := os.Getenv("ROMCURE_ESDE_SYSTEMS")
	if path == "" {
		t.Skip("ROMCURE_ESDE_SYSTEMS not set")
	}
	xml, err := os.ReadFile(path)
	if err != nil {
		t.Skip("no ES-DE install")
	}
	known := map[string]bool{}
	for _, m := range regexp.MustCompile(`<name>([^<]+)</name>`).FindAllStringSubmatch(string(xml), -1) {
		known[m[1]] = true
	}
	for sys, folder := range esdeFolders {
		if !known[folder] {
			t.Errorf("system %s → %q is not an ES-DE system name", sys, folder)
		}
	}
}

func TestGenerateESFamilyFolderNamesDifferPerFrontend(t *testing.T) {
	root := t.TempDir()
	writeZip(t, filepath.Join(root, "Game (USA).zip"), map[string]string{"Game (USA).gba": "rom"})
	db := openTestDB(t)
	syncSystem(t, db, root, "genesis") // folder name differs: genesis vs megadrive

	for fe, want := range map[Frontend]string{FrontendESDE: "genesis", FrontendBatocera: "megadrive", FrontendRetroDECK: "genesis", FrontendRetroBat: "megadrive"} {
		out := t.TempDir()
		report, err := GenerateESFamily(db, "genesis", fe, Options{OutputRoot: out})
		if err != nil || len(report.Errors) != 0 || len(report.Linked) != 1 {
			t.Fatalf("%s: err=%v report=%+v", fe, err, report)
		}
		if filepath.Base(filepath.Dir(report.Linked[0].Path)) != want {
			t.Errorf("%s linked into %s, want folder %s", fe, report.Linked[0].Path, want)
		}
		if m, _ := filepath.Glob(filepath.Join(out, "*.lpl")); len(m) != 0 {
			t.Errorf("%s: ES-family profiles must not write .lpl playlists", fe)
		}
	}
}

func TestGenerateESFamilyMultiDiscAndUnknownSystem(t *testing.T) {
	root := t.TempDir()
	for _, n := range []string{"1", "2"} {
		writeFile(t, filepath.Join(root, "Epic RPG (Disc "+n+").bin"), "bin"+n)
		writeFile(t, filepath.Join(root, "Epic RPG (Disc "+n+").cue"), "FILE \"Epic RPG (Disc "+n+").bin\" BINARY\n  TRACK 01 MODE2/2352\n    INDEX 01 00:00:00\n")
	}
	db := openTestDB(t)
	syncSystem(t, db, root, "psx")
	out := t.TempDir()
	report, err := GenerateESFamily(db, "psx", FrontendESDE, Options{OutputRoot: out})
	if err != nil || len(report.Errors) != 0 {
		t.Fatalf("err=%v report=%+v", err, report)
	}
	m3u, _ := filepath.Glob(filepath.Join(out, "psx", "*.m3u"))
	if len(m3u) != 1 {
		t.Fatalf("m3u files = %v", m3u)
	}
	if b, _ := os.ReadFile(m3u[0]); !strings.Contains(string(b), "Epic RPG (Disc 2).cue") {
		t.Errorf("m3u = %q", b)
	}

	// An unmapped system falls back to its id and says so.
	db2 := openTestDB(t)
	r2 := t.TempDir()
	writeFile(t, filepath.Join(r2, "Thing (USA).rom"), "x")
	syncSystem(t, db2, r2, "weirdsys")
	rep, _ := GenerateESFamily(db2, "weirdsys", FrontendBatocera, Options{OutputRoot: t.TempDir()})
	if len(rep.Notes) != 1 || !strings.Contains(rep.Notes[0], "weirdsys") {
		t.Errorf("notes = %v", rep.Notes)
	}
	if _, err := GenerateESFamily(db2, "psx", Frontend("nope"), Options{}); err == nil {
		t.Error("unknown frontend should error")
	}
}

func TestGenerateESFamilyIsIdempotentAndUsesOwnProfileID(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "Game (USA).nes"), "rom")
	db := openTestDB(t)
	syncSystem(t, db, root, "nes")
	out := t.TempDir()
	for i := 0; i < 2; i++ {
		if _, err := GenerateESFamily(db, "nes", FrontendESDE, Options{OutputRoot: out}); err != nil {
			t.Fatal(err)
		}
	}
	var n int
	db.QueryRow(`SELECT COUNT(*) FROM link_state WHERE profile_id = 'esde'`).Scan(&n)
	if n != 1 {
		t.Errorf("esde link_state rows = %d, want 1", n)
	}
}
