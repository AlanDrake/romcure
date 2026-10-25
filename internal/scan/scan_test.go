package scan

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"hash/crc32"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"romcure/internal/store"
)

func TestClassify(t *testing.T) {
	tests := []struct {
		path string
		want string
	}{
		{"game.zip", "zip"},
		{"Game.ZIP", "zip"},
		{"game.7z", "sevenzip"},
		{"game.7Z", "sevenzip"},
		{"game.nes", "raw"},
		{"game.bin", "raw"},
		{"game", "raw"},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			if got := classify(tt.path); got != tt.want {
				t.Errorf("classify(%q) = %q, want %q", tt.path, got, tt.want)
			}
		})
	}
}

func TestCrc32File(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "game.bin")
	content := []byte("hello rom world")
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatal(err)
	}

	got, _, err := hashFile(path, int64(len(content)))
	if err != nil {
		t.Fatalf("hashFile: %v", err)
	}
	want := fmt.Sprintf("%08x", crc32.ChecksumIEEE(content))
	if got != want {
		t.Errorf("hashFile(%s) = %s, want %s", path, got, want)
	}
}

func writeTestZip(t *testing.T, path string, files map[string][]byte) {
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
		if _, err := w.Write(content); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestReadZipMembers(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "set.zip")
	contentA := []byte("rom-a-content")
	contentB := []byte("rom-b-content-longer")
	writeTestZip(t, path, map[string][]byte{
		"Game A (USA).nes": contentA,
		"Game B (USA).nes": contentB,
	})

	members, _, err := readZipMembers(path)
	if err != nil {
		t.Fatalf("readZipMembers: %v", err)
	}
	if len(members) != 2 {
		t.Fatalf("len(members) = %d, want 2", len(members))
	}

	byName := make(map[string]store.ArchiveMemberRecord)
	for _, m := range members {
		byName[m.MemberName] = m
	}

	wantCRCA := fmt.Sprintf("%08x", crc32.ChecksumIEEE(contentA))
	wantCRCB := fmt.Sprintf("%08x", crc32.ChecksumIEEE(contentB))

	a, ok := byName["Game A (USA).nes"]
	if !ok {
		t.Fatal("missing member Game A (USA).nes")
	}
	if a.Size != int64(len(contentA)) || a.CRC32 != wantCRCA {
		t.Errorf("Game A: size=%d crc32=%s, want size=%d crc32=%s", a.Size, a.CRC32, len(contentA), wantCRCA)
	}

	b, ok := byName["Game B (USA).nes"]
	if !ok {
		t.Fatal("missing member Game B (USA).nes")
	}
	if b.Size != int64(len(contentB)) || b.CRC32 != wantCRCB {
		t.Errorf("Game B: size=%d crc32=%s, want size=%d crc32=%s", b.Size, b.CRC32, len(contentB), wantCRCB)
	}
}

func TestReadSevenZipMembers(t *testing.T) {
	// testdata/archives/sample.7z contains exactly one member,
	// "Sample Game (USA).bin", 39 bytes, built with real 7-Zip (bodgit/
	// sevenzip is read-only, so this fixture can't be generated in Go).
	members, err := readSevenZipMembers(filepath.Join("..", "..", "testdata", "archives", "sample.7z"))
	if err != nil {
		t.Fatalf("readSevenZipMembers: %v", err)
	}
	if len(members) != 1 {
		t.Fatalf("len(members) = %d, want 1", len(members))
	}

	m := members[0]
	if m.MemberName != "Sample Game (USA).bin" {
		t.Errorf("MemberName = %q, want %q", m.MemberName, "Sample Game (USA).bin")
	}
	if m.Size != 39 {
		t.Errorf("Size = %d, want 39", m.Size)
	}
	if m.CRC32 != "c5b80c6b" {
		t.Errorf("CRC32 = %q, want %q", m.CRC32, "c5b80c6b")
	}
}

