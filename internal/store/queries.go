package store

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"strings"
)

// FileRecord mirrors a row in the files table (schema §0001_initial.sql).
type FileRecord struct {
	ID            int64
	Path          string
	Size          int64
	MtimeUnix     int64
	Kind          string // "raw" | "zip" | "sevenzip" | "rar"
	CRC32         sql.NullString
	SHA1          sql.NullString
	MD5           sql.NullString
	LastScannedAt string
	// AltCRC32 and AltSize describe the data after a copier header (romheader);
	// "" and 0 when the file has none.
	AltCRC32 string
	AltSize  int64
	// DiscID and DiscTitle come from a GameCube/Wii disc header (discid).
	// DiscID is NULL until looked at, "" when the file has no such header.
	DiscID     sql.NullString
	DiscTitle  string
	DiscNumber int // 1-based; 0 when unknown
	// ZipMethod is the first compression method in a zip that Go cannot read
	// ("implode"), "" when there is none, NULL until looked at (see migration 0016).
	ZipMethod sql.NullString
}

// GetFileByPath returns the cached record for path, or nil if there isn't
// one yet.
func (db *DB) GetFileByPath(path string) (*FileRecord, error) {
	var rec FileRecord
	err := db.QueryRow(
		`SELECT id, path, size, mtime_unix, kind, crc32, sha1, md5, last_scanned_at, COALESCE(alt_crc32, ''), COALESCE(alt_size, 0), disc_id, COALESCE(disc_title, ''), COALESCE(disc_number, 0), zip_method
		 FROM files WHERE path = ?`, path,
	).Scan(&rec.ID, &rec.Path, &rec.Size, &rec.MtimeUnix, &rec.Kind, &rec.CRC32, &rec.SHA1, &rec.MD5, &rec.LastScannedAt, &rec.AltCRC32, &rec.AltSize, &rec.DiscID, &rec.DiscTitle, &rec.DiscNumber, &rec.ZipMethod)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get file %s: %w", path, err)
	}
	return &rec, nil
}

// UpsertFile inserts or replaces the files row for rec.Path, refreshing
// last_scanned_at, and returns its id. Replace-on-conflict is safe here
// because archive_members cascades from files.id (ON DELETE CASCADE) and
// callers always re-supply the full set of archive_members immediately
// after via ReplaceArchiveMembers when the file is an archive.
func (db *DB) UpsertFile(rec FileRecord) (int64, error) {
	// RETURNING, not LastInsertId: last_insert_rowid() only updates on an
	// actual INSERT. When ON CONFLICT resolves to the UPDATE branch (a
	// re-scan of an already-known path), LastInsertId silently returns a
	// stale id from whatever this connection's last real INSERT was —
	// not 0, not an error — so it can't be used to detect which branch
	// ran. RETURNING reports the real id either way.
	var id int64
	err := db.QueryRow(
		`INSERT INTO files (path, size, mtime_unix, kind, crc32, sha1, md5, alt_crc32, alt_size, disc_id, disc_title, disc_number, zip_method, last_scanned_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, NULLIF(?, ''), NULLIF(?, 0), ?, NULLIF(?, ''), NULLIF(?, 0), ?, strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
		 ON CONFLICT (path) DO UPDATE SET
		   size = excluded.size,
		   mtime_unix = excluded.mtime_unix,
		   kind = excluded.kind,
		   crc32 = excluded.crc32,
		   sha1 = excluded.sha1,
		   md5 = excluded.md5,
		   alt_crc32 = excluded.alt_crc32,
		   alt_size = excluded.alt_size,
		   disc_id = excluded.disc_id,
		   disc_title = excluded.disc_title,
		   disc_number = excluded.disc_number,
		   zip_method = excluded.zip_method,
		   last_scanned_at = excluded.last_scanned_at
		 RETURNING id`,
		rec.Path, rec.Size, rec.MtimeUnix, rec.Kind, rec.CRC32, rec.SHA1, rec.MD5, rec.AltCRC32, rec.AltSize, rec.DiscID, rec.DiscTitle, rec.DiscNumber, rec.ZipMethod,
	).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("upsert file %s: %w", rec.Path, err)
	}
	return id, nil
}

