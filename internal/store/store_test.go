package store

import (
	"path/filepath"
	"testing"
)

var schemaV1Tables = []string{
	"dats",
	"files",
	"archive_members",
	"dat_entries",
	"game_groups",
	"games",
	"rules",
	"overrides",
	"link_state",
	"transcode_cache",
}

func openTestDB(t *testing.T) *DB {
	t.Helper()
	path := filepath.Join(t.TempDir(), "library.db")
	db, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestOpenCreatesSchemaV1(t *testing.T) {
	db := openTestDB(t)

	for _, table := range schemaV1Tables {
		var name string
		err := db.QueryRow(`SELECT name FROM sqlite_master WHERE type = 'table' AND name = ?`, table).Scan(&name)
		if err != nil {
			t.Errorf("table %q: %v", table, err)
		}
	}

	var version int
	if err := db.QueryRow(`SELECT MAX(version) FROM schema_migrations`).Scan(&version); err != nil {
		t.Fatalf("read schema_migrations: %v", err)
	}
	want := migrationFileCount(t)
	if version != want {
		t.Errorf("schema_migrations max version = %d, want %d (one per embedded migration file)", version, want)
	}
}

// migrationFileCount counts the embedded migration files directly,
// rather than hardcoding a count that would go stale every time a
// migration is added.
func migrationFileCount(t *testing.T) int {
	t.Helper()
	entries, err := migrationFiles.ReadDir("migrations")
	if err != nil {
		t.Fatalf("read embedded migrations dir: %v", err)
	}
	return len(entries)
}

func TestOpenIsIdempotent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "library.db")

	db1, err := Open(path)
	if err != nil {
		t.Fatalf("first Open: %v", err)
	}
	db1.Close()

	// Re-opening an already-migrated database must not error or re-apply
	// migrations (schema_migrations has a PRIMARY KEY on version, so a
	// re-apply would fail the INSERT).
	db2, err := Open(path)
	if err != nil {
		t.Fatalf("second Open: %v", err)
	}
	defer db2.Close()

	var count int
	if err := db2.QueryRow(`SELECT COUNT(*) FROM schema_migrations`).Scan(&count); err != nil {
		t.Fatalf("count schema_migrations: %v", err)
	}
	want := migrationFileCount(t)
	if count != want {
		t.Errorf("schema_migrations row count = %d, want %d (one per embedded migration file, not re-applied)", count, want)
	}
}

func TestForeignKeysEnforced(t *testing.T) {
	db := openTestDB(t)

	_, err := db.Exec(`INSERT INTO dat_entries (dat_id, system_id, game_name, rom_name, size) VALUES (999, 'nes', 'Example', 'Example.nes', 0)`)
	if err == nil {
		t.Fatal("expected a foreign key violation inserting dat_entries with a nonexistent dat_id, got nil error")
	}
}

func TestLinkStateUniqueConstraint(t *testing.T) {
	db := openTestDB(t)

	insert := `INSERT INTO link_state
		(profile_id, system_id, game_key, target_path, source_kind, source_path, source_hash, link_type, link_status)
		VALUES ('retroarch', 'nes', 'abc123', 'nes/Game.zip', 'raw_file', '/src/Game.zip', 'deadbeef', 'symlink_rel', 'ok')`

	if _, err := db.Exec(insert); err != nil {
		t.Fatalf("first insert: %v", err)
	}
	if _, err := db.Exec(insert); err == nil {
		t.Fatal("expected a unique constraint violation on duplicate (profile_id, target_path), got nil error")
	}
}

func TestListFilesUnderRoot(t *testing.T) {
	db := openTestDB(t)
	root := filepath.Join(t.TempDir(), "roms")
	sibling := root + "-sibling" // shares the "roms" prefix but is a different directory

	paths := []string{
		filepath.Join(root, "game.nes"),
		filepath.Join(root, "sub", "game2.nes"),
		filepath.Join(sibling, "game3.nes"),
	}
	for _, p := range paths {
		if _, err := db.UpsertFile(FileRecord{Path: p, Size: 1, MtimeUnix: 1, Kind: "raw"}); err != nil {
			t.Fatalf("UpsertFile(%s): %v", p, err)
		}
	}
	// root itself as a file (the "path = root" branch of the query).
	if _, err := db.UpsertFile(FileRecord{Path: root, Size: 1, MtimeUnix: 1, Kind: "raw"}); err != nil {
		t.Fatalf("UpsertFile(%s): %v", root, err)
	}

	got, err := db.ListFilesUnderRoot(root)
	if err != nil {
		t.Fatalf("ListFilesUnderRoot: %v", err)
	}

	gotPaths := make(map[string]bool)
	for _, rec := range got {
		gotPaths[rec.Path] = true
	}

	wantPresent := []string{root, paths[0], paths[1]}
	for _, p := range wantPresent {
		if !gotPaths[p] {
			t.Errorf("ListFilesUnderRoot(%s) missing %s", root, p)
		}
	}
	if gotPaths[paths[2]] {
		t.Errorf("ListFilesUnderRoot(%s) incorrectly included sibling path %s", root, paths[2])
	}
	if len(got) != len(wantPresent) {
		t.Errorf("len(got) = %d, want %d (got paths: %v)", len(got), len(wantPresent), gotPaths)
	}
}

// TestUpsertFileReturnsCorrectIDOnUpdatePath guards against a real bug:
// sqlite3_last_insert_rowid() only updates on an actual INSERT, so when
// ON CONFLICT resolves to the UPDATE branch, naively trusting
// res.LastInsertId() returns a stale id from whatever this connection's
// last real INSERT was (not 0, not an error) — in this test, file B's id
// instead of file A's. UpsertFile must use RETURNING instead.
func TestUpsertFileReturnsCorrectIDOnUpdatePath(t *testing.T) {
	db := openTestDB(t)

	idA, err := db.UpsertFile(FileRecord{Path: "/a", Size: 1, MtimeUnix: 1, Kind: "raw"})
	if err != nil {
		t.Fatalf("insert A: %v", err)
	}
	idB, err := db.UpsertFile(FileRecord{Path: "/b", Size: 1, MtimeUnix: 1, Kind: "raw"})
	if err != nil {
		t.Fatalf("insert B: %v", err)
	}
	if idA == idB {
		t.Fatalf("test setup invalid: A and B got the same id (%d)", idA)
	}

	// Re-upsert A (hits the UPDATE branch). Must return A's real id, not
	// B's (the last real INSERT on this connection).
	gotID, err := db.UpsertFile(FileRecord{Path: "/a", Size: 2, MtimeUnix: 2, Kind: "raw"})
	if err != nil {
		t.Fatalf("update A: %v", err)
	}
	if gotID != idA {
		t.Errorf("UpsertFile on the update path returned id %d, want %d (A's real id, not B's %d)", gotID, idA, idB)
	}
}
