package app

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
)

func zipOf(t *testing.T, files map[string]string) []byte {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for n, c := range files {
		w, _ := zw.Create(n)
		w.Write([]byte(c))
	}
	zw.Close()
	return buf.Bytes()
}

func TestUpdateDATsRedumpThenSyncUsesIt(t *testing.T) {
	page, err := os.ReadFile(filepath.Join("..", "..", "testdata", "redump", "downloads.html"))
	if err != nil {
		t.Fatal(err)
	}
	a := newApp(t)

	// A PSX folder with one raw image whose bytes the fake DAT will verify.
	src := filepath.Join(t.TempDir(), "psx")
	os.MkdirAll(src, 0o755)
	img := filepath.Join(src, "whatever.iso")
	os.WriteFile(img, []byte("disc-bytes"), 0o644)
	// crc32("disc-bytes"), computed by the scanner in the real flow; here we
	// precompute it the same way.
	crc := crcHex([]byte("disc-bytes"))
	dat := `<?xml version="1.0"?><datafile><header><name>Sony - PlayStation</name></header>` +
		`<game name="Real Game (Europe)"><description>x</description><rom name="Real Game (Europe).iso" size="10" crc="` + crc + `"/></game></datafile>`

	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		switch r.URL.Path {
		case "/downloads":
			w.Write(page)
		case "/datfile/PSX":
			w.Write(zipOf(t, map[string]string{"psx.dat": dat}))
		case "/sbi/PSX":
			w.Write(zipOf(t, map[string]string{"Real Game (Europe).sbi": "SBI"}))
		case "/nointro":
			w.Write([]byte("[]"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	if _, err := a.AddSource(src, "psx"); err != nil {
		t.Fatal(err)
	}
	nes := filepath.Join(t.TempDir(), "nes")
	writeROMs(t, nes)
	if _, err := a.AddSource(nes, "nes"); err != nil {
		t.Fatal(err)
	}

	ep := Endpoints{Client: srv.Client(), RedumpURL: srv.URL, TOSECURL: srv.URL, NoIntroURL: srv.URL + "/nointro"}
	results, err := a.UpdateDATs(context.Background(), ep, nil)
	if err != nil {
		t.Fatal(err)
	}
	byID := map[string]DATResult{}
	for _, r := range results {
		byID[r.SystemID] = r
	}
	if r := byID["psx"]; r.Source != "redump" || !r.Updated || r.Error != "" || !strings.Contains(r.Message, "SBI") {
		t.Errorf("psx = %+v", r)
	}
	if r := byID["nes"]; r.Source != "" || r.Message == "" {
		t.Errorf("nes (no online DAT) = %+v", r)
	}

	// The SBI landed where profile generation looks for it.
	if _, err := os.Stat(filepath.Join(a.Dir, "sbi", "PSX", "Real Game (Europe).sbi")); err != nil {
		t.Errorf("sbi not stored for profiles: %v", err)
	}

	// Sync now identifies the image by its DAT name.
	rep, err := a.Sync(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	games, _ := a.DB.SelectedGames("psx")
	if len(games) != 1 || games[0].DisplayName != "Real Game (Europe)" {
		t.Errorf("games = %+v report=%+v", games, rep.Systems)
	}

	// Second update: catalog only, no DAT download.
	before := hits.Load()
	results, _ = a.UpdateDATs(context.Background(), ep, nil)
	if hits.Load() != before+2 {
		t.Errorf("unchanged DAT caused %d requests, want 2 (Redump's catalog and the No-Intro index)", hits.Load()-before)
	}
	for _, r := range results {
		if r.SystemID == "psx" && r.Updated {
			t.Error("unchanged DAT reported as updated")
		}
	}
}

func TestUpdateDATsReportsSiteFailurePerSystem(t *testing.T) {
	a := newApp(t)
	src := filepath.Join(t.TempDir(), "psx")
	os.MkdirAll(src, 0o755)
	a.AddSource(src, "psx")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Error(w, "down", http.StatusServiceUnavailable) }))
	defer srv.Close()
	results, err := a.UpdateDATs(context.Background(), Endpoints{Client: srv.Client(), RedumpURL: srv.URL}, nil)
	if err != nil || len(results) != 1 || results[0].Error == "" {
		t.Errorf("results=%+v err=%v", results, err)
	}
}

func TestVerifyCHDsNeedsChdman(t *testing.T) {
	a := newApp(t)
	if _, err := a.VerifyCHDs(context.Background(), nil); err == nil || !strings.Contains(err.Error(), "chdman") {
		t.Errorf("err = %v", err)
	}
}
