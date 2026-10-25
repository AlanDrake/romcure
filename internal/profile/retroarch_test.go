package profile

import (
	"archive/zip"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"romcure/internal/library"
	"romcure/internal/rules"
	"romcure/internal/scan"
	"romcure/internal/store"
)

func openTestDB(t *testing.T) *store.DB {
	t.Helper()
	db, err := store.Open(filepath.Join(t.TempDir(), "library.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func writeZip(t *testing.T, path string, files map[string]string) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	zw := zip.NewWriter(f)
	for name, content := range files {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
}

// sync is the scan+group+select pipeline shared by every test here.
func syncSystem(t *testing.T, db *store.DB, root, systemID string) {
	t.Helper()
	if err := scan.New(db).Scan(context.Background(), root, nil); err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if _, err := library.Sync(db, root, systemID, rules.Preferences{}); err != nil {
		t.Fatalf("Sync: %v", err)
	}
}

func readFileContent(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}

func TestGenerateRetroArchRawFile(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "Super Game (USA).nes"), "raw-rom-content")

	db := openTestDB(t)
	syncSystem(t, db, root, "nes")

	outRoot := t.TempDir()
	report, err := GenerateRetroArch(db, "nes", Options{OutputRoot: outRoot})
	if err != nil {
		t.Fatalf("GenerateRetroArch: %v", err)
	}
	if len(report.Errors) != 0 {
		t.Fatalf("Errors = %v, want none", report.Errors)
	}
	if len(report.Linked) != 1 {
		t.Fatalf("len(Linked) = %d, want 1", len(report.Linked))
	}

	linked := report.Linked[0]
	// MatchHeuristic's Title strips the "(USA)" tag (spec §2.6 — tested
	// in internal/match), so the clean filename does too.
	if filepath.Base(linked.Path) != "Super Game.nes" {
		t.Errorf("linked path = %s, want basename %q", linked.Path, "Super Game.nes")
	}
	if readFileContent(t, linked.Path) != "raw-rom-content" {
		t.Errorf("linked file content mismatch")
	}

	// Playlist exists and references the linked path.
	dbName, _ := LibretroDBName("nes")
	playlistPath := filepath.Join(outRoot, dbName+".lpl")
	if _, err := os.Stat(playlistPath); err != nil {
		t.Errorf("playlist not written: %v", err)
	}
}

func TestGenerateRetroArchZipPassthrough(t *testing.T) {
	root := t.TempDir()
	writeZip(t, filepath.Join(root, "set.zip"), map[string]string{
		"Clean Game (USA).nes": "zipped-content",
	})

	db := openTestDB(t)
	syncSystem(t, db, root, "nes")

	outRoot := t.TempDir()
	report, err := GenerateRetroArch(db, "nes", Options{OutputRoot: outRoot})
	if err != nil {
		t.Fatalf("GenerateRetroArch: %v", err)
	}
	if len(report.Errors) != 0 {
		t.Fatalf("Errors = %v, want none", report.Errors)
	}
	if len(report.Linked) != 1 {
		t.Fatalf("len(Linked) = %d, want 1", len(report.Linked))
	}

	linked := report.Linked[0]
	if filepath.Ext(linked.Path) != ".zip" {
		t.Errorf("passthrough should keep the .zip extension, got %s", linked.Path)
	}
	if _, err := os.Stat(linked.Path); err != nil {
		t.Errorf("linked archive missing: %v", err)
	}
}

func TestGenerateRetroArchMultiROMZipExtracts(t *testing.T) {
	// Two real ROMs in one zip — archive policy must extract both,
	// since passthrough would hand either target the whole archive
	// (spec §2.8: passthrough only applies to a single-ROM archive).
	root := t.TempDir()
	writeZip(t, filepath.Join(root, "multi.zip"), map[string]string{
		"Wanted Game (USA).nes": "wanted-content",
		"Other Game (USA).nes":  "other-content",
	})

	db := openTestDB(t)
	syncSystem(t, db, root, "nes")

	outRoot := t.TempDir()
	extractDir := t.TempDir()
	report, err := GenerateRetroArch(db, "nes", Options{OutputRoot: outRoot, ExtractCacheDir: extractDir})
	if err != nil {
		t.Fatalf("GenerateRetroArch: %v", err)
	}
	if len(report.Errors) != 0 {
		t.Fatalf("Errors = %v, want none", report.Errors)
	}
	if len(report.Linked) != 2 {
		t.Fatalf("len(Linked) = %d, want 2", len(report.Linked))
	}

	byContent := make(map[string]string) // content -> extension
	for _, l := range report.Linked {
		if filepath.Ext(l.Path) != ".nes" {
			t.Errorf("extracted link should have the member's own extension (.nes), got %s", l.Path)
		}
		byContent[readFileContent(t, l.Path)] = filepath.Ext(l.Path)
	}
	if _, ok := byContent["wanted-content"]; !ok {
		t.Error("wanted-content was not extracted")
	}
	if _, ok := byContent["other-content"]; !ok {
		t.Error("other-content was not extracted")
	}
}

func TestGenerateRetroArchMissingExtractCacheDirIsReportedNotFatal(t *testing.T) {
	root := t.TempDir()
	writeZip(t, filepath.Join(root, "multi.zip"), map[string]string{
		"Wanted Game (USA).nes": "wanted-content",
		"Other Game (USA).nes":  "other-content",
	})

	db := openTestDB(t)
	syncSystem(t, db, root, "nes")

	outRoot := t.TempDir()
	report, err := GenerateRetroArch(db, "nes", Options{OutputRoot: outRoot}) // no ExtractCacheDir
	if err != nil {
		t.Fatalf("GenerateRetroArch returned a hard error, want it reported per-game: %v", err)
	}
	if len(report.Errors) != 2 {
		t.Fatalf("Errors = %v, want exactly 2 (both games need extraction)", report.Errors)
	}
	if len(report.Linked) != 0 {
		t.Errorf("Linked = %v, want none", report.Linked)
	}
}

func TestGenerateRetroArchIsIdempotent(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "Super Game (USA).nes"), "raw-rom-content")

	db := openTestDB(t)
	syncSystem(t, db, root, "nes")

	outRoot := t.TempDir()
	opts := Options{OutputRoot: outRoot}

	if _, err := GenerateRetroArch(db, "nes", opts); err != nil {
		t.Fatalf("first generate: %v", err)
	}
	linkedPath := filepath.Join(outRoot, func() string { n, _ := LibretroDBName("nes"); return n }(), "Super Game.nes")
	info1, err := os.Stat(linkedPath)
	if err != nil {
		t.Fatalf("stat after first generate: %v", err)
	}

	report2, err := GenerateRetroArch(db, "nes", opts)
	if err != nil {
		t.Fatalf("second generate: %v", err)
	}
	if len(report2.Errors) != 0 {
		t.Fatalf("second generate Errors = %v, want none", report2.Errors)
	}
	info2, err := os.Stat(linkedPath)
	if err != nil {
		t.Fatalf("stat after second generate: %v", err)
	}
	if info1.ModTime() != info2.ModTime() {
		t.Error("re-generating touched an already-linked file (should be a no-op for existing targets)")
	}

	var linkStateCount int
	if err := db.QueryRow(`SELECT COUNT(*) FROM link_state WHERE profile_id = 'retroarch'`).Scan(&linkStateCount); err != nil {
		t.Fatal(err)
	}
	if linkStateCount != 1 {
		t.Errorf("link_state row count = %d, want 1 (no duplicate rows across re-generates)", linkStateCount)
	}
}