// DatRecord mirrors a row in the dats table.
type DatRecord struct {
	ID             int64
	SystemID       string
	Format         string
	SourcePath     string
	HasParentClone bool
	ImportedAt     string
}

// InsertDat inserts a new dats row and returns its id. Each DAT import
// gets its own row, even re-importing the same file — callers that want
// to replace a prior import should delete it first (dat_entries cascades).
func (db *DB) InsertDat(rec DatRecord) (int64, error) {
	res, err := db.Exec(
		`INSERT INTO dats (system_id, format, source_path, has_parent_clone)
		 VALUES (?, ?, ?, ?)`,
		rec.SystemID, rec.Format, rec.SourcePath, rec.HasParentClone,
	)
	if err != nil {
		return 0, fmt.Errorf("insert dat %s: %w", rec.SourcePath, err)
	}
	return res.LastInsertId()
}

// DatEntryRecord mirrors a row in the dat_entries table.
type DatEntryRecord struct {
	DatID    int64
	SystemID string
	GameName string
	RomName  string
	Size     int64
	CRC32    sql.NullString
	SHA1     sql.NullString
	MD5      sql.NullString
	CloneOf  sql.NullString
	Status   sql.NullString
	Serial   sql.NullString // cartridge game code, see the dat package
}

// InsertDatEntries inserts entries in one transaction.
func (db *DB) InsertDatEntries(entries []DatEntryRecord) error {
	if len(entries) == 0 {
		return nil
	}

	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin insert dat_entries: %w", err)
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(
		`INSERT INTO dat_entries (dat_id, system_id, game_name, rom_name, size, crc32, sha1, md5, clone_of, status, serial)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
	)
	if err != nil {
		return fmt.Errorf("prepare insert dat_entries: %w", err)
	}
	defer stmt.Close()

	for _, e := range entries {
		if _, err := stmt.Exec(e.DatID, e.SystemID, e.GameName, e.RomName, e.Size, e.CRC32, e.SHA1, e.MD5, e.CloneOf, e.Status, e.Serial); err != nil {
			return fmt.Errorf("insert dat_entry %s/%s: %w", e.GameName, e.RomName, err)
		}
	}

	return tx.Commit()
}

// ArchiveMemberRecord mirrors a row in the archive_members table. FileID
// is populated by ListArchiveMembers; it's ignored by ReplaceArchiveMembers
// since that already takes fileID as a parameter.
type ArchiveMemberRecord struct {
	FileID       int64
	MemberName   string
	Size         int64
	CRC32        string
	SHA1         sql.NullString
	IsOnlyMember bool
	AltCRC32     string // the CRC after a copier header; "" when there is none
	AltSize      int64
	DiscID       sql.NullString // see FileRecord
	DiscTitle    string
	DiscNumber   int
}

// ReplaceArchiveMembers deletes any existing archive_members for fileID
// and inserts members, inside one transaction. Called whenever a zip/7z's
// contents are (re)read — e.g. the file changed size/mtime since the last
// scan.
func (db *DB) ReplaceArchiveMembers(fileID int64, members []ArchiveMemberRecord) error {
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin replace archive_members for file %d: %w", fileID, err)
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`DELETE FROM archive_members WHERE file_id = ?`, fileID); err != nil {
		return fmt.Errorf("clear archive_members for file %d: %w", fileID, err)
	}

	stmt, err := tx.Prepare(
		`INSERT INTO archive_members (file_id, member_name, size, crc32, sha1, is_only_member, alt_crc32, alt_size, disc_id, disc_title, disc_number)
		 VALUES (?, ?, ?, ?, ?, ?, NULLIF(?, ''), NULLIF(?, 0), ?, NULLIF(?, ''), NULLIF(?, 0))`,
	)
	if err != nil {
		return fmt.Errorf("prepare insert archive_members: %w", err)
	}
	defer stmt.Close()

	for _, m := range members {
		if _, err := stmt.Exec(fileID, m.MemberName, m.Size, m.CRC32, m.SHA1, m.IsOnlyMember, m.AltCRC32, m.AltSize, m.DiscID, m.DiscTitle, m.DiscNumber); err != nil {
			return fmt.Errorf("insert archive_member %s for file %d: %w", m.MemberName, fileID, err)
		}
	}

	return tx.Commit()
}

// ListFilesUnderRoot returns every files row whose path is root itself or
// begins with root followed by a path separator — i.e. everything a scan
// of root would have populated. Used by the library browser (spec §2.6)
// to scope matching to one source root.
func (db *DB) ListFilesUnderRoot(root string) ([]FileRecord, error) {
	// Escape the separator along with root itself (not append it
	// afterward): on Windows the separator is '\', the same character as
	// the LIKE ESCAPE char, so an unescaped trailing separator would
	// escape the wildcard that follows it instead of matching literally.
	rawPrefix := root + string(filepath.Separator)
	prefix := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(rawPrefix) + "%"

	rows, err := db.Query(
		`SELECT id, path, size, mtime_unix, kind, crc32, sha1, md5, last_scanned_at, COALESCE(alt_crc32, ''), COALESCE(alt_size, 0), disc_id, COALESCE(disc_title, ''), COALESCE(disc_number, 0), zip_method
		 FROM files WHERE path = ? OR path LIKE ? ESCAPE '\'`,
		root, prefix,
	)
	if err != nil {
		return nil, fmt.Errorf("list files under %s: %w", root, err)
	}
	defer rows.Close()

	var out []FileRecord
	for rows.Next() {
		var rec FileRecord
		if err := rows.Scan(&rec.ID, &rec.Path, &rec.Size, &rec.MtimeUnix, &rec.Kind, &rec.CRC32, &rec.SHA1, &rec.MD5, &rec.LastScannedAt, &rec.AltCRC32, &rec.AltSize, &rec.DiscID, &rec.DiscTitle, &rec.DiscNumber, &rec.ZipMethod); err != nil {
			return nil, fmt.Errorf("scan file row: %w", err)
		}
		out = append(out, rec)
	}
	return out, rows.Err()
}

