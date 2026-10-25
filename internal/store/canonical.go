package store

import (
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
)

// GameGroupRecord mirrors a row in the game_groups table.
type GameGroupRecord struct {
	ID               int64
	SystemID         string
	GroupKey         string
	GroupingSource   string
	DisplayName      string
	IsManualOverride bool
	Excluded         bool
	ExclusionReason  string
	// SelectionReason explains the current pick (empty on the placeholder
	// upsert that runs before the pick is known).
	SelectionReason string
}

// UpsertGameGroup inserts or updates a game_groups row keyed by
// (system_id, group_key), preserving its id across re-syncs — and
// therefore keeping any override valid, since overrides reference this
// id, not the group_key directly.
func (db *DB) UpsertGameGroup(rec GameGroupRecord) (int64, error) {
	// RETURNING, not LastInsertId — see the comment on UpsertFile: it's
	// unreliable whenever ON CONFLICT resolves to an UPDATE, which is the
	// common case here (a group re-synced after its first creation).
	var id int64
	err := db.QueryRow(
		`INSERT INTO game_groups (system_id, group_key, grouping_source, display_name, is_manual_override, excluded, exclusion_reason, selection_reason, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
		 ON CONFLICT (system_id, group_key) DO UPDATE SET
		   grouping_source = excluded.grouping_source,
		   display_name = excluded.display_name,
		   is_manual_override = excluded.is_manual_override,
		   excluded = excluded.excluded,
		   exclusion_reason = excluded.exclusion_reason,
		   selection_reason = CASE WHEN excluded.selection_reason = '' THEN game_groups.selection_reason ELSE excluded.selection_reason END,
		   updated_at = excluded.updated_at
		 RETURNING id`,
		rec.SystemID, rec.GroupKey, rec.GroupingSource, rec.DisplayName, rec.IsManualOverride, rec.Excluded, rec.ExclusionReason, rec.SelectionReason,
	).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("upsert game_group %s/%s: %w", rec.SystemID, rec.GroupKey, err)
	}
	return id, nil
}

// GameRecord mirrors a row in the games table.
type GameRecord struct {
	ID         int64
	GroupID    int64
	SystemID   string
	GameKey    string
	DatEntryID sql.NullInt64
	Confidence string
	Region     string
	Language   string
	Revision   string

	IsVerifiedDump  bool
	IsBetaProtoDemo bool
	IsSelected      bool

	UltimateSourceKind string
	UltimateSourceRef  string
	// MemberName is the matched archive member's name within
	// UltimateSourceRef, when UltimateSourceKind == "archive". "" for a
	// raw file.
	MemberName string
}

// UpsertGame inserts or updates a games row keyed by (system_id, game_key).
func (db *DB) UpsertGame(rec GameRecord) (int64, error) {
	// RETURNING, not LastInsertId — see the comment on UpsertFile.
	var id int64
	err := db.QueryRow(
		`INSERT INTO games (group_id, system_id, game_key, dat_entry_id, confidence, region, language, revision, is_verified_dump, is_beta_proto_demo, is_selected, ultimate_source_kind, ultimate_source_ref, member_name)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT (system_id, game_key) DO UPDATE SET
		   group_id = excluded.group_id,
		   dat_entry_id = excluded.dat_entry_id,
		   confidence = excluded.confidence,
		   region = excluded.region,
		   language = excluded.language,
		   revision = excluded.revision,
		   is_verified_dump = excluded.is_verified_dump,
		   is_beta_proto_demo = excluded.is_beta_proto_demo,
		   is_selected = excluded.is_selected,
		   ultimate_source_kind = excluded.ultimate_source_kind,
		   ultimate_source_ref = excluded.ultimate_source_ref,
		   member_name = excluded.member_name
		 RETURNING id`,
		rec.GroupID, rec.SystemID, rec.GameKey, rec.DatEntryID, rec.Confidence,
		rec.Region, rec.Language, rec.Revision,
		rec.IsVerifiedDump, rec.IsBetaProtoDemo, rec.IsSelected,
		rec.UltimateSourceKind, rec.UltimateSourceRef, sql.NullString{String: rec.MemberName, Valid: rec.MemberName != ""},
	).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("upsert game %s/%s: %w", rec.SystemID, rec.GameKey, err)
	}
	return id, nil
}

