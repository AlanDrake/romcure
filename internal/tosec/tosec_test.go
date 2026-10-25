package tosec

import (
	"archive/zip"
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"romcure/internal/store"
)

func TestParseDatName(t *testing.T) {
	n, ok := ParseDatName(`TOSEC\Commodore Amiga - Games - [ADF] (TOSEC-v2025-01-30_CM).dat`)
	if !ok || n.Product != "Commodore Amiga" || n.Category != "Games" || n.Format != "[ADF]" || n.Version != "2025-01-30" {
		t.Errorf("name = %+v ok=%v", n, ok)
	}
	n, ok = ParseDatName("Atari ST - Collections - 4U (TOSEC-v2013-12-13_CM).dat")
	if !ok || n.Product != "Atari ST" || n.Category != "Collections" || n.Format != "" {
		t.Errorf("name = %+v", n)
	}
	if _, ok := ParseDatName("README.dat"); ok {
		t.Error("a name without a TOSEC version is not a TOSEC DAT")
	}
	if id, ok := SystemID("Commodore C64"); !ok || id != "c64" {
		t.Error("c64 mapping")
	}
	if _, ok := SystemID("Zilog MCZ-2"); ok {
		t.Error("unmapped product must not map")
	}
}

func TestParseLatestAndPackLink(t *testing.T) {
	index := `<a href="/downloads/category/58-2024-05-17">a</a><a href="/downloads/category/59-2025-03-13">b</a><a href="/downloads/category/57-2023-07-10">c</a>`
	id, date, p, ok := ParseLatest(index)
	if !ok || id != 59 || date != "2025-03-13" || p != "/downloads/category/59-2025-03-13" {
		t.Errorf("latest = %d %s %s %v", id, date, p, ok)
	}
	page := `<a href="/downloads/category/59-2025-03-13?download=117:tosec-dat-pack-complete-4743-tosec-v2025-03-13">x</a><a href="/other?download=9:foo">y</a>`
	link, ok := ParsePackLink(page)
	if !ok || !strings.Contains(link, "download=117") {
		t.Errorf("link = %q ok=%v", link, ok)
	}
	if _, _, _, ok := ParseLatest("<html></html>"); ok {
		t.Error("no releases must not parse")
	}
}

func datXML(games ...string) string {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0"?><datafile><header><name>x</name></header>`)
	for _, g := range games {
		b.WriteString(`<game name="` + g + `"><description>` + g + `</description><rom name="` + g + `.adf" size="901120" crc="aaaaaaaa"/></game>`)
	}
	return b.String() + `</datafile>`
}

func buildPack(t *testing.T, files map[string]string) []byte {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for n, c := range files {
		w, _ := zw.Create(n)
		w.Write([]byte(c))
	}
	zw.Close()
	return buf.Bytes()
}

