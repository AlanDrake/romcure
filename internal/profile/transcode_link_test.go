package profile

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"romcure/internal/transcode"
)

func TestGenerateRetroArchZippedCueBinBecomesVerifiedCHD(t *testing.T) {
	chdman, err := filepath.Abs(filepath.Join("..", "..", "tools", "bin", runtime.GOOS+"-"+runtime.GOARCH, "chdman"))
	if runtime.GOOS == "windows" {
		chdman += ".exe"
	}
	if _, serr := os.Stat(chdman); err != nil || serr != nil {
		t.Skip("real chdman not present")
	}

	root := t.TempDir()
	bin := make([]byte, 2352*400)
	for i := range bin {
		bin[i] = byte(i * 13)
	}
	writeZip(t, filepath.Join(root, "Disc Game (USA).zip"), map[string]string{
		"Disc Game (USA).cue": "FILE \"Disc Game (USA).bin\" BINARY\n  TRACK 01 MODE1/2352\n    INDEX 01 00:00:00\n",
		"Disc Game (USA).bin": string(bin),
	})
	db := openTestDB(t)
	syncSystem(t, db, root, "psx")

	out, scratch := t.TempDir(), t.TempDir()
	opts := Options{OutputRoot: out, Transcode: &TranscodeOptions{
		Cache:      &transcode.Cache{Dir: filepath.Join(t.TempDir(), "cache"), DB: db},
		Tool:       &transcode.Chdman{Path: chdman, Version: "test"},
		ScratchDir: scratch,
	}}
	report, err := GenerateRetroArch(db, "psx", opts)
	if err != nil || len(report.Errors) != 0 {
		t.Fatalf("err=%v report errors=%v", err, report.Errors)
	}
	if len(report.Linked) != 1 || !strings.HasSuffix(report.Linked[0].Path, ".chd") {
		t.Fatalf("Linked = %+v, want one .chd", report.Linked)
	}
	if st, err := os.Stat(report.Linked[0].Path); err != nil || st.Size() == 0 {
		t.Fatalf("linked CHD unreadable: %v", err)
	}
	if left, _ := os.ReadDir(scratch); len(left) != 0 {
		t.Errorf("scratch not cleaned: %v", left)
	}

	// Re-run: cache hit, same link, still no errors.
	report, err = GenerateRetroArch(db, "psx", opts)
	if err != nil || len(report.Errors) != 0 {
		t.Fatalf("second run err=%v errors=%v", err, report.Errors)
	}
}

func TestMediaForSystem(t *testing.T) {
	if mediaForSystem("psp", 100<<20) != transcode.MediaDVD {
		t.Error("a small PSP image must still use createdvd")
	}
	if mediaForSystem("psx", 600<<20) != transcode.MediaCD || mediaForSystem("ps2", 4<<30) != transcode.MediaDVD {
		t.Error("psx/ps2 should be decided by size")
	}
}

