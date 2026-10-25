package dat

import (
	"path/filepath"
	"strings"
	"testing"

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

func TestImport(t *testing.T) {
	db := openTestDB(t)

	df, err := Parse(strings.NewReader(logiqxPC))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	datID, err := Import(db, df, "nes", "/dats/nes-pc.dat")
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	if datID == 0 {
		t.Fatal("Import returned dat id 0")
	}

	var gotFormat, gotSourcePath string
	var gotHasPC bool
	err = db.QueryRow(`SELECT format, source_path, has_parent_clone FROM dats WHERE id = ?`, datID).
		Scan(&gotFormat, &gotSourcePath, &gotHasPC)
	if err != nil {
		t.Fatalf("query dats: %v", err)
	}
	if gotFormat != string(FormatLogiqxXML) || gotSourcePath != "/dats/nes-pc.dat" || !gotHasPC {
		t.Errorf("dats row = (format=%s, source_path=%s, has_parent_clone=%v), unexpected", gotFormat, gotSourcePath, gotHasPC)
	}

	var entryCount int
	if err := db.QueryRow(`SELECT COUNT(*) FROM dat_entries WHERE dat_id = ?`, datID).Scan(&entryCount); err != nil {
		t.Fatalf("count dat_entries: %v", err)
	}
	if entryCount != 2 {
		t.Errorf("dat_entries count = %d, want 2", entryCount)
	}

	var cloneOf string
	err = db.QueryRow(`SELECT clone_of FROM dat_entries WHERE dat_id = ? AND game_name = ?`, datID, "Super Game (Japan)").
		Scan(&cloneOf)
	if err != nil {
		t.Fatalf("query clone entry: %v", err)
	}
	if cloneOf != "Super Game (USA)" {
		t.Errorf("clone_of = %q, want %q", cloneOf, "Super Game (USA)")
	}
}

func TestImportNonPCHasNullCloneOf(t *testing.T) {
	db := openTestDB(t)

	df, err := Parse(strings.NewReader(logiqxNonPC))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	datID, err := Import(db, df, "nes", "/dats/nes.dat")
	if err != nil {
		t.Fatalf("Import: %v", err)
	}

	var cloneOf *string
	err = db.QueryRow(`SELECT clone_of FROM dat_entries WHERE dat_id = ? AND game_name = ?`, datID, "Super Game (USA)").
		Scan(&cloneOf)
	if err != nil {
		t.Fatalf("query entry: %v", err)
	}
	if cloneOf != nil {
		t.Errorf("clone_of = %q, want NULL", *cloneOf)
	}
}

func TestImportStoresSerials(t *testing.T) {
	db := openTestDB(t)
	df, err := Parse(strings.NewReader(`<datafile><header><name>GBA</name></header>
	<game name="Example Quest (USA, Europe)"><rom name="a.gba" size="8" crc="a1b2c3d4" serial="AXQE"/></game>
	<game name="Plain (USA)"><rom name="b.gba" size="8" crc="a1b2c3d5"/></game></datafile>`))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Import(db, df, "gba", "gba.dat"); err != nil {
		t.Fatal(err)
	}
	var withSerial, without int
	db.QueryRow(`SELECT COUNT(*) FROM dat_entries WHERE system_id = 'gba' AND serial = 'AXQE'`).Scan(&withSerial)
	db.QueryRow(`SELECT COUNT(*) FROM dat_entries WHERE serial IS NULL`).Scan(&without)
	if withSerial != 1 || without != 1 {
		t.Errorf("with serial %d, without %d; want 1 and 1", withSerial, without)
	}
}
