package store

import (
	"database/sql"
	"embed"
	"fmt"
	"io/fs"
	"sort"
	"strings"
)

//go:embed migrations/*.sql
var migrationFiles embed.FS

// migrate applies any migration files not yet recorded in schema_migrations,
// in filename order, each inside its own transaction. Re-running it against
// an already-migrated database is a no-op.
func migrate(db *sql.DB) error {
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (
		version    INTEGER PRIMARY KEY,
		applied_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
	)`); err != nil {
		return fmt.Errorf("create schema_migrations: %w", err)
	}

	entries, err := fs.ReadDir(migrationFiles, "migrations")
	if err != nil {
		return fmt.Errorf("read migrations dir: %w", err)
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })

	for _, entry := range entries {
		name := entry.Name()

		var version int
		if _, err := fmt.Sscanf(name, "%04d_", &version); err != nil {
			return fmt.Errorf("migration %s: filename must start with a 4-digit version: %w", name, err)
		}

		var alreadyApplied bool
		err := db.QueryRow(`SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE version = ?)`, version).Scan(&alreadyApplied)
		if err != nil {
			return fmt.Errorf("check migration %d applied: %w", version, err)
		}
		if alreadyApplied {
			continue
		}

		if err := applyMigration(db, name, version); err != nil {
			return err
		}
	}
	return nil
}

func applyMigration(db *sql.DB, filename string, version int) error {
	sqlBytes, err := migrationFiles.ReadFile("migrations/" + filename)
	if err != nil {
		return fmt.Errorf("read migration %s: %w", filename, err)
	}

	// A migration that rebuilds a table other tables point at must run with
	// foreign keys off (dropping the old table would otherwise cascade into
	// them); the pragma only works outside a transaction. The pool has a single
	// connection, so it applies to the migration below.
	rebuild := strings.HasPrefix(string(sqlBytes), "-- migrate: foreign_keys=off")
	if rebuild {
		if _, err := db.Exec(`PRAGMA foreign_keys = OFF`); err != nil {
			return fmt.Errorf("migration %s: %w", filename, err)
		}
		defer db.Exec(`PRAGMA foreign_keys = ON`)
	}

	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin migration %s: %w", filename, err)
	}
	defer tx.Rollback()

	if _, err := tx.Exec(string(sqlBytes)); err != nil {
		return fmt.Errorf("apply migration %s: %w", filename, err)
	}
	if rebuild {
		var table, parent string
		var rowid, fk sql.NullInt64
		rows, err := tx.Query(`PRAGMA foreign_key_check`)
		if err != nil {
			return fmt.Errorf("check migration %s: %w", filename, err)
		}
		bad := rows.Next()
		if bad {
			rows.Scan(&table, &rowid, &parent, &fk)
		}
		rows.Close()
		if bad {
			return fmt.Errorf("migration %s left a broken reference in %s", filename, table)
		}
	}
	if _, err := tx.Exec(`INSERT INTO schema_migrations (version) VALUES (?)`, version); err != nil {
		return fmt.Errorf("record migration %s: %w", filename, err)
	}
	return tx.Commit()
}
