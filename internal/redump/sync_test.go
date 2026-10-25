package redump

import (
	"archive/zip"
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"romcure/internal/store"
)

func makeZip(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, content := range files {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		w.Write([]byte(content))
	}
	zw.Close()
	return buf.Bytes()
}

func datXML(games ...string) string {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0"?><datafile><header><name>Sony - PlayStation</name></header>`)
	for _, g := range games {
		b.WriteString(`<game name="` + g + `"><description>` + g + `</description>` +
			`<rom name="` + g + `.bin" size="4" crc="deadbeef" sha1="da39a3ee5e6b4b0d3255bfef95601890afd80709"/></game>`)
	}
	b.WriteString(`</datafile>`)
	return b.String()
}

type fakeSite struct {
	dat, sbi []byte
	hits     atomic.Int32
}

func (f *fakeSite) start(t *testing.T) *Downloader {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.hits.Add(1)
		switch r.URL.Path {
		case "/datfile/PSX":
			w.Write(f.dat)
		case "/sbi/PSX":
			w.Write(f.sbi)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return &Downloader{Client: srv.Client(), BaseURL: srv.URL, Dir: t.TempDir()}
}

func openDB(t *testing.T) *store.DB {
	db, err := store.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func entryCount(t *testing.T, db *store.DB) int {
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM dat_entries`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestSyncSystemImportsSkipsAndUpdates(t *testing.T) {
	site := &fakeSite{dat: makeZip(t, map[string]string{"Sony - PlayStation.dat": datXML("Game A", "Game B")})}
	dl := site.start(t)
	db := openDB(t)
	sys := System{Name: "Sony PlayStation", Slug: "PSX", Version: "2026-01-01 00:00:00"}

	res, err := SyncSystem(context.Background(), db, dl, sys, false)
	if err != nil || !res.Updated || res.SystemID != "psx" || res.DatGames != 2 {
		t.Fatalf("first sync: %+v err=%v", res, err)
	}
	if entryCount(t, db) != 2 {
		t.Fatalf("entries = %d", entryCount(t, db))
	}

	hits := site.hits.Load()
	res, err = SyncSystem(context.Background(), db, dl, sys, false)
	if err != nil || res.Updated || site.hits.Load() != hits {
		t.Fatalf("unchanged version must not hit the network: %+v hits %d->%d err=%v", res, hits, site.hits.Load(), err)
	}

	site.dat = makeZip(t, map[string]string{"Sony - PlayStation.dat": datXML("Game A renamed", "Game B", "Game C")})
	sys.Version = "2026-02-02 00:00:00"
	res, err = SyncSystem(context.Background(), db, dl, sys, false)
	if err != nil || !res.Updated || !strings.Contains(res.Reason, "updated") {
		t.Fatalf("update: %+v err=%v", res, err)
	}
	if entryCount(t, db) != 3 {
		t.Errorf("old import not pruned: entries = %d, want 3", entryCount(t, db))
	}
	var dats int
	db.QueryRow(`SELECT COUNT(*) FROM dats`).Scan(&dats)
	if dats != 1 {
		t.Errorf("dats rows = %d, want 1", dats)
	}
}

func TestSyncSystemUnmappedSlugDoesNothing(t *testing.T) {
	site := &fakeSite{}
	dl := site.start(t)
	res, err := SyncSystem(context.Background(), openDB(t), dl, System{Slug: "ZZZ", Version: "x"}, false)
	if err != nil || res.Updated || res.Reason == "" || site.hits.Load() != 0 {
		t.Fatalf("%+v err=%v hits=%d", res, err, site.hits.Load())
	}
}

func TestFetchSBIIgnoresTraversalAndNonSBI(t *testing.T) {
	site := &fakeSite{sbi: makeZip(t, map[string]string{
		"Game A.sbi":         "SBI",
		"../../escape.sbi":   "evil",
		"sub/dir/Game B.sbi": "SBI",
		"readme.txt":         "no",
	})}
	dl := site.start(t)
	n, err := dl.FetchSBI(context.Background(), "PSX")
	if err != nil {
		t.Fatal(err)
	}
	if n != 3 {
		t.Errorf("wrote %d, want 3 (txt skipped)", n)
	}
	dir := filepath.Join(dl.Dir, "sbi", "PSX")
	for _, name := range []string{"Game A.sbi", "Game B.sbi", "escape.sbi"} {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Errorf("missing %s: %v", name, err)
		}
	}
	if _, err := os.Stat(filepath.Join(dl.Dir, "escape.sbi")); err == nil {
		t.Error("zip-slip: file escaped the sbi directory")
	}
}

func TestSyncSystemWithSBI(t *testing.T) {
	site := &fakeSite{
		dat: makeZip(t, map[string]string{"x.dat": datXML("Game A")}),
		sbi: makeZip(t, map[string]string{"Game A.sbi": "SBI"}),
	}
	dl := site.start(t)
	res, err := SyncSystem(context.Background(), openDB(t), dl, System{Slug: "PSX", Version: "v1", HasSBI: true}, true)
	if err != nil || res.SBIFiles != 1 {
		t.Fatalf("%+v err=%v", res, err)
	}
}

func TestFetchDatWithoutDatEntryFails(t *testing.T) {
	site := &fakeSite{dat: makeZip(t, map[string]string{"readme.txt": "x"})}
	dl := site.start(t)
	if _, err := dl.FetchDat(context.Background(), "PSX"); err == nil {
		t.Fatal("expected error")
	}
}

func TestFetchDatHTTPError(t *testing.T) {
	site := &fakeSite{}
	dl := site.start(t)
	if _, err := dl.FetchDat(context.Background(), "NOPE"); err == nil {
		t.Fatal("expected HTTP 404 error")
	}
}