// GamesInGroup returns every games row belonging to groupID.
func (db *DB) GamesInGroup(groupID int64) ([]GameRecord, error) {
	rows, err := db.Query(
		`SELECT id, group_id, system_id, game_key, dat_entry_id, confidence, region, language, revision,
		        is_verified_dump, is_beta_proto_demo, is_selected, ultimate_source_kind, ultimate_source_ref, member_name
		 FROM games WHERE group_id = ?`,
		groupID,
	)
	if err != nil {
		return nil, fmt.Errorf("list games in group %d: %w", groupID, err)
	}
	defer rows.Close()

	var out []GameRecord
	for rows.Next() {
		var rec GameRecord
		var memberName sql.NullString
		if err := rows.Scan(&rec.ID, &rec.GroupID, &rec.SystemID, &rec.GameKey, &rec.DatEntryID, &rec.Confidence,
			&rec.Region, &rec.Language, &rec.Revision, &rec.IsVerifiedDump, &rec.IsBetaProtoDemo, &rec.IsSelected,
			&rec.UltimateSourceKind, &rec.UltimateSourceRef, &memberName); err != nil {
			return nil, fmt.Errorf("scan game row: %w", err)
		}
		rec.MemberName = memberName.String
		out = append(out, rec)
	}
	return out, rows.Err()
}

// SelectedGames returns every selected (is_selected = 1) games row for
// systemID, joined with its group's display name — what a profile
// generator (spec §2, M3) needs to link each chosen game.
func (db *DB) SelectedGames(systemID string) ([]SelectedGame, error) {
	rows, err := db.Query(
		`SELECT g.game_key, g.group_id, g.system_id, gg.display_name, g.ultimate_source_kind, g.ultimate_source_ref, g.member_name
		 FROM games g
		 JOIN game_groups gg ON gg.id = g.group_id
		 WHERE g.system_id = ? AND g.is_selected = 1`,
		systemID,
	)
	if err != nil {
		return nil, fmt.Errorf("list selected games for %s: %w", systemID, err)
	}
	defer rows.Close()

	var out []SelectedGame
	for rows.Next() {
		var rec SelectedGame
		var memberName sql.NullString
		if err := rows.Scan(&rec.GameKey, &rec.GroupID, &rec.SystemID, &rec.DisplayName, &rec.UltimateSourceKind, &rec.UltimateSourceRef, &memberName); err != nil {
			return nil, fmt.Errorf("scan selected game row: %w", err)
		}
		rec.MemberName = memberName.String
		out = append(out, rec)
	}
	return out, rows.Err()
}

// SelectedGame is one row of SelectedGames' result.
type SelectedGame struct {
	GroupID            int64
	SystemID           string
	GameKey            string
	DisplayName        string
	UltimateSourceKind string
	UltimateSourceRef  string
	MemberName         string // "" for a raw file
}

// SetOverride records a manual override: within the group identified by
// groupID, force overrideGameKey to be the selected variant, replacing
// any existing override for that group (spec §2.1 — an override applies
// to all profiles at once, it's not a history log).
func (db *DB) SetOverride(groupID int64, overrideGameKey, reason string) error {
	_, err := db.Exec(
		`INSERT INTO overrides (group_id, override_game_key, reason)
		 VALUES (?, ?, ?)
		 ON CONFLICT (group_id) DO UPDATE SET
		   override_game_key = excluded.override_game_key,
		   reason = excluded.reason`,
		groupID, overrideGameKey, reason,
	)
	if err != nil {
		return fmt.Errorf("set override for group %d: %w", groupID, err)
	}
	return nil
}

// GetOverride returns the overridden game_key for groupID, if any.
func (db *DB) GetOverride(groupID int64) (gameKey string, reason string, ok bool, err error) {
	var reasonNS sql.NullString
	err = db.QueryRow(`SELECT override_game_key, reason FROM overrides WHERE group_id = ?`, groupID).Scan(&gameKey, &reasonNS)
	if err == sql.ErrNoRows {
		return "", "", false, nil
	}
	if err != nil {
		return "", "", false, fmt.Errorf("get override for group %d: %w", groupID, err)
	}
	return gameKey, reasonNS.String, true, nil
}