// ListArchiveMembers returns every archive_members row for fileID.
func (db *DB) ListArchiveMembers(fileID int64) ([]ArchiveMemberRecord, error) {
	rows, err := db.Query(
		`SELECT file_id, member_name, size, crc32, sha1, is_only_member, COALESCE(alt_crc32, ''), COALESCE(alt_size, 0), disc_id, COALESCE(disc_title, ''), COALESCE(disc_number, 0)
		 FROM archive_members WHERE file_id = ?`,
		fileID,
	)
	if err != nil {
		return nil, fmt.Errorf("list archive_members for file %d: %w", fileID, err)
	}
	defer rows.Close()

	var out []ArchiveMemberRecord
	for rows.Next() {
		var rec ArchiveMemberRecord
		if err := rows.Scan(&rec.FileID, &rec.MemberName, &rec.Size, &rec.CRC32, &rec.SHA1, &rec.IsOnlyMember, &rec.AltCRC32, &rec.AltSize, &rec.DiscID, &rec.DiscTitle, &rec.DiscNumber); err != nil {
			return nil, fmt.Errorf("scan archive_member row: %w", err)
		}
		out = append(out, rec)
	}
	return out, rows.Err()
}

// discExts is the SQL test for names that may hold a disc image or a cartridge dump
// (discid.MayHaveAny); col is the column with the name.
func discExts(col string) string {
	var parts []string
	for _, e := range []string{".iso", ".gcm", ".rvz", ".wia", ".wbfs", ".ciso", ".bin", ".img", ".gba", ".agb", ".nds", ".n64", ".v64", ".z64"} {
		parts = append(parts, "lower("+col+") LIKE '%"+e+"'")
	}
	return "(" + strings.Join(parts, " OR ") + ")"
}

