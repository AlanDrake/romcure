package store

import "fmt"

// CHDTrack is one extracted, hashed track (or the ISO) of a CHD.
type CHDTrack struct {
	Ordinal int
	Name    string
	Size    int64
	CRC32   string
	SHA1    string
	MD5     string
}

// ReplaceCHDTracks stores the verified track hashes for the file, tagged
// with the size/mtime of the CHD they were computed from.
func (db *DB) ReplaceCHDTracks(fileID, size, mtimeUnix int64, tracks []CHDTrack) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`DELETE FROM chd_tracks WHERE file_id = ?`, fileID); err != nil {
		return err
	}
	for _, t := range tracks {
		if _, err := tx.Exec(`INSERT INTO chd_tracks (file_id, ordinal, name, size, crc32, sha1, md5) VALUES (?, ?, ?, ?, ?, ?, ?)`,
			fileID, t.Ordinal, t.Name, t.Size, t.CRC32, t.SHA1, t.MD5); err != nil {
			return fmt.Errorf("insert chd track: %w", err)
		}
	}
	if _, err := tx.Exec(`INSERT INTO chd_checked (file_id, size, mtime_unix) VALUES (?, ?, ?)
		ON CONFLICT(file_id) DO UPDATE SET size = excluded.size, mtime_unix = excluded.mtime_unix,
		checked_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now')`, fileID, size, mtimeUnix); err != nil {
		return err
	}
	return tx.Commit()
}

// CHDTracks returns the stored track hashes for the file, or nil when it
// was never checked or has changed (size/mtime) since.
func (db *DB) CHDTracks(f FileRecord) ([]CHDTrack, error) {
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM chd_checked WHERE file_id = ? AND size = ? AND mtime_unix = ?`,
		f.ID, f.Size, f.MtimeUnix).Scan(&n); err != nil || n == 0 {
		return nil, err
	}
	rows, err := db.Query(`SELECT ordinal, name, size, crc32, sha1, md5 FROM chd_tracks WHERE file_id = ? ORDER BY ordinal`, f.ID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []CHDTrack
	for rows.Next() {
		var t CHDTrack
		if err := rows.Scan(&t.Ordinal, &t.Name, &t.Size, &t.CRC32, &t.SHA1, &t.MD5); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}
