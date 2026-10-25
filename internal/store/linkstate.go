package store

import (
	"database/sql"
	"fmt"
)

// OutputKey is the profile_id that link_state records an output's files under.
func OutputKey(outputID int64) string { return fmt.Sprintf("p%d", outputID) }

// LinkStateRecord mirrors a row in the link_state table (spec §2.7).
type LinkStateRecord struct {
	ProfileID        string
	SystemID         string
	GameKey          string
	TargetPath       string
	SourceKind       string
	SourcePath       string
	SourceHash       string
	LinkType         string
	IsManualOverride bool
	DiscIndex        *int
	LinkStatus       string
}

// UpsertLinkState inserts or updates a link_state row keyed by
// (profile_id, target_path).
func (db *DB) UpsertLinkState(rec LinkStateRecord) error {
	_, err := db.Exec(
		`INSERT INTO link_state (profile_id, system_id, game_key, target_path, source_kind, source_path, source_hash, link_type, is_manual_override, disc_index, link_status, last_verified_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
		 ON CONFLICT (profile_id, target_path) DO UPDATE SET
		   system_id = excluded.system_id,
		   game_key = excluded.game_key,
		   source_kind = excluded.source_kind,
		   source_path = excluded.source_path,
		   source_hash = excluded.source_hash,
		   link_type = excluded.link_type,
		   is_manual_override = excluded.is_manual_override,
		   disc_index = excluded.disc_index,
		   link_status = excluded.link_status,
		   last_verified_at = excluded.last_verified_at`,
		rec.ProfileID, rec.SystemID, rec.GameKey, rec.TargetPath, rec.SourceKind, rec.SourcePath, rec.SourceHash,
		rec.LinkType, rec.IsManualOverride, rec.DiscIndex, rec.LinkStatus,
	)
	if err != nil {
		return fmt.Errorf("upsert link_state %s/%s: %w", rec.ProfileID, rec.TargetPath, err)
	}
	return nil
}

// GetLinkState returns the link_state row for (profileID, targetPath), if any.
func (db *DB) GetLinkState(profileID, targetPath string) (*LinkStateRecord, error) {
	var rec LinkStateRecord
	rec.ProfileID, rec.TargetPath = profileID, targetPath
	err := db.QueryRow(
		`SELECT system_id, game_key, source_kind, source_path, source_hash, link_type, is_manual_override, link_status
		 FROM link_state WHERE profile_id = ? AND target_path = ?`,
		profileID, targetPath,
	).Scan(&rec.SystemID, &rec.GameKey, &rec.SourceKind, &rec.SourcePath, &rec.SourceHash, &rec.LinkType, &rec.IsManualOverride, &rec.LinkStatus)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get link_state %s/%s: %w", profileID, targetPath, err)
	}
	return &rec, nil
}

// ListLinkStates returns every link_state row of a profile, ordered by
// target path.
func (db *DB) ListLinkStates(profileID string) ([]LinkStateRecord, error) {
	rows, err := db.Query(`SELECT profile_id, system_id, game_key, target_path, source_kind, source_path,
		source_hash, link_type, is_manual_override, disc_index, link_status
		FROM link_state WHERE profile_id = ? ORDER BY target_path`, profileID)
	if err != nil {
		return nil, fmt.Errorf("list link_state for %s: %w", profileID, err)
	}
	defer rows.Close()
	var out []LinkStateRecord
	for rows.Next() {
		var r LinkStateRecord
		var disc sql.NullInt64
		if err := rows.Scan(&r.ProfileID, &r.SystemID, &r.GameKey, &r.TargetPath, &r.SourceKind, &r.SourcePath,
			&r.SourceHash, &r.LinkType, &r.IsManualOverride, &disc, &r.LinkStatus); err != nil {
			return nil, err
		}
		if disc.Valid {
			d := int(disc.Int64)
			r.DiscIndex = &d
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// SetLinkStatus records the outcome of a verification and stamps
// last_verified_at.
func (db *DB) SetLinkStatus(profileID, targetPath, status string) error {
	_, err := db.Exec(`UPDATE link_state SET link_status = ?, last_verified_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
		WHERE profile_id = ? AND target_path = ?`, status, profileID, targetPath)
	return err
}

// DeleteLinkState forgets one recorded output.
func (db *DB) DeleteLinkState(profileID, targetPath string) error {
	_, err := db.Exec(`DELETE FROM link_state WHERE profile_id = ? AND target_path = ?`, profileID, targetPath)
	return err
}
