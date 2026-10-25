package store

import (
	"database/sql"
	"errors"
	"fmt"
)

// Inclusion answers "is this game on this output?" from the user's ticks.
// The state of a game is the first that exists of: the tick on the game
// (the newest among its group's variants), the tick on its system, the
// output's default.
type Inclusion struct {
	Default bool
	Systems map[string]bool // system id -> ticked
	Groups  map[int64]bool  // game group id -> ticked
}

// Includes reports whether a game of systemID in group groupID is on the output.
func (in *Inclusion) Includes(systemID string, groupID int64) bool {
	if v, ok := in.Groups[groupID]; ok {
		return v
	}
	if v, ok := in.Systems[systemID]; ok {
		return v
	}
	return in.Default
}

// Inclusion loads one output's ticks.
func (db *DB) Inclusion(profileID int64) (*Inclusion, error) {
	in := &Inclusion{Systems: map[string]bool{}, Groups: map[int64]bool{}}
	if err := db.QueryRow(`SELECT include_by_default FROM profiles WHERE id = ?`, profileID).Scan(&in.Default); err != nil {
		return nil, fmt.Errorf("load output %d: %w", profileID, err)
	}
	rows, err := db.Query(`SELECT system_id, included FROM output_systems WHERE profile_id = ?`, profileID)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var s string
		var v bool
		if err := rows.Scan(&s, &v); err != nil {
			rows.Close()
			return nil, err
		}
		in.Systems[s] = v
	}
	rows.Close()

	// The newest tick among a group's variant keys wins, so a group that a
	// DAT import splits or merges keeps the user's choice.
	rows, err = db.Query(`SELECT g.group_id, og.included
		FROM output_games og JOIN games g ON g.system_id = og.system_id AND g.game_key = og.game_key
		WHERE og.profile_id = ? ORDER BY og.updated_at, og.rowid`, profileID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var gid int64
		var v bool
		if err := rows.Scan(&gid, &v); err != nil {
			return nil, err
		}
		in.Groups[gid] = v
	}
	return in, rows.Err()
}

// SetSystemIncluded ticks or unticks a whole system on an output and drops the
// game ticks inside it, so the system's setting applies to all of them.
func (db *DB) SetSystemIncluded(profileID int64, systemID string, included bool) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`DELETE FROM output_games WHERE profile_id = ? AND system_id = ?`, profileID, systemID); err != nil {
		return err
	}
	var def bool
	if err := tx.QueryRow(`SELECT include_by_default FROM profiles WHERE id = ?`, profileID).Scan(&def); err != nil {
		return err
	}
	if included == def {
		_, err = tx.Exec(`DELETE FROM output_systems WHERE profile_id = ? AND system_id = ?`, profileID, systemID)
	} else {
		_, err = tx.Exec(`INSERT INTO output_systems (profile_id, system_id, included) VALUES (?, ?, ?)
			ON CONFLICT (profile_id, system_id) DO UPDATE SET included = excluded.included`, profileID, systemID, included)
	}
	if err != nil {
		return err
	}
	return tx.Commit()
}

// GroupRef names a game group, with its system.
type GroupRef struct {
	ID       int64
	SystemID string
}

// SetGameIncluded ticks or unticks one game on an output.
func (db *DB) SetGameIncluded(profileID int64, groupID int64, included bool) error {
	var systemID string
	if err := db.QueryRow(`SELECT system_id FROM game_groups WHERE id = ?`, groupID).Scan(&systemID); err != nil {
		return fmt.Errorf("game group %d: %w", groupID, err)
	}
	return db.SetGamesIncluded(profileID, []GroupRef{{ID: groupID, SystemID: systemID}}, included)
}

// SetGamesIncluded ticks or unticks many games on an output in one
// transaction: a row for every variant key of each group. A tick equal to what
// the game would inherit is stored as no row at all (only exceptions are kept).
func (db *DB) SetGamesIncluded(profileID int64, refs []GroupRef, included bool) error {
	var def bool
	if err := db.QueryRow(`SELECT include_by_default FROM profiles WHERE id = ?`, profileID).Scan(&def); err != nil {
		return fmt.Errorf("output %d: %w", profileID, err)
	}
	inherited := map[string]bool{}
	rows, err := db.Query(`SELECT system_id, included FROM output_systems WHERE profile_id = ?`, profileID)
	if err != nil {
		return err
	}
	for rows.Next() {
		var s string
		var v bool
		if err := rows.Scan(&s, &v); err != nil {
			rows.Close()
			return err
		}
		inherited[s] = v
	}
	rows.Close()

	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	del, err := tx.Prepare(`DELETE FROM output_games WHERE profile_id = ? AND system_id = ?
		AND game_key IN (SELECT game_key FROM games WHERE group_id = ?)`)
	if err != nil {
		return err
	}
	defer del.Close()
	ins, err := tx.Prepare(`INSERT INTO output_games (profile_id, system_id, game_key, included)
		SELECT ?, system_id, game_key, ? FROM games WHERE group_id = ?
		ON CONFLICT (profile_id, system_id, game_key) DO UPDATE SET included = excluded.included,
			updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now')`)
	if err != nil {
		return err
	}
	defer ins.Close()
	for _, ref := range refs {
		inh, ok := inherited[ref.SystemID]
		if !ok {
			inh = def
		}
		if included == inh {
			_, err = del.Exec(profileID, ref.SystemID, ref.ID)
		} else {
			_, err = ins.Exec(profileID, included, ref.ID)
		}
		if err != nil {
			return err
		}
	}
	return tx.Commit()
}