func site(t *testing.T, pack []byte, hits *atomic.Int32) *httptest.Server {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/downloads/category/22-datfiles":
			w.Write([]byte(`<a href="/downloads/category/59-2025-03-13">x</a>`))
		case r.URL.Path == "/downloads/category/59-2025-03-13" && r.URL.Query().Get("download") != "":
			hits.Add(1)
			w.Write(pack)
		case r.URL.Path == "/downloads/category/59-2025-03-13":
			w.Write([]byte(`<a href="/downloads/category/59-2025-03-13?download=117:tosec-dat-pack-complete-1">dl</a>`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestSyncSystemSelectsDatsImportsAndSkips(t *testing.T) {
	pack := buildPack(t, map[string]string{
		`TOSEC/Commodore Amiga - Games - [ADF] (TOSEC-v2025-01-30_CM).dat`: datXML("Game A (1990)(Pub)(EU)(en)", "Game B (1991)(Pub)"),
		`TOSEC/Commodore Amiga - Demos - [ADF] (TOSEC-v2020-01-01_CM).dat`: datXML("Demo A (1992)(Grp)"),
		`ISO/Commodore Amiga - Games - [ISO] (TOSEC-v2021-01-01_CM).dat`:   datXML("CD Game (1995)(Pub)"),
		`TOSEC/Commodore C64 - Games - [D64] (TOSEC-v2025-01-30_CM).dat`:   datXML("C64 Game (1986)(Pub)"),
		`PIX/Commodore Amiga - Games - [PIX] (TOSEC-v2025-01-30_CM).dat`:   datXML("Scan (1990)"),
		`TOSEC/Zilog MCZ-2 - Utilities (TOSEC-v2017-04-05_CM).dat`:         datXML("Unmapped (1980)"),
	})
	var hits atomic.Int32
	srv := site(t, pack, &hits)
	db, err := store.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	s := &Syncer{DB: db, Client: srv.Client(), Dir: t.TempDir()}

	rel, err := FetchLatest(context.Background(), srv.Client(), srv.URL)
	if err != nil || rel.Date != "2025-03-13" || !strings.Contains(rel.PackURL, "download=117") {
		t.Fatalf("release = %+v err=%v", rel, err)
	}

	// All Amiga DATs from TOSEC/ and ISO/, not PIX, not other systems.
	res, err := s.SyncSystem(context.Background(), rel, "amiga", nil)
	if err != nil || !res.Updated || res.Dats != 3 || res.Games != 4 {
		t.Fatalf("all: %+v err=%v", res, err)
	}
	var n int
	db.QueryRow(`SELECT COUNT(*) FROM dat_entries WHERE system_id = 'amiga'`).Scan(&n)
	if n != 4 {
		t.Errorf("amiga entries = %d, want 4", n)
	}
	db.QueryRow(`SELECT COUNT(*) FROM dat_entries WHERE system_id = 'c64'`).Scan(&n)
	if n != 0 {
		t.Error("c64 must not be imported when syncing amiga")
	}

	// Same release + selection: no work, no network.
	before := hits.Load()
	res, err = s.SyncSystem(context.Background(), rel, "amiga", nil)
	if err != nil || res.Updated || hits.Load() != before {
		t.Fatalf("unchanged: %+v hits %d->%d err=%v", res, before, hits.Load(), err)
	}

	// A different category selection re-imports and prunes the old import,
	// reusing the already-downloaded pack.
	res, err = s.SyncSystem(context.Background(), rel, "amiga", []string{"Games"})
	if err != nil || !res.Updated || res.Dats != 2 || hits.Load() != before {
		t.Fatalf("games only: %+v hits=%d err=%v", res, hits.Load(), err)
	}
	db.QueryRow(`SELECT COUNT(*) FROM dats WHERE remote_source = 'tosec' AND remote_slug = 'amiga'`).Scan(&n)
	if n != 2 {
		t.Errorf("dats rows = %d, want 2 (old import pruned)", n)
	}

	// Syncing a second system from the same release does not re-download.
	if res, err := s.SyncSystem(context.Background(), rel, "c64", nil); err != nil || res.Dats != 1 || hits.Load() != before {
		t.Fatalf("c64: %+v hits=%d err=%v", res, hits.Load(), err)
	}
}

func TestSyncSystemNoMatchingDats(t *testing.T) {
	var hits atomic.Int32
	srv := site(t, buildPack(t, map[string]string{`TOSEC/Commodore C64 - Games (TOSEC-v2025-01-30_CM).dat`: datXML("G (1986)")}), &hits)
	db, _ := store.Open(filepath.Join(t.TempDir(), "t.db"))
	defer db.Close()
	s := &Syncer{DB: db, Client: srv.Client(), Dir: t.TempDir()}
	rel, _ := FetchLatest(context.Background(), srv.Client(), srv.URL)
	res, err := s.SyncSystem(context.Background(), rel, "amiga", nil)
	if err != nil || res.Updated || res.Dats != 0 || res.Reason == "" {
		t.Fatalf("%+v err=%v", res, err)
	}
}
