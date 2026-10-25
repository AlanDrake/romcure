package store

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"strings"
)

// Variant is one candidate version of a game, as stored after a sync.
type Variant struct {
	GameKey       string
	Title         string // DAT name when DAT-verified, else the file (or member) name
	Confidence    string
	Region        string
	Language      string
	Revision      string
	VerifiedDump  bool
	BetaProtoDemo bool
	Selected      bool
	SourcePath    string
	MemberName    string
}

// Group is a game with all of its variants and the explanation of the pick.
type Group struct {
	ID              int64
	SystemID        string
	Name            string
	GroupingSource  string // dat_pc | clone_list | title_normalization | none
	SelectionReason string
	Overridden      bool
	OverrideKey     string // the forced variant, "" when there is no override
	Variants        []Variant
}

// ListGroups returns every group of a system with its variants, groups
// ordered by name and variants with the selected one first.
func (db *DB) ListGroups(systemID string) ([]Group, error) {
	return db.listGroups(`gg.system_id = ?`, systemID)
}

// GroupsByID returns the given groups (any systems) with their variants.
func (db *DB) GroupsByID(ids []int64) ([]Group, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	args := make([]any, len(ids))
	marks := make([]string, len(ids))
	for i, id := range ids {
		args[i], marks[i] = id, "?"
	}
	return db.listGroups(`gg.id IN (`+strings.Join(marks, ",")+`)`, args...)
}

func (db *DB) listGroups(where string, args ...any) ([]Group, error) {
	rows, err := db.Query(`
		SELECT gg.id, gg.system_id, gg.display_name, gg.grouping_source, gg.selection_reason,
		       COALESCE(o.override_game_key, ''),
		       g.game_key, COALESCE(e.game_name, ''), g.confidence, COALESCE(g.region, ''),
		       COALESCE(g.language, ''), COALESCE(g.revision, ''), g.is_verified_dump,
		       g.is_beta_proto_demo, g.is_selected, g.ultimate_source_ref, COALESCE(g.member_name, '')
		FROM game_groups gg
		JOIN games g ON g.group_id = gg.id
		LEFT JOIN dat_entries e ON e.id = g.dat_entry_id
		LEFT JOIN overrides o ON o.group_id = gg.id
		WHERE `+where+`
		ORDER BY gg.display_name COLLATE NOCASE, gg.id, g.is_selected DESC, g.game_key`, args...)
	if err != nil {
		return nil, fmt.Errorf("list groups: %w", err)
	}
	defer rows.Close()

	var out []Group
	for rows.Next() {
		var (
			gid                               int64
			sysID                             string
			name, source, reason, overrideKey string
			v                                 Variant
			datName                           string
		)
		if err := rows.Scan(&gid, &sysID, &name, &source, &reason, &overrideKey,
			&v.GameKey, &datName, &v.Confidence, &v.Region, &v.Language, &v.Revision, &v.VerifiedDump,
			&v.BetaProtoDemo, &v.Selected, &v.SourcePath, &v.MemberName); err != nil {
			return nil, err
		}
		v.Title = datName
		if v.Title == "" {
			if v.MemberName != "" {
				v.Title = v.MemberName
			} else {
				v.Title = filepath.Base(v.SourcePath)
			}
		}
		if len(out) == 0 || out[len(out)-1].ID != gid {
			out = append(out, Group{ID: gid, SystemID: sysID, Name: name, GroupingSource: source,
				SelectionReason: reason, Overridden: overrideKey != "", OverrideKey: overrideKey})
		}
		g := &out[len(out)-1]
		g.Variants = append(g.Variants, v)
	}
	return out, rows.Err()
}

// GroupOf returns the group id a game belongs to.
func (db *DB) GroupOf(systemID, gameKey string) (int64, error) {
	var id int64
	err := db.QueryRow(`SELECT group_id FROM games WHERE system_id = ? AND game_key = ?`, systemID, gameKey).Scan(&id)
	if err == sql.ErrNoRows {
		return 0, fmt.Errorf("game %s not found in %s", gameKey, systemID)
	}
	return id, err
}