func openTestDB(t *testing.T) *store.DB {
	t.Helper()
	db, err := store.Open(filepath.Join(t.TempDir(), "library.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestScanPopulatesFilesAndArchiveMembers(t *testing.T) {
	root := t.TempDir()
	rawContent := []byte("raw-rom-content")
	if err := os.WriteFile(filepath.Join(root, "game.nes"), rawContent, 0o644); err != nil {
		t.Fatal(err)
	}
	zipContent := []byte("zipped-rom-content")
	writeTestZip(t, filepath.Join(root, "set.zip"), map[string][]byte{
		"Only Game (USA).nes": zipContent,
	})

	db := openTestDB(t)
	s := New(db)

	var lastProgress Progress
	err := s.Scan(context.Background(), root, func(p Progress) { lastProgress = p })
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if lastProgress.FilesScanned != 2 {
		t.Errorf("FilesScanned = %d, want 2", lastProgress.FilesScanned)
	}

	rawRec, err := db.GetFileByPath(filepath.Join(root, "game.nes"))
	if err != nil {
		t.Fatalf("GetFileByPath(raw): %v", err)
	}
	if rawRec == nil {
		t.Fatal("raw file not found in cache")
	}
	if rawRec.Kind != "raw" {
		t.Errorf("raw file kind = %q, want raw", rawRec.Kind)
	}
	wantRawCRC := fmt.Sprintf("%08x", crc32.ChecksumIEEE(rawContent))
	if !rawRec.CRC32.Valid || rawRec.CRC32.String != wantRawCRC {
		t.Errorf("raw file crc32 = %v, want %q", rawRec.CRC32, wantRawCRC)
	}

	zipRec, err := db.GetFileByPath(filepath.Join(root, "set.zip"))
	if err != nil {
		t.Fatalf("GetFileByPath(zip): %v", err)
	}
	if zipRec == nil {
		t.Fatal("zip file not found in cache")
	}
	if zipRec.Kind != "zip" {
		t.Errorf("zip file kind = %q, want zip", zipRec.Kind)
	}
	if zipRec.CRC32.Valid {
		t.Errorf("zip file crc32 = %v, want NULL (member CRC lives in archive_members)", zipRec.CRC32)
	}

	var memberName, memberCRC string
	var memberSize int64
	var isOnly bool
	err = db.QueryRow(
		`SELECT member_name, size, crc32, is_only_member FROM archive_members WHERE file_id = ?`,
		zipRec.ID,
	).Scan(&memberName, &memberSize, &memberCRC, &isOnly)
	if err != nil {
		t.Fatalf("query archive_members: %v", err)
	}
	if memberName != "Only Game (USA).nes" {
		t.Errorf("member_name = %q, want %q", memberName, "Only Game (USA).nes")
	}
	wantZipCRC := fmt.Sprintf("%08x", crc32.ChecksumIEEE(zipContent))
	if memberCRC != wantZipCRC {
		t.Errorf("member crc32 = %q, want %q", memberCRC, wantZipCRC)
	}
	if !isOnly {
		t.Error("is_only_member = false, want true (archive has exactly one member)")
	}
}

func TestScanCacheHitSkipsRehash(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "game.nes")
	if err := os.WriteFile(path, []byte("content"), 0o644); err != nil {
		t.Fatal(err)
	}

	db := openTestDB(t)
	s := New(db)

	if err := s.Scan(context.Background(), root, nil); err != nil {
		t.Fatalf("first Scan: %v", err)
	}
	first, err := db.GetFileByPath(path)
	if err != nil || first == nil {
		t.Fatalf("GetFileByPath after first scan: rec=%v err=%v", first, err)
	}

	// Sleep so a real re-hash (which refreshes last_scanned_at) would be
	// detectable even at low timestamp resolution.
	time.Sleep(10 * time.Millisecond)

	if err := s.Scan(context.Background(), root, nil); err != nil {
		t.Fatalf("second Scan: %v", err)
	}
	second, err := db.GetFileByPath(path)
	if err != nil || second == nil {
		t.Fatalf("GetFileByPath after second scan: rec=%v err=%v", second, err)
	}

	if second.LastScannedAt != first.LastScannedAt {
		t.Errorf("last_scanned_at changed on an unchanged file: %s -> %s (rehashed instead of using the cache)", first.LastScannedAt, second.LastScannedAt)
	}
}

func TestScanResumeAfterCancel(t *testing.T) {
	root := t.TempDir()
	const total = 5
	for i := 0; i < total; i++ {
		name := filepath.Join(root, "game"+string(rune('A'+i))+".nes")
		if err := os.WriteFile(name, []byte("content"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	db := openTestDB(t)
	s := New(db)

	ctx, cancel := context.WithCancel(context.Background())
	seen := 0
	err := s.Scan(ctx, root, func(p Progress) {
		seen++
		if seen == 2 {
			cancel()
		}
	})
	if err == nil {
		t.Fatal("Scan with a cancelled context returned nil error, want context.Canceled")
	}

	countFiles := func() int {
		var n int
		if err := db.QueryRow(`SELECT COUNT(*) FROM files`).Scan(&n); err != nil {
			t.Fatalf("count files: %v", err)
		}
		return n
	}

	partial := countFiles()
	if partial >= total {
		t.Fatalf("got %d files cached after cancellation, want fewer than %d", partial, total)
	}

	// Resume: a fresh, uncancelled Scan should pick up where the first
	// left off and reach full coverage, without needing separate resume
	// state (spec §5).
	if err := s.Scan(context.Background(), root, nil); err != nil {
		t.Fatalf("resumed Scan: %v", err)
	}
	if got := countFiles(); got != total {
		t.Errorf("files cached after resume = %d, want %d", got, total)
	}
}

// One damaged archive must not stop the scan of everything else: it is
// skipped, reported with a reason, and any older record of it is forgotten.
func TestScanSkipsUnreadableFilesAndKeepsGoing(t *testing.T) {
	root := t.TempDir()
	writeTestZip(t, filepath.Join(root, "good.zip"), map[string][]byte{"Good (USA).nes": []byte("good")})
	os.WriteFile(filepath.Join(root, "garbage.7z"), []byte("this is not a 7z archive at all"), 0o644)
	os.WriteFile(filepath.Join(root, "truncated.zip"), []byte("PK\x03\x04 cut off"), 0o644)
	os.WriteFile(filepath.Join(root, "plain.nes"), []byte("plain"), 0o644)
	os.WriteFile(filepath.Join(root, "zzz-last.nes"), []byte("last"), 0o644) // sorts after the bad ones

	db := openTestDB(t)
	sc := New(db)
	if err := sc.Scan(context.Background(), root, nil); err != nil {
		t.Fatalf("a damaged file must not fail the scan: %v", err)
	}
	if len(sc.Problems) != 2 {
		t.Fatalf("problems = %+v, want the two damaged archives", sc.Problems)
	}
	for _, p := range sc.Problems {
		if !strings.Contains(p.Reason, "damaged or unreadable archive") || strings.Contains(p.Reason, root) {
			t.Errorf("reason for %s should say what is wrong without repeating the path: %q", p.Path, p.Reason)
		}
	}
	files, _ := db.ListFilesUnderRoot(root)
	names := map[string]bool{}
	for _, f := range files {
		names[filepath.Base(f.Path)] = true
	}
	for _, want := range []string{"good.zip", "plain.nes", "zzz-last.nes"} {
		if !names[want] {
			t.Errorf("%s was not scanned (files: %v)", want, names)
		}
	}
	if names["garbage.7z"] || names["truncated.zip"] {
		t.Errorf("unreadable files must not be recorded: %v", names)
	}

	// A file that was fine and later gets damaged is forgotten, not kept stale.
	os.WriteFile(filepath.Join(root, "good.zip"), []byte("PK\x03\x04 now broken, and a different size"), 0o644)
	if err := sc.Scan(context.Background(), root, nil); err != nil {
		t.Fatal(err)
	}
	files, _ = db.ListFilesUnderRoot(root)
	for _, f := range files {
		if filepath.Base(f.Path) == "good.zip" {
			t.Error("stale record of a now-damaged archive was kept")
		}
	}
	if len(sc.Problems) != 3 {
		t.Errorf("problems = %d, want 3", len(sc.Problems))
	}
}

func TestScanStillFailsWhenTheRootCannotBeRead(t *testing.T) {
	db := openTestDB(t)
	if err := New(db).Scan(context.Background(), filepath.Join(t.TempDir(), "missing"), nil); err == nil {
		t.Error("an unreadable root means nothing can be scanned and must be an error")
	}
}

// Headered cartridge dumps also get the CRC of the data after the header,
// raw and inside zip / 7z archives, so No-Intro DATs (which list headerless
// CRCs) can match them.
func TestScanRecordsTheCRCWithoutTheCopierHeader(t *testing.T) {
	db := openTestDB(t)
	root := t.TempDir()
	body := bytes.Repeat([]byte{0x4c, 0x00, 0x80}, 4000)
	nes := append(append([]byte("NES\x1a"), make([]byte, 12)...), body...)
	want := fmt.Sprintf("%08x", crc32.ChecksumIEEE(body))
	os.WriteFile(filepath.Join(root, "Raw (USA).nes"), nes, 0o644)
	writeTestZip(t, filepath.Join(root, "Zipped (USA).zip"), map[string][]byte{"Zipped (USA).nes": nes, "readme.txt": []byte("hi")})

	if err := New(db).Scan(context.Background(), root, nil); err != nil {
		t.Fatal(err)
	}
	raw, err := db.GetFileByPath(filepath.Join(root, "Raw (USA).nes"))
	if err != nil || raw == nil {
		t.Fatal(err)
	}
	if raw.AltCRC32 != want || raw.AltSize != int64(len(body)) {
		t.Errorf("raw: alt = %s/%d, want %s/%d", raw.AltCRC32, raw.AltSize, want, len(body))
	}
	if raw.CRC32.String != fmt.Sprintf("%08x", crc32.ChecksumIEEE(nes)) {
		t.Errorf("raw: the whole-file CRC must not change: %s", raw.CRC32.String)
	}
	zf, _ := db.GetFileByPath(filepath.Join(root, "Zipped (USA).zip"))
	members, _ := db.ListArchiveMembers(zf.ID)
	for _, m := range members {
		switch m.MemberName {
		case "Zipped (USA).nes":
			if m.AltCRC32 != want {
				t.Errorf("zip member alt = %q, want %s", m.AltCRC32, want)
			}
		case "readme.txt":
			if m.AltCRC32 != "" {
				t.Errorf("a text file has no header: %q", m.AltCRC32)
			}
		}
	}
}

// Files scanned before disc headers were recorded get them on the next scan,
// without being rehashed; raw images and archive members alike.
func TestDiscHeadersAreFilledInForFilesScannedBefore(t *testing.T) {
	db := openTestDB(t)
	root := t.TempDir()
	img := make([]byte, 0x9000)
	copy(img, "GEXE01")
	binary.BigEndian.PutUint32(img[0x1C:], 0xC2339F3D)
	copy(img[0x20:], "Adventure")
	os.WriteFile(filepath.Join(root, "scn-gzlt.gcm"), img, 0o644)
	writeTestZip(t, filepath.Join(root, "packed.zip"), map[string][]byte{"disc.iso": img})

	if err := New(db).Scan(context.Background(), root, nil); err != nil {
		t.Fatal(err)
	}
	// Pretend these were scanned by an older version.
	db.Exec(`UPDATE files SET disc_id = NULL, disc_title = NULL, last_scanned_at = 'old'`)
	db.Exec(`UPDATE archive_members SET disc_id = NULL, disc_title = NULL`)

	if err := New(db).Scan(context.Background(), root, nil); err != nil {
		t.Fatal(err)
	}
	raw, _ := db.GetFileByPath(filepath.Join(root, "scn-gzlt.gcm"))
	if raw.DiscID.String != "GEXE01" || raw.DiscTitle != "Adventure" {
		t.Errorf("raw: %q %q", raw.DiscID.String, raw.DiscTitle)
	}
	if raw.LastScannedAt != "old" {
		t.Error("the file was rehashed; only its header should have been read")
	}
	z, _ := db.GetFileByPath(filepath.Join(root, "packed.zip"))
	members, _ := db.ListArchiveMembers(z.ID)
	if len(members) != 1 || members[0].DiscID.String != "GEXE01" {
		t.Errorf("member: %+v", members)
	}
}

func TestScanRereadsAFileWhoseKindChanged(t *testing.T) {
	db := openTestDB(t)
	root := t.TempDir()
	path := filepath.Join(root, "pack.zip")
	writeTestZip(t, path, map[string][]byte{"game.gba": []byte("rom data")})
	if err := New(db).Scan(context.Background(), root, nil); err != nil {
		t.Fatal(err)
	}
	// An older version, without archive support for this extension, stored it as raw.
	if _, err := db.Exec(`DELETE FROM archive_members; UPDATE files SET kind = 'raw'`); err != nil {
		t.Fatal(err)
	}
	if err := New(db).Scan(context.Background(), root, nil); err != nil {
		t.Fatal(err)
	}
	f, _ := db.GetFileByPath(path)
	if f.Kind != "zip" {
		t.Fatalf("kind = %q, want zip", f.Kind)
	}
	members, _ := db.ListArchiveMembers(f.ID)
	if len(members) != 1 || members[0].MemberName != "game.gba" {
		t.Errorf("members = %+v", members)
	}
}

func TestCartridgeHeadersAreFilledInForFilesScannedBefore(t *testing.T) {
	db := openTestDB(t)
	root := t.TempDir()
	rom := make([]byte, 0x400)
	copy(rom[0xA0:], "EXQUEST")
	copy(rom[0xAC:], "AXQE")
	rom[0xB2] = 0x96
	var sum int
	for _, c := range rom[0xA0:0xBD] {
		sum += int(c)
	}
	rom[0xBD] = byte(-sum - 0x19)
	os.WriteFile(filepath.Join(root, "grp-exq.gba"), rom, 0o644)
	writeTestZip(t, filepath.Join(root, "packed.zip"), map[string][]byte{"exq.gba": rom})

	if err := New(db).Scan(context.Background(), root, nil); err != nil {
		t.Fatal(err)
	}
	// Pretend these were scanned before cartridge headers were read.
	db.Exec(`UPDATE files SET disc_id = NULL, disc_title = NULL, last_scanned_at = 'old'`)
	db.Exec(`UPDATE archive_members SET disc_id = NULL, disc_title = NULL`)

	if err := New(db).Scan(context.Background(), root, nil); err != nil {
		t.Fatal(err)
	}
	raw, _ := db.GetFileByPath(filepath.Join(root, "grp-exq.gba"))
	if raw.DiscID.String != "AXQE" || raw.DiscTitle != "EXQUEST" || raw.LastScannedAt != "old" {
		t.Errorf("raw: %+v", raw)
	}
	z, _ := db.GetFileByPath(filepath.Join(root, "packed.zip"))
	members, _ := db.ListArchiveMembers(z.ID)
	if len(members) != 1 || members[0].DiscID.String != "AXQE" {
		t.Errorf("member: %+v", members)
	}
}

// writeOldMethodZip writes a zip whose members claim PKZIP method 6 (implode):
// the bytes are stored, so only the header is what Go cannot decompress.
func writeOldMethodZip(t *testing.T, path string, files map[string][]byte) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	zw := zip.NewWriter(f)
	zw.RegisterCompressor(6, func(w io.Writer) (io.WriteCloser, error) { return nopCloser{w}, nil })
	for name, data := range files {
		w, err := zw.CreateHeader(&zip.FileHeader{Name: name, Method: 6})
		if err != nil {
			t.Fatal(err)
		}
		w.Write(data)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
}

type nopCloser struct{ io.Writer }

func (nopCloser) Close() error { return nil }

func TestScanRecordsTheOldZipMethod(t *testing.T) {
	db := openTestDB(t)
	root := t.TempDir()
	writeOldMethodZip(t, filepath.Join(root, "old.zip"), map[string][]byte{"game.prg": []byte("p")})
	writeTestZip(t, filepath.Join(root, "fine.zip"), map[string][]byte{"game.prg": []byte("p")})
	if err := New(db).Scan(context.Background(), root, nil); err != nil {
		t.Fatal(err)
	}
	for name, want := range map[string]string{"old.zip": "implode", "fine.zip": ""} {
		f, _ := db.GetFileByPath(filepath.Join(root, name))
		if !f.ZipMethod.Valid || f.ZipMethod.String != want {
			t.Errorf("%s: zip_method = %+v, want %q", name, f.ZipMethod, want)
		}
	}
}

func TestZipMethodIsFilledInForZipsScannedBefore(t *testing.T) {
	db := openTestDB(t)
	root := t.TempDir()
	writeOldMethodZip(t, filepath.Join(root, "old.zip"), map[string][]byte{"game.prg": []byte("p")})
	if err := New(db).Scan(context.Background(), root, nil); err != nil {
		t.Fatal(err)
	}
	db.Exec(`UPDATE files SET zip_method = NULL, last_scanned_at = 'old'`)
	if err := New(db).Scan(context.Background(), root, nil); err != nil {
		t.Fatal(err)
	}
	f, _ := db.GetFileByPath(filepath.Join(root, "old.zip"))
	if f.ZipMethod.String != "implode" || f.LastScannedAt != "old" {
		t.Errorf("zip_method = %+v, last_scanned_at = %q (must be filled without a rehash)", f.ZipMethod, f.LastScannedAt)
	}
}

// A member packed with a method Go cannot read keeps the archive scannable:
// its CRC comes from the central directory, only the header reads are skipped.
func TestOldMethodMembersDoNotFailTheArchive(t *testing.T) {
	db := openTestDB(t)
	root := t.TempDir()
	writeOldMethodZip(t, filepath.Join(root, "old.zip"), map[string][]byte{"game.gba": []byte("rom"), "game.nes": []byte("nes")})
	s := New(db)
	if err := s.Scan(context.Background(), root, nil); err != nil {
		t.Fatal(err)
	}
	if len(s.Problems) != 0 {
		t.Fatalf("problems: %+v", s.Problems)
	}
	f, _ := db.GetFileByPath(filepath.Join(root, "old.zip"))
	members, _ := db.ListArchiveMembers(f.ID)
	if len(members) != 2 {
		t.Fatalf("members: %+v", members)
	}
	for _, m := range members {
		if m.AltCRC32 != "" || m.MemberName == "game.gba" && (!m.DiscID.Valid || m.DiscID.String != "") {
			t.Errorf("%s: %+v", m.MemberName, m)
		}
	}
}
