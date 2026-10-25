package store

import (
	"fmt"
)

// Source is a registered ROM collection root for one system.
type Source struct {
	ID       int64
	Path     string
	SystemID string
}

// Profile is a configured output (spec §2.1): one layout for one consumer.
type Profile struct {
	ID         int64
	Name       string
	Kind       string // retroarch | esde | retrodeck | batocera | retrobat | raw | device
	OutputRoot string
	// Systems, when given to AddProfile, limits the output to those systems
	// (stored as ticks, see output_systems); it is not read back.
	Systems []string
	// StartEmpty makes a new output take no games until they are ticked.
	StartEmpty bool
	// RetroArchConfig is the retroarch.cfg of the RetroArch this output is for
	// ("" when none was chosen); only RetroArch outputs use it.
	RetroArchConfig string
}

// AddSource registers a source; both the path and the system must be new.
func (db *DB) AddSource(path, systemID string) (Source, error) {
	res, err := db.Exec(`INSERT INTO sources (path, system_id) VALUES (?, ?)`, path, systemID)
	if err != nil {
		return Source{}, fmt.Errorf("add source %s (%s): %w", path, systemID, err)
	}
	id, _ := res.LastInsertId()
	return Source{ID: id, Path: path, SystemID: systemID}, nil
}

// Sources lists registered sources ordered by system.
func (db *DB) Sources() ([]Source, error) {
	rows, err := db.Query(`SELECT id, path, system_id FROM sources ORDER BY system_id, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Source
	for rows.Next() {
		var s Source
		if err := rows.Scan(&s.ID, &s.Path, &s.SystemID); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// RemoveSource unregisters a source. Scanned files and games stay until the
// next sync prunes them.
func (db *DB) RemoveSource(id int64) error {
	_, err := db.Exec(`DELETE FROM sources WHERE id = ?`, id)
	return err
}

// AddProfile stores a profile.
func (db *DB) AddProfile(p Profile) (Profile, error) {
	startEmpty := p.StartEmpty || len(p.Systems) > 0
	res, err := db.Exec(`INSERT INTO profiles (name, kind, output_root, include_by_default, retroarch_config) VALUES (?, ?, ?, ?, ?)`,
		p.Name, p.Kind, p.OutputRoot, !startEmpty, p.RetroArchConfig)
	if err != nil {
		return Profile{}, fmt.Errorf("add profile %q: %w", p.Name, err)
	}
	p.ID, _ = res.LastInsertId()
	for _, s := range p.Systems {
		if _, err := db.Exec(`INSERT OR IGNORE INTO output_systems (profile_id, system_id, included) VALUES (?, ?, 1)`, p.ID, s); err != nil {
			return Profile{}, err
		}
	}
	p.StartEmpty = startEmpty
	return p, nil
}

// Profiles lists configured profiles ordered by name.
func (db *DB) Profiles() ([]Profile, error) {
	rows, err := db.Query(`SELECT id, name, kind, output_root, NOT include_by_default, retroarch_config FROM profiles ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Profile
	for rows.Next() {
		var p Profile
		if err := rows.Scan(&p.ID, &p.Name, &p.Kind, &p.OutputRoot, &p.StartEmpty, &p.RetroArchConfig); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// RemoveProfile deletes a profile's configuration (its output files and
// link_state rows are left for the caller to clean up).
func (db *DB) RemoveProfile(id int64) error {
	_, err := db.Exec(`DELETE FROM profiles WHERE id = ?`, id)
	return err
}

// SetRetroArchConfig records which retroarch.cfg an output is for ("" clears it).
func (db *DB) SetRetroArchConfig(id int64, path string) error {
	res, err := db.Exec(`UPDATE profiles SET retroarch_config = ? WHERE id = ?`, path, id)
	if err != nil {
		return fmt.Errorf("update output %d: %w", id, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("output %d not found", id)
	}
	return nil
}

// UpdateProfile changes an output's name and folder.
func (db *DB) UpdateProfile(id int64, name, outputRoot string) error {
	res, err := db.Exec(`UPDATE profiles SET name = ?, output_root = ? WHERE id = ?`, name, outputRoot, id)
	if err != nil {
		return fmt.Errorf("update output %d: %w", id, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("output %d not found", id)
	}
	return nil
}