// SetOutputDefault ticks or unticks every game of an output at once, dropping
// all system and game ticks.
func (db *DB) SetOutputDefault(profileID int64, included bool) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, q := range []string{`DELETE FROM output_games WHERE profile_id = ?`, `DELETE FROM output_systems WHERE profile_id = ?`} {
		if _, err := tx.Exec(q, profileID); err != nil {
			return err
		}
	}
	if _, err := tx.Exec(`UPDATE profiles SET include_by_default = ? WHERE id = ?`, included, profileID); err != nil {
		return err
	}
	return tx.Commit()
}

// GameTick is one stored game-level tick.
type GameTick struct {
	SystemID, GameKey, UpdatedAt string
	Included                     bool
}

// TickState is everything the user ticked on one output, for undo.
type TickState struct {
	Default bool
	Systems map[string]bool
	Games   []GameTick
}

// TickState captures an output's ticks.
func (db *DB) TickState(profileID int64) (TickState, error) {
	ts := TickState{Systems: map[string]bool{}}
	if err := db.QueryRow(`SELECT include_by_default FROM profiles WHERE id = ?`, profileID).Scan(&ts.Default); err != nil {
		return ts, err
	}
	rows, err := db.Query(`SELECT system_id, included FROM output_systems WHERE profile_id = ?`, profileID)
	if err != nil {
		return ts, err
	}
	for rows.Next() {
		var s string
		var v bool
		if err := rows.Scan(&s, &v); err != nil {
			rows.Close()
			return ts, err
		}
		ts.Systems[s] = v
	}
	rows.Close()
	rows, err = db.Query(`SELECT system_id, game_key, included, updated_at FROM output_games WHERE profile_id = ? ORDER BY updated_at, rowid`, profileID)
	if err != nil {
		return ts, err
	}
	defer rows.Close()
	for rows.Next() {
		var g GameTick
		if err := rows.Scan(&g.SystemID, &g.GameKey, &g.Included, &g.UpdatedAt); err != nil {
			return ts, err
		}
		ts.Games = append(ts.Games, g)
	}
	return ts, rows.Err()
}

// RestoreTickState puts an output's ticks back as captured.
func (db *DB) RestoreTickState(profileID int64, ts TickState) error {
	if err := db.SetOutputDefault(profileID, ts.Default); err != nil {
		return err
	}
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for s, v := range ts.Systems {
		if _, err := tx.Exec(`INSERT INTO output_systems (profile_id, system_id, included) VALUES (?, ?, ?)`, profileID, s, v); err != nil {
			return err
		}
	}
	for _, g := range ts.Games {
		if _, err := tx.Exec(`INSERT INTO output_games (profile_id, system_id, game_key, included, updated_at) VALUES (?, ?, ?, ?, ?)`,
			profileID, g.SystemID, g.GameKey, g.Included, g.UpdatedAt); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// FindMovedOverride finds an override that points at one of keys but belongs
// to another group of the same system (a DAT import regrouped the game), so
// the caller can move it to the group the key is in now.
func (db *DB) FindMovedOverride(systemID string, keys []string) (groupID int64, ok bool, err error) {
	for _, k := range keys {
		err = db.QueryRow(`SELECT o.group_id FROM overrides o JOIN game_groups gg ON gg.id = o.group_id
			WHERE o.override_game_key = ? AND gg.system_id = ? LIMIT 1`, k, systemID).Scan(&groupID)
		if err == nil {
			return groupID, true, nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return 0, false, err
		}
	}
	return 0, false, nil
}

// MoveOverride re-attaches an override to another group.
func (db *DB) MoveOverride(from, to int64) error {
	_, err := db.Exec(`UPDATE overrides SET group_id = ? WHERE group_id = ?`, to, from)
	return err
}

// RemoveProfileData forgets an output's recorded link state (its ticks go
// with the profile row, by cascade). Needed because profile ids can be reused.
func (db *DB) RemoveProfileData(profileID string) error {
	_, err := db.Exec(`DELETE FROM link_state WHERE profile_id = ?`, profileID)
	return err
}

// ClearGamesTicks drops the user's own tick on each group, so the games follow
// their system (or the output's default) again.
func (db *DB) ClearGamesTicks(profileID int64, refs []GroupRef) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, ref := range refs {
		if _, err := tx.Exec(`DELETE FROM output_games WHERE profile_id = ? AND system_id = ?
			AND game_key IN (SELECT game_key FROM games WHERE group_id = ?)`, profileID, ref.SystemID, ref.ID); err != nil {
			return err
		}
	}
	return tx.Commit()
}