func TestGenerateRetroArchRawCueBinBecomesCHDAndStorageIsMeasured(t *testing.T) {
	chdman, err := filepath.Abs(filepath.Join("..", "..", "tools", "bin", runtime.GOOS+"-"+runtime.GOARCH, "chdman"))
	if runtime.GOOS == "windows" {
		chdman += ".exe"
	}
	if _, serr := os.Stat(chdman); err != nil || serr != nil {
		t.Skip("real chdman not present")
	}
	root := t.TempDir()
	bin := make([]byte, 2352*400)
	for i := range bin {
		bin[i] = byte(i * 11)
	}
	writeFile(t, filepath.Join(root, "Raw Disc (USA).bin"), string(bin))
	writeFile(t, filepath.Join(root, "Raw Disc (USA).cue"), "FILE \"Raw Disc (USA).bin\" BINARY\n  TRACK 01 MODE1/2352\n    INDEX 01 00:00:00\n")
	db := openTestDB(t)
	syncSystem(t, db, root, "psx")

	out := t.TempDir()
	cache := &transcode.Cache{Dir: filepath.Join(t.TempDir(), "cache"), DB: db}
	report, err := GenerateRetroArch(db, "psx", Options{OutputRoot: out, Transcode: &TranscodeOptions{
		Cache: cache, Tool: &transcode.Chdman{Path: chdman, Version: "test"}, ScratchDir: t.TempDir(),
	}})
	if err != nil || len(report.Errors) != 0 || len(report.Linked) != 1 {
		t.Fatalf("err=%v report=%+v", err, report)
	}
	if !strings.HasSuffix(report.Linked[0].Path, ".chd") {
		t.Fatalf("linked %s, want .chd", report.Linked[0].Path)
	}
	// The source folder must be untouched (read-only guarantee).
	if ents, _ := os.ReadDir(root); len(ents) != 2 {
		t.Errorf("source dir modified: %v", ents)
	}

	s, err := transcode.MeasureStorage(db, cache, []string{root}, []string{out})
	if err != nil {
		t.Fatal(err)
	}
	if s.SourceBytes < int64(len(bin)) || s.CacheEntries != 1 || s.CacheBytes <= 0 || s.CacheBytes >= s.SourceBytes {
		t.Errorf("storage = %+v", s)
	}
	// Output holds a symlink + .lpl only; the CHD isn't double-counted.
	// Only when it is a symlink: without symlink rights (Windows without Developer Mode) the CHD is copied.
	if li, err := os.Lstat(report.Linked[0].Path); err == nil && li.Mode()&os.ModeSymlink != 0 && s.OutputBytes >= s.CacheBytes {
		t.Errorf("OutputBytes=%d counts linked CHD (cache=%d)", s.OutputBytes, s.CacheBytes)
	}
}

func TestGenerateRetroArchLinksSBIBesideGame(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "Libcrypt Game (Europe).bin"), "bin-data")
	writeFile(t, filepath.Join(root, "Libcrypt Game (Europe).cue"), "FILE \"Libcrypt Game (Europe).bin\" BINARY\n  TRACK 01 MODE2/2352\n    INDEX 01 00:00:00\n")
	sbiDir := t.TempDir()
	writeFile(t, filepath.Join(sbiDir, "Libcrypt Game (Europe).sbi"), "SBI-bytes")
	writeFile(t, filepath.Join(sbiDir, "Other Game.sbi"), "unused")

	db := openTestDB(t)
	syncSystem(t, db, root, "psx")
	// Make the game a DAT match (as a Redump import would): without a DAT
	// match there is no DAT name to look an SBI up by.
	if _, err := db.Exec(`INSERT INTO dats (id, system_id, format, source_path) VALUES (1, 'psx', 'logiqx_xml', 'x')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO dat_entries (id, dat_id, system_id, game_name, rom_name, size) VALUES (1, 1, 'psx', 'Libcrypt Game (Europe)', 'a.bin', 8)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE games SET dat_entry_id = 1`); err != nil {
		t.Fatal(err)
	}

	out := t.TempDir()
	report, err := GenerateRetroArch(db, "psx", Options{OutputRoot: out, SBIDir: sbiDir})
	if err != nil || len(report.Errors) != 0 || len(report.Linked) != 1 {
		t.Fatalf("err=%v report=%+v", err, report)
	}
	game := report.Linked[0].Path
	sbi := strings.TrimSuffix(game, filepath.Ext(game)) + ".sbi"
	if got := readFileContent(t, sbi); got != "SBI-bytes" {
		t.Errorf("sbi content = %q", got)
	}
	if _, err := os.Lstat(filepath.Join(filepath.Dir(game), "Other Game.sbi")); err == nil {
		t.Error("an SBI for a different game was linked")
	}

	// Without SBIDir nothing is linked and nothing fails.
	out2 := t.TempDir()
	if _, err := GenerateRetroArch(db, "psx", Options{OutputRoot: out2}); err != nil {
		t.Fatal(err)
	}
	if m, _ := filepath.Glob(filepath.Join(out2, "*", "*.sbi")); len(m) != 0 {
		t.Errorf("sbi linked with SBIDir unset: %v", m)
	}
}
