package store

import (
	"database/sql"
	"fmt"

	_ "modernc.org/sqlite"
)

// DB is the app's SQLite handle. Open limits the pool to a single
// connection: SQLite serializes writers regardless, and a single
// connection means the per-connection PRAGMAs below apply uniformly
// without needing a connection-lifecycle hook.
type DB struct {
	*sql.DB
}

// Open opens (creating if necessary) the SQLite database at path and
// applies any pending migrations.
func Open(path string) (*DB, error) {
	sqlDB, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	sqlDB.SetMaxOpenConns(1)

	if _, err := sqlDB.Exec(`PRAGMA foreign_keys = ON`); err != nil {
		sqlDB.Close()
		return nil, fmt.Errorf("enable foreign_keys on %s: %w", path, err)
	}
	if _, err := sqlDB.Exec(`PRAGMA journal_mode = WAL`); err != nil {
		sqlDB.Close()
		return nil, fmt.Errorf("enable WAL on %s: %w", path, err)
	}

	if err := migrate(sqlDB); err != nil {
		sqlDB.Close()
		return nil, fmt.Errorf("migrate %s: %w", path, err)
	}

	return &DB{sqlDB}, nil
}