// DiscHeadersPending lists the scanned files whose disc header (or that of an
// archive member) has not been looked at yet, so a scan can read just that
// without rehashing them.
func (db *DB) DiscHeadersPending() (map[string]bool, error) {
	rows, err := db.Query(`SELECT path FROM files WHERE disc_id IS NULL AND kind = 'raw' AND ` + discExts("path") + `
		UNION SELECT f.path FROM files f JOIN archive_members m ON m.file_id = f.id
		WHERE m.disc_id IS NULL AND ` + discExts("m.member_name"))
	if err != nil {
		return nil, fmt.Errorf("disc headers pending: %w", err)
	}
	defer rows.Close()
	out := map[string]bool{}
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err != nil {
			return nil, err
		}
		out[p] = true
	}
	return out, rows.Err()
}

// SetFileDisc records what a raw file's disc header says ("" = no header).
func (db *DB) SetFileDisc(fileID int64, discID, title string, number int) error {
	_, err := db.Exec(`UPDATE files SET disc_id = ?, disc_title = NULLIF(?, ''), disc_number = NULLIF(?, 0) WHERE id = ?`, discID, title, number, fileID)
	return err
}

// SetMemberDisc records what an archive member's disc header says.
func (db *DB) SetMemberDisc(fileID int64, member, discID, title string, number int) error {
	_, err := db.Exec(`UPDATE archive_members SET disc_id = ?, disc_title = NULLIF(?, ''), disc_number = NULLIF(?, 0) WHERE file_id = ? AND member_name = ?`, discID, title, number, fileID, member)
	return err
}

// DeleteFile removes a files row; archive_members and chd_tracks cascade.
func (db *DB) DeleteFile(id int64) error {
	_, err := db.Exec(`DELETE FROM files WHERE id = ?`, id)
	return err
}

// DatSerialEntry is a DAT game that lists a given serial.
type DatSerialEntry struct {
	GameName string
	Size     int64
}

// DatEntriesBySerial lists the DAT entries of systemID that carry serial (the
// header game code of a cartridge), one per ROM.
func (db *DB) DatEntriesBySerial(systemID, serial string) ([]DatSerialEntry, error) {
	rows, err := db.Query(`SELECT game_name, size FROM dat_entries WHERE system_id = ? AND serial = ?`, systemID, serial)
	if err != nil {
		return nil, fmt.Errorf("dat entries by serial %s/%s: %w", systemID, serial, err)
	}
	defer rows.Close()
	var out []DatSerialEntry
	for rows.Next() {
		var e DatSerialEntry
		if err := rows.Scan(&e.GameName, &e.Size); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// ZipMethodPending lists the scanned zips whose compression methods have not
// been looked at yet (scanned before migration 0016).
func (db *DB) ZipMethodPending() ([]string, error) {
	rows, err := db.Query(`SELECT path FROM files WHERE kind = 'zip' AND zip_method IS NULL`)
	if err != nil {
		return nil, fmt.Errorf("zip methods pending: %w", err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// SetFileZipMethod records a zip's first unreadable compression method ("" = none).
func (db *DB) SetFileZipMethod(fileID int64, method string) error {
	_, err := db.Exec(`UPDATE files SET zip_method = ? WHERE id = ?`, method, fileID)
	return err
}

// ZipMethodName is the PKZIP name of a compression method code.
func ZipMethodName(m uint16) string {
	switch m {
	case 1:
		return "shrink"
	case 2, 3, 4, 5:
		return "reduce"
	case 6:
		return "implode"
	case 9:
		return "deflate64"
	case 12:
		return "bzip2"
	case 14:
		return "lzma"
	}
	return fmt.Sprintf("method %d", m)
}
