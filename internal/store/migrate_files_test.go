package store

import (
	"database/sql"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	_ "modernc.org/sqlite"
)

// Migration 0010 rebuilds the files table (to allow the 'rar' kind) while other
// tables point at it. Their rows must survive.
func TestFilesRebuildKeepsRowsThatPointAtFiles(t *testing.T) {
	db, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "old.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(`PRAGMA foreign_keys = ON`); err != nil {
		t.Fatal(err)
	}

	// Bring the database to schema 9, as an existing install would be.
	if _, err := db.Exec(`CREATE TABLE schema_migrations (version INTEGER PRIMARY KEY, applied_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')))`); err != nil {
		t.Fatal(err)
	}
	entries, _ := fs.ReadDir(migrationFiles, "migrations")
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name())
	}
	sort.Strings(names)
	for i, n := range names {
		if strings.HasPrefix(n, "0010_") {
			break
		}
		b, _ := migrationFiles.ReadFile("migrations/" + n)
		if _, err := db.Exec(string(b)); err != nil {
			t.Fatalf("%s: %v", n, err)
		}
		db.Exec(`INSERT INTO schema_migrations (version) VALUES (?)`, i+1)
	}

	for _, q := range []string{
		`INSERT INTO files (id, path, size, mtime_unix, kind) VALUES (7, '/r/a.7z', 10, 1, 'sevenzip'), (8, '/r/b.chd', 20, 2, 'raw')`,
		`INSERT INTO archive_members (file_id, member_name, size, crc32) VALUES (7, 'a.gb', 10, 'deadbeef')`,
		`INSERT INTO chd_checked (file_id, size, mtime_unix) VALUES (8, 20, 2)`,
		`INSERT INTO chd_tracks (file_id, ordinal, name, size, crc32, sha1, md5) VALUES (8, 0, 't', 1, 'a', 'b', 'c')`,
	} {
		if _, err := db.Exec(q); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}

	if err := migrate(db); err != nil {
		t.Fatal(err)
	}

	count := func(q string) (n int) {
		if err := db.QueryRow(q).Scan(&n); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
		return n
	}
	if n := count(`SELECT COUNT(*) FROM files`); n != 2 {
		t.Errorf("files = %d, want 2", n)
	}
	if n := count(`SELECT COUNT(*) FROM archive_members`); n != 1 {
		t.Errorf("archive_members = %d, want 1 (a cascade deleted them)", n)
	}
	if n := count(`SELECT COUNT(*) FROM chd_tracks`) + count(`SELECT COUNT(*) FROM chd_checked`); n != 2 {
		t.Errorf("chd rows = %d, want 2", n)
	}
	if _, err := db.Exec(`INSERT INTO files (path, size, mtime_unix, kind) VALUES ('/r/c.rar', 1, 1, 'rar')`); err != nil {
		t.Errorf("a rar file must be accepted now: %v", err)
	}
	if n := count(`PRAGMA foreign_keys`); n != 1 {
		t.Errorf("foreign_keys = %d after migrating, want it back on", n)
	}
	// And deleting a file still cascades to its members.
	db.Exec(`DELETE FROM files WHERE id = 7`)
	if n := count(`SELECT COUNT(*) FROM archive_members`); n != 0 {
		t.Errorf("cascade lost after the rebuild: %d members left", n)
	}
}

// Migration 0012 makes the next scan re-read files that may carry a copier
// header (so their headerless CRC gets recorded), and leaves the rest cached.
func TestHeaderlessMigrationForgetsOnlyHeaderableFiles(t *testing.T) {
	db, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "old.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	db.Exec(`PRAGMA foreign_keys = ON`)
	db.Exec(`CREATE TABLE schema_migrations (version INTEGER PRIMARY KEY, applied_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')))`)
	entries, _ := fs.ReadDir(migrationFiles, "migrations")
	for i, e := range entries {
		if strings.HasPrefix(e.Name(), "0012_") {
			break
		}
		b, _ := migrationFiles.ReadFile("migrations/" + e.Name())
		if _, err := db.Exec(string(b)); err != nil {
			t.Fatalf("%s: %v", e.Name(), err)
		}
		db.Exec(`INSERT INTO schema_migrations (version) VALUES (?)`, i+1)
	}
	for _, q := range []string{
		`INSERT INTO files (id, path, size, mtime_unix, kind) VALUES (1, '/r/Mario (USA).NES', 1, 1, 'raw'), (2, '/r/set.zip', 1, 1, 'zip'), (3, '/r/Puzzle.gb', 1, 1, 'raw'), (4, '/r/gb.zip', 1, 1, 'zip')`,
		`INSERT INTO archive_members (file_id, member_name, size, crc32) VALUES (2, 'Zelda (USA).nes', 1, 'aa'), (4, 'Puzzle.gb', 1, 'bb')`,
	} {
		if _, err := db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	if err := migrate(db); err != nil {
		t.Fatal(err)
	}
	var left []string
	rows, _ := db.Query(`SELECT path FROM files ORDER BY id`)
	for rows.Next() {
		var p string
		rows.Scan(&p)
		left = append(left, p)
	}
	rows.Close()
	if strings.Join(left, ",") != "/r/Puzzle.gb,/r/gb.zip" {
		t.Errorf("files left = %v", left)
	}
}