// ClearOverride removes any override for groupID.
func (db *DB) ClearOverride(groupID int64) error {
	if _, err := db.Exec(`DELETE FROM overrides WHERE group_id = ?`, groupID); err != nil {
		return fmt.Errorf("clear override for group %d: %w", groupID, err)
	}
	return nil
}

// DatGameName returns the DAT game name a game was matched to, or ok=false
// when it has no DAT match (heuristic/unidentified, or the DAT was since
// replaced and the game hasn't been re-synced yet).
func (db *DB) DatGameName(systemID, gameKey string) (name string, ok bool, err error) {
	err = db.QueryRow(`SELECT e.game_name FROM games g JOIN dat_entries e ON e.id = g.dat_entry_id
		WHERE g.system_id = ? AND g.game_key = ?`, systemID, gameKey).Scan(&name)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("look up DAT game name for %s: %w", gameKey, err)
	}
	return name, true, nil
}

// PruneGamesUnderRoot deletes games of systemID whose source lies under root
// and whose game_key is not in keep (they vanished from the library, or
// their identity changed, e.g. after a DAT update verified them under a new
// content hash). Groups left with no games and no manual override are
// removed too. Returns how many games were deleted.
func (db *DB) PruneGamesUnderRoot(systemID, root string, keep map[string]bool) (int, error) {
	rows, err := db.Query(`SELECT id, game_key, ultimate_source_ref FROM games WHERE system_id = ?`, systemID)
	if err != nil {
		return 0, err
	}
	prefix := strings.ToLower(filepath.Clean(root)) + string(filepath.Separator)
	var doomed []int64
	for rows.Next() {
		var id int64
		var key, ref string
		if err := rows.Scan(&id, &key, &ref); err != nil {
			rows.Close()
			return 0, err
		}
		lref := strings.ToLower(filepath.Clean(ref))
		if !keep[key] && strings.HasPrefix(lref, prefix) {
			doomed = append(doomed, id)
		}
	}
	rows.Close()
	for _, id := range doomed {
		if _, err := db.Exec(`DELETE FROM games WHERE id = ?`, id); err != nil {
			return 0, err
		}
	}
	if _, err := db.Exec(`DELETE FROM game_groups WHERE system_id = ?
		AND id NOT IN (SELECT group_id FROM games)
		AND id NOT IN (SELECT group_id FROM overrides)`, systemID); err != nil {
		return 0, err
	}
	return len(doomed), nil
}

// SelectedKey is the selected variant of one game group.
type SelectedKey struct {
	GroupID  int64
	SystemID string
	GameKey  string
}

// SelectedKeys lists the selected variant of every group, all systems, in one
// pass (cheaper than SelectedGames per system when only keys are needed).
func (db *DB) SelectedKeys() ([]SelectedKey, error) {
	rows, err := db.Query(`SELECT group_id, system_id, game_key FROM games WHERE is_selected = 1`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []SelectedKey
	for rows.Next() {
		var k SelectedKey
		if err := rows.Scan(&k.GroupID, &k.SystemID, &k.GameKey); err != nil {
			return nil, err
		}
		out = append(out, k)
	}
	return out, rows.Err()
}

// LinkedGame is a game key an output holds files for, with its group (0 when
// the game is no longer in the library).
type LinkedGame struct {
	SystemID string
	GameKey  string
	GroupID  int64
}

// LinkedGames lists the distinct games an output has recorded outputs for.
func (db *DB) LinkedGames(profileID string) ([]LinkedGame, error) {
	rows, err := db.Query(`SELECT DISTINCT ls.system_id, ls.game_key, COALESCE(g.group_id, 0)
		FROM link_state ls LEFT JOIN games g ON g.system_id = ls.system_id AND g.game_key = ls.game_key
		WHERE ls.profile_id = ?`, profileID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []LinkedGame
	for rows.Next() {
		var l LinkedGame
		if err := rows.Scan(&l.SystemID, &l.GameKey, &l.GroupID); err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}
