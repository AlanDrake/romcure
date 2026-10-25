package app

import (
	"context"
	"fmt"
	"hash/crc32"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
)

// libretroDAT writes a DAT the way libretro-database does: ClrMamePro text,
// each game repeated once per file variant (here a .nes and a .unh).
func libretroDAT(games map[string][]byte) string {
	s := "clrmamepro (\n\tname \"Nintendo - Nintendo Entertainment System\"\n\tversion \"2026.10.07\"\n)\n\n"
	for g, data := range games {
		for _, ext := range []string{".nes", ".unh"} {
			s += fmt.Sprintf("game (\n\tname %q\n\trom ( name %q size %d crc %08X )\n)\n", g, g+ext, len(data), crc32.ChecksumIEEE(data))
		}
	}
	return s
}

// "Update DATs" downloads a cartridge system's No-Intro DAT from libretro's
// copy, skips it while unchanged, and leaves a DAT imported by hand alone.
func TestUpdateDATsNoIntroFromLibretro(t *testing.T) {
	a, src := selectionApp(t) // Alpha (USA), Alpha (Europe), Beta (USA), Gamma (USA)
	var (
		sha       atomic.Value
		downloads atomic.Int32
		srv       *httptest.Server
	)
	sha.Store("aaa")
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/index":
			fmt.Fprintf(w, `[{"name":"Nintendo - Nintendo Entertainment System.dat","sha":%q,"size":1,"download_url":%q},
				{"name":"Atari - Lynx.dat","sha":"x","size":1,"download_url":%q}]`,
				sha.Load(), srv.URL+"/raw/nes.dat", srv.URL+"/raw/lynx.dat")
		case "/raw/nes.dat":
			downloads.Add(1)
			w.Write([]byte(libretroDAT(map[string][]byte{"Beta (USA)": []byte("beta")})))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	ep := Endpoints{Client: srv.Client(), NoIntroURL: srv.URL + "/index"}
	update := func() DATResult {
		t.Helper()
		res, err := a.UpdateDATs(context.Background(), ep, nil)
		if err != nil || len(res) != 1 {
			t.Fatalf("results=%+v err=%v", res, err)
		}
		return res[0]
	}

	if r := update(); r.Source != "no-intro" || !r.Updated || r.Error != "" {
		t.Errorf("first update = %+v", r)
	}
	doSync(t, a)
	var conf string
	a.DB.QueryRow(`SELECT confidence FROM games WHERE ultimate_source_ref = ?`, filepath.Join(src, "Beta (USA).nes")).Scan(&conf)
	if conf != "dat_verified" {
		t.Errorf("Beta after the update: %q", conf)
	}

	// Unchanged: no download.
	if r := update(); r.Updated || r.Error != "" || downloads.Load() != 1 {
		t.Errorf("unchanged update = %+v, %d downloads", r, downloads.Load())
	}
	// Changed upstream: downloaded again, replacing the old import.
	sha.Store("bbb")
	if r := update(); !r.Updated || downloads.Load() != 2 {
		t.Errorf("changed update = %+v, %d downloads", r, downloads.Load())
	}
	countDATs := func() int {
		var n int
		a.DB.QueryRow(`SELECT COUNT(*) FROM dats WHERE system_id = 'nes'`).Scan(&n)
		return n
	}
	if n := countDATs(); n != 1 {
		t.Errorf("%d DATs for nes after the second download, want 1", n)
	}

	// A DAT imported by hand replaces the downloaded one and is kept.
	plain := filepath.Join(t.TempDir(), "newer.dat")
	os.WriteFile(plain, []byte(noIntroDAT("Nintendo - Nintendo Entertainment System", map[string][]byte{"Gamma (USA)": []byte("gamma")})), 0o644)
	if _, err := a.ImportDAT(plain, "nes"); err != nil {
		t.Fatal(err)
	}
	if n := countDATs(); n != 1 {
		t.Errorf("%d DATs for nes after the import by hand, want 1", n)
	}
	sha.Store("ccc")
	if r := update(); r.Source != "manual" || r.Updated || downloads.Load() != 2 {
		t.Errorf("update after an import by hand = %+v, %d downloads", r, downloads.Load())
	}
}

// A system libretro has no No-Intro DAT for says so instead of failing.
func TestUpdateDATsNoIntroMissingSystem(t *testing.T) {
	a := newApp(t)
	src := filepath.Join(t.TempDir(), "jaguar")
	os.MkdirAll(src, 0o755)
	a.AddSource(src, "jaguar")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("[]")) }))
	defer srv.Close()
	res, err := a.UpdateDATs(context.Background(), Endpoints{Client: srv.Client(), NoIntroURL: srv.URL}, nil)
	if err != nil || len(res) != 1 || res[0].Source != "" || res[0].Error != "" || res[0].Message == "" {
		t.Errorf("results=%+v err=%v", res, err)
	}
}
