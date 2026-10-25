package store

import (
	"database/sql"
	"fmt"
)

// SerialEntry is one serial and the name of its game.
type SerialEntry struct {
	Serial string
	Name   string
}

// ReplaceSerials stores the serial list of systemID, replacing the previous one.
// A serial listed twice keeps its first name.
func (db *DB) ReplaceSerials(systemID, version string, entries []SerialEntry) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`DELETE FROM serials WHERE system_id = ?`, systemID); err != nil {
		return err
	}
	stmt, err := tx.Prepare(`INSERT OR IGNORE INTO serials (system_id, serial, name) VALUES (?, ?, ?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for _, e := range entries {
		if _, err := stmt.Exec(systemID, e.Serial, e.Name); err != nil {
			return fmt.Errorf("store serial %s: %w", e.Serial, err)
		}
	}
	if _, err := tx.Exec(`INSERT INTO serial_lists (system_id, version) VALUES (?, ?)
		ON CONFLICT (system_id) DO UPDATE SET version = excluded.version, fetched_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now')`,
		systemID, version); err != nil {
		return err
	}
	return tx.Commit()
}

// SerialName returns the name of the game with this serial ("SLUS-00594"). A
// list entry with a suffix ("SLUS-20265GH") counts when there is no exact one.
func (db *DB) SerialName(systemID, serial string) (string, bool, error) {
	var name string
	err := db.QueryRow(`SELECT name FROM serials WHERE system_id = ? AND (serial = ? OR serial LIKE ? || '%')
		ORDER BY serial = ? DESC, serial LIMIT 1`, systemID, serial, serial, serial).Scan(&name)
	if err == sql.ErrNoRows {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return name, true, nil
}