func TestGenerateRetroArchDiscWithoutTranscodeToolReportsError(t *testing.T) {
	root := t.TempDir()
	writeZip(t, filepath.Join(root, "Disc Game (USA).zip"), map[string]string{
		"Disc Game (USA).cue": "disc-content",
	})

	db := openTestDB(t)
	syncSystem(t, db, root, "psx") // psx is a disc system

	report, err := GenerateRetroArch(db, "psx", Options{OutputRoot: t.TempDir()})
	if err != nil {
		t.Fatalf("GenerateRetroArch returned a hard error, want it reported per-game: %v", err)
	}
	if len(report.Linked) != 0 {
		t.Errorf("Linked = %v, want none (no transcode tool configured)", report.Linked)
	}
	if len(report.Errors) != 1 {
		t.Fatalf("Errors = %v, want exactly 1", report.Errors)
	}
}

func TestGenerateRetroArchSevenZipExtracts(t *testing.T) {
	// testdata/archives/profile_sample.7z contains one member,
	// "7z Game (USA).nes", content "sevenzip-extracted-content". Built
	// with real 7-Zip (bodgit/sevenzip is read-only).
	root := t.TempDir()
	fixture, err := os.ReadFile(filepath.Join("..", "..", "testdata", "archives", "profile_sample.7z"))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "game.7z"), fixture, 0o644); err != nil {
		t.Fatal(err)
	}

	db := openTestDB(t)
	syncSystem(t, db, root, "nes")

	outRoot := t.TempDir()
	extractDir := t.TempDir()
	report, err := GenerateRetroArch(db, "nes", Options{OutputRoot: outRoot, ExtractCacheDir: extractDir})
	if err != nil {
		t.Fatalf("GenerateRetroArch: %v", err)
	}
	if len(report.Errors) != 0 {
		t.Fatalf("Errors = %v, want none", report.Errors)
	}
	if len(report.Linked) != 1 {
		t.Fatalf("len(Linked) = %d, want 1", len(report.Linked))
	}

	linked := report.Linked[0]
	if filepath.Ext(linked.Path) != ".nes" {
		t.Errorf("7z always extracts; link should have the member's own extension (.nes), got %s", linked.Path)
	}
	if got := readFileContent(t, linked.Path); got != "sevenzip-extracted-content" {
		t.Errorf("extracted content = %q, want %q", got, "sevenzip-extracted-content")
	}
}

