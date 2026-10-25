package profile

import (
	"database/sql"
	"fmt"
	"hash/crc32"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"romcure/internal/store"
)

// A C64 game packed as several PC64 files is one game whose output is the
// archive itself, renamed: in a RetroArch folder and in a device export.
func TestC64ProgramArchiveIsLinkedWhole(t *testing.T) {
	root := t.TempDir()
	zipPath := filepath.Join(root, "Castle_Quest.Publisher.zip")
	writeZip(t, zipPath, map[string]string{"LOADER.P00": "loader", "00.P00": "d0", "01.P00": "d1"})
	db := openTestDB(t)
	syncSystem(t, db, root, "c64")

	var games, kind string
	db.QueryRow(`SELECT COUNT(*) FROM games WHERE system_id = 'c64' AND is_selected = 1`).Scan(&games)
	db.QueryRow(`SELECT ultimate_source_kind FROM games WHERE system_id = 'c64'`).Scan(&kind)
	if games != "1" || kind != "raw_file" {
		t.Fatalf("selected games = %s, kind = %s", games, kind)
	}

	out := t.TempDir()
	rep, err := GenerateRetroArch(db, "c64", Options{OutputRoot: out})
	if err != nil || len(rep.Errors) != 0 || len(rep.Linked) != 1 {
		t.Fatalf("err=%v rep=%+v", err, rep)
	}
	if filepath.Base(rep.Linked[0].Path) != "Castle Quest.zip" {
		t.Errorf("linked %s", rep.Linked[0].Path)
	}
	if readFileContent(t, rep.Linked[0].Path) != readFileContent(t, zipPath) {
		t.Error("the linked file is not the archive")
	}

	dev := t.TempDir()
	opts := DeviceExportOptions{Options: Options{OutputRoot: dev}, Frontend: FrontendESDE, Systems: []string{"c64"}}
	plan, reports, err := ExportToDevice(db, opts)
	if err != nil || !plan.OK() || len(reports["c64"].Errors) != 0 || len(reports["c64"].Linked) != 1 {
		t.Fatalf("export: plan=%+v reports=%+v err=%v", plan, reports, err)
	}
	st, err := os.Stat(reports["c64"].Linked[0].Path)
	if err != nil || st.Size() != fileSize(t, zipPath) || plan.TotalBytes != st.Size() {
		t.Errorf("exported %v err=%v plan=%+v", st, err, plan)
	}
}

// Sides of one game dumped by different people carry different TOSEC flags;
// they still make one playlist, named without them.
func TestSidesWithDifferentDumpFlagsMakeOneM3U(t *testing.T) {
	root := t.TempDir()
	sides := map[string]string{
		"Quest of Ages (1987)(Publisher)(Disk 1 of 2 Side A)[a4]":       "side-a",
		"Quest of Ages (1987)(Publisher)(Disk 2 of 2 Side B)[cr Group]": "side-b",
	}
	db := openTestDB(t)
	datID, err := db.InsertDat(store.DatRecord{SystemID: "c64", Format: "clrmamepro", SourcePath: "c64.dat"})
	if err != nil {
		t.Fatal(err)
	}
	for name, content := range sides {
		writeFile(t, filepath.Join(root, name+".d64"), content)
		err := db.InsertDatEntries([]store.DatEntryRecord{{
			DatID: datID, SystemID: "c64", GameName: name, RomName: name + ".d64", Size: int64(len(content)),
			CRC32: sql.NullString{String: fmt.Sprintf("%08x", crc32.ChecksumIEEE([]byte(content))), Valid: true},
		}})
		if err != nil {
			t.Fatal(err)
		}
	}
	syncSystem(t, db, root, "c64")

	out := t.TempDir()
	rep, err := GenerateRetroArch(db, "c64", Options{OutputRoot: out})
	if err != nil || len(rep.Errors) != 0 || len(rep.Linked) != 2 {
		t.Fatalf("err=%v rep=%+v", err, rep)
	}
	dbName, _ := LibretroDBName("c64")
	matches, _ := filepath.Glob(filepath.Join(out, dbName, "*.m3u"))
	if len(matches) != 1 || filepath.Base(matches[0]) != "Quest of Ages (1987)(Publisher).m3u" {
		t.Fatalf("playlists = %v", matches)
	}
	if got := strings.Count(readFileContent(t, matches[0]), "\n"); got != 2 {
		t.Errorf("the playlist lists %d lines, want 2:\n%s", got, readFileContent(t, matches[0]))
	}
}
