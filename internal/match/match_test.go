package match

import (
	"database/sql"
	"path/filepath"
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

func seedDatEntry(t *testing.T, db *store.DB, systemID, gameName, romName, crc32 string, size int64) {
	t.Helper()
	seedDatEntryWithStatus(t, db, systemID, gameName, romName, crc32, size, "")
}

func seedDatEntryWithStatus(t *testing.T, db *store.DB, systemID, gameName, romName, crc32 string, size int64, status string) {
	t.Helper()
	datID, err := db.InsertDat(store.DatRecord{SystemID: systemID, Format: "logiqx_xml", SourcePath: "/test.dat"})
	if err != nil {
		t.Fatalf("InsertDat: %v", err)
	}
	err = db.InsertDatEntries([]store.DatEntryRecord{{
		DatID: datID, SystemID: systemID, GameName: gameName, RomName: romName,
		Size: size, CRC32: sql.NullString{String: crc32, Valid: crc32 != ""},
		Status: sql.NullString{String: status, Valid: status != ""},
	}})
	if err != nil {
		t.Fatalf("InsertDatEntries: %v", err)
	}
}

func TestMatchByHash(t *testing.T) {
	db := openTestDB(t)
	seedDatEntry(t, db, "nes", "Super Game (USA)", "Super Game (USA).nes", "a1b2c3d4", 131088)

	t.Run("exact crc32 and size match", func(t *testing.T) {
		res, ok, err := MatchByHash(db, "nes", "a1b2c3d4", 131088)
		if err != nil {
			t.Fatalf("MatchByHash: %v", err)
		}
		if !ok {
			t.Fatal("ok = false, want true")
		}
		if res.Confidence != ConfidenceDATVerified {
			t.Errorf("Confidence = %v, want %v", res.Confidence, ConfidenceDATVerified)
		}
		if res.Title != "Super Game (USA)" {
			t.Errorf("Title = %q, want %q", res.Title, "Super Game (USA)")
		}
		if res.Region != "USA" {
			t.Errorf("Region = %q, want %q (derived from the DAT's own game name)", res.Region, "USA")
		}
		if !res.IsVerifiedDump {
			t.Error("IsVerifiedDump = false, want true (no baddump/nodump status)")
		}
	})

	t.Run("baddump status -> not a verified dump", func(t *testing.T) {
		seedDatEntryWithStatus(t, db, "nes", "Suspect Game (USA)", "Suspect Game (USA).nes", "f0f0f0f0", 42, "baddump")
		res, ok, err := MatchByHash(db, "nes", "f0f0f0f0", 42)
		if err != nil {
			t.Fatalf("MatchByHash: %v", err)
		}
		if !ok {
			t.Fatal("ok = false, want true")
		}
		if res.IsVerifiedDump {
			t.Error("IsVerifiedDump = true, want false (status=baddump)")
		}
	})

	t.Run("beta tag in DAT game name is detected", func(t *testing.T) {
		seedDatEntry(t, db, "nes", "Super Game (USA) (Beta)", "Super Game (USA) (Beta).nes", "cafe1234", 99)
		res, ok, err := MatchByHash(db, "nes", "cafe1234", 99)
		if err != nil {
			t.Fatalf("MatchByHash: %v", err)
		}
		if !ok {
			t.Fatal("ok = false, want true")
		}
		if !res.IsBeta {
			t.Error("IsBeta = false, want true")
		}
	})

	t.Run("right crc32, wrong size -> no match", func(t *testing.T) {
		_, ok, err := MatchByHash(db, "nes", "a1b2c3d4", 999)
		if err != nil {
			t.Fatalf("MatchByHash: %v", err)
		}
		if ok {
			t.Error("ok = true, want false (size mismatch should not match)")
		}
	})

	t.Run("right crc32 and size, wrong system -> no match", func(t *testing.T) {
		_, ok, err := MatchByHash(db, "snes", "a1b2c3d4", 131088)
		if err != nil {
			t.Fatalf("MatchByHash: %v", err)
		}
		if ok {
			t.Error("ok = true, want false (wrong system should not match)")
		}
	})

	t.Run("unknown crc32 -> no match", func(t *testing.T) {
		_, ok, err := MatchByHash(db, "nes", "deadbeef", 131088)
		if err != nil {
			t.Fatalf("MatchByHash: %v", err)
		}
		if ok {
			t.Error("ok = true, want false")
		}
	})
}

func TestMatchFile(t *testing.T) {
	db := openTestDB(t)
	seedDatEntry(t, db, "nes", "Super Game (USA)", "Super Game (USA).nes", "a1b2c3d4", 131088)

	t.Run("dat hit takes priority over heuristic", func(t *testing.T) {
		res, err := MatchFile(db, "nes", "a1b2c3d4", 131088, "Super Game (USA).nes")
		if err != nil {
			t.Fatalf("MatchFile: %v", err)
		}
		if res.Confidence != ConfidenceDATVerified {
			t.Errorf("Confidence = %v, want %v", res.Confidence, ConfidenceDATVerified)
		}
	})

	t.Run("dat miss falls back to heuristic", func(t *testing.T) {
		res, err := MatchFile(db, "nes", "deadbeef", 999, "Other Game (Europe).nes")
		if err != nil {
			t.Fatalf("MatchFile: %v", err)
		}
		if res.Confidence != ConfidenceHeuristic {
			t.Errorf("Confidence = %v, want %v", res.Confidence, ConfidenceHeuristic)
		}
		if res.Title != "Other Game" || res.Region != "Europe" {
			t.Errorf("Title/Region = %q/%q, want %q/%q", res.Title, res.Region, "Other Game", "Europe")
		}
	})

	t.Run("no crc32 (e.g. zero-DAT operation) goes straight to heuristic", func(t *testing.T) {
		res, err := MatchFile(db, "nes", "", 0, "Homebrew Thing.gb")
		if err != nil {
			t.Fatalf("MatchFile: %v", err)
		}
		if res.Confidence != ConfidenceUnidentified {
			t.Errorf("Confidence = %v, want %v", res.Confidence, ConfidenceUnidentified)
		}
	})
}