func writeCueBin(t *testing.T, dir, baseName string) {
	t.Helper()
	cue := baseName + ".cue"
	bin := baseName + ".bin"
	writeFile(t, filepath.Join(dir, cue), `FILE "`+bin+`" BINARY
  TRACK 01 MODE2/2352
    INDEX 01 00:00:00
`)
	writeFile(t, filepath.Join(dir, bin), "bin-content-for-"+baseName)
}

func TestGenerateRetroArchCueBinLinksBothFiles(t *testing.T) {
	root := t.TempDir()
	writeCueBin(t, root, "Single Disc Game (USA)")

	db := openTestDB(t)
	syncSystem(t, db, root, "psx")

	outRoot := t.TempDir()
	report, err := GenerateRetroArch(db, "psx", Options{OutputRoot: outRoot})
	if err != nil {
		t.Fatalf("GenerateRetroArch: %v", err)
	}
	if len(report.Errors) != 0 {
		t.Fatalf("Errors = %v, want none", report.Errors)
	}
	if len(report.Linked) != 1 {
		t.Fatalf("len(Linked) = %d, want 1 (the .cue is one game)", len(report.Linked))
	}

	cueDst := report.Linked[0].Path
	if filepath.Ext(cueDst) != ".cue" {
		t.Fatalf("linked path = %s, want a .cue", cueDst)
	}
	// The .bin must be linked alongside it with its EXACT original name
	// (spec §2.5), not the cleaned-up display name.
	binDst := filepath.Join(filepath.Dir(cueDst), "Single Disc Game (USA).bin")
	if _, err := os.Stat(binDst); err != nil {
		t.Fatalf(".bin not linked alongside .cue: %v", err)
	}
	if got := readFileContent(t, binDst); got != "bin-content-for-Single Disc Game (USA)" {
		t.Errorf(".bin content = %q, want the original bin content", got)
	}
}

func TestGenerateRetroArchMultiDiscM3U(t *testing.T) {
	root := t.TempDir()
	writeCueBin(t, root, "Epic RPG (USA) (Disc 1)")
	writeCueBin(t, root, "Epic RPG (USA) (Disc 2)")

	db := openTestDB(t)
	syncSystem(t, db, root, "psx")

	outRoot := t.TempDir()
	report, err := GenerateRetroArch(db, "psx", Options{OutputRoot: outRoot})
	if err != nil {
		t.Fatalf("GenerateRetroArch: %v", err)
	}
	if len(report.Errors) != 0 {
		t.Fatalf("Errors = %v, want none", report.Errors)
	}
	// Each disc links its .cue AND .bin, but report.Linked only tracks
	// the "game" (the .cue) per linkOneGame call, so 2 discs -> 2 entries.
	if len(report.Linked) != 2 {
		t.Fatalf("len(Linked) = %d, want 2 (one per disc)", len(report.Linked))
	}

	dbName, _ := LibretroDBName("psx")
	sysDir := filepath.Join(outRoot, dbName)
	discsDir := filepath.Join(sysDir, ".discs")

	for _, l := range report.Linked {
		if filepath.Dir(l.Path) != discsDir {
			t.Errorf("disc %s linked into %s, want %s", l.Title, filepath.Dir(l.Path), discsDir)
		}
	}

	// MatchHeuristic's Title strips every parenthesized tag, region
	// included (tested in internal/match), so the base title here is
	// "Epic RPG", not "Epic RPG (USA)".
	m3uPath := filepath.Join(sysDir, "Epic RPG.m3u")
	data, err := os.ReadFile(m3uPath)
	if err != nil {
		t.Fatalf("read m3u: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, ".discs/Epic RPG (Disc 1).cue") && !strings.Contains(content, ".discs\\Epic RPG (Disc 1).cue") {
		t.Errorf("m3u content = %q, missing disc 1 relative path", content)
	}
	if !strings.Contains(content, ".discs/Epic RPG (Disc 2).cue") && !strings.Contains(content, ".discs\\Epic RPG (Disc 2).cue") {
		t.Errorf("m3u content = %q, missing disc 2 relative path", content)
	}

	// Exactly one .lpl item for the whole set, not one per disc.
	playlistData, err := os.ReadFile(filepath.Join(outRoot, dbName+".lpl"))
	if err != nil {
		t.Fatalf("read playlist: %v", err)
	}
	var playlist lplFile
	if err := json.Unmarshal(playlistData, &playlist); err != nil {
		t.Fatalf("unmarshal playlist: %v", err)
	}
	if len(playlist.Items) != 1 {
		t.Fatalf("len(playlist.Items) = %d, want 1", len(playlist.Items))
	}
	if playlist.Items[0].Path != m3uPath {
		t.Errorf("playlist item path = %s, want the .m3u path %s", playlist.Items[0].Path, m3uPath)
	}
}
