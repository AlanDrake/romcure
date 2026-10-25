package store

import "testing"

func TestSerialNames(t *testing.T) {
	db := openTestDB(t)
	err := db.ReplaceSerials("ps2", "v1", []SerialEntry{
		{"SLES-12345", "Example Kart (Europe)"},
		{"SLUS-20265GH", "Some Game (USA) (Greatest Hits)"},
		{"SLES-12345", "Duplicate (ignored)"},
	})
	if err != nil {
		t.Fatal(err)
	}
	for serial, want := range map[string]string{"SLES-12345": "Example Kart (Europe)", "SLUS-20265": "Some Game (USA) (Greatest Hits)"} {
		if got, ok, err := db.SerialName("ps2", serial); err != nil || !ok || got != want {
			t.Errorf("%s: %q %v %v", serial, got, ok, err)
		}
	}
	if _, ok, _ := db.SerialName("psx", "SLES-12345"); ok {
		t.Error("lists are per system")
	}
	// Replacing drops the old list.
	db.ReplaceSerials("ps2", "v2", []SerialEntry{{"SLUS-99999", "New"}})
	if _, ok, _ := db.SerialName("ps2", "SLES-12345"); ok {
		t.Error("the old list must be gone")
	}
}

// Every column of games that references another table needs an index: without
// one, each deleted file, archive member or DAT entry scans all games (a
// one-minute migration on a 14,000-game library, slow DAT replacement).
func TestGamesReferencesAreIndexed(t *testing.T) {
	db := openTestDB(t)
	indexed := map[string]bool{}
	rows, err := db.Query(`SELECT ii.name FROM pragma_index_list('games') il, pragma_index_info(il.name) ii WHERE ii.seqno = 0`)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var col string
		rows.Scan(&col)
		indexed[col] = true
	}
	rows.Close()
	fks, _ := db.Query(`SELECT "from" FROM pragma_foreign_key_list('games')`)
	for fks.Next() {
		var col string
		fks.Scan(&col)
		if !indexed[col] {
			t.Errorf("games.%s references another table but has no index", col)
		}
	}
	fks.Close()
}
