package app

import (
	"archive/zip"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"romcure/internal/dat"
	"romcure/internal/profile"
)

// DATImport is the outcome of importing a DAT file by hand.
type DATImport struct {
	SystemID string
	Name     string // the DAT's own name (its header)
	Games    int
	// ParentClone is true for a parent/clone DAT, which groups the versions of
	// a game exactly (spec §2.9); otherwise versions are grouped by title.
	ParentClone bool
}

// ImportDAT imports a DAT file the user downloaded (typically a newer No-Intro
// DAT than libretro-database's copy): Logiqx XML or ClrMamePro text, or a zip
// holding one. systemID "" means "the system the DAT names". It replaces the
// DAT imported by hand earlier for that system and the No-Intro DAT the app
// downloaded for it; Redump and TOSEC DATs are left alone. Sync afterwards to
// apply it.
func (a *App) ImportDAT(path, systemID string) (DATImport, error) {
	df, err := readDAT(path)
	if err != nil {
		return DATImport{}, err
	}
	if len(df.Games) == 0 {
		return DATImport{}, fmt.Errorf("%s lists no games; is it a DAT file?", filepath.Base(path))
	}
	if systemID == "" {
		var ok bool
		if systemID, ok = SystemOfDAT(df.SystemName); !ok {
			return DATImport{}, fmt.Errorf("can't tell which system %q is for; choose the system and import again", df.SystemName)
		}
	}
	id, err := dat.Import(a.DB, df, systemID, path)
	if err != nil {
		return DATImport{}, err
	}
	// Replace the earlier manual import of this system, and the No-Intro DAT
	// downloaded for it (which "Update DATs" then leaves alone), only after
	// the new one is in.
	if _, err := a.DB.Exec(`UPDATE dats SET remote_source = 'manual', remote_slug = ? WHERE id = ?`, systemID, id); err != nil {
		return DATImport{}, err
	}
	if _, err := a.DB.Exec(`DELETE FROM dats WHERE remote_source IN ('manual', 'no-intro') AND remote_slug = ? AND id <> ?`, systemID, id); err != nil {
		return DATImport{}, err
	}
	a.Log.Info("imported DAT", "system", systemID, "name", df.SystemName, "games", len(df.Games), "path", path)
	return DATImport{SystemID: systemID, Name: df.SystemName, Games: len(df.Games), ParentClone: df.HasParentClone}, nil
}

// readDAT parses a DAT file, or the first DAT inside a zip (No-Intro and
// Redump hand theirs out zipped).
func readDAT(path string) (*dat.Datafile, error) {
	if !strings.EqualFold(filepath.Ext(path), ".zip") {
		df, err := dat.ParseFile(path)
		if err != nil {
			return nil, fmt.Errorf("%s is not a DAT file this app can read: %w", filepath.Base(path), err)
		}
		return df, nil
	}
	zr, err := zip.OpenReader(path)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", filepath.Base(path), err)
	}
	defer zr.Close()
	for _, f := range zr.File {
		ext := strings.ToLower(filepath.Ext(f.Name))
		if ext != ".dat" && ext != ".xml" {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return nil, err
		}
		df, err := dat.Parse(io.LimitReader(rc, 512<<20))
		rc.Close()
		if err != nil {
			return nil, fmt.Errorf("%s in %s is not a DAT file this app can read: %w", f.Name, filepath.Base(path), err)
		}
		return df, nil
	}
	return nil, errors.New(filepath.Base(path) + " holds no .dat or .xml file")
}

// SystemOfDAT finds the system a DAT is for from its name, as No-Intro and
// Redump write it ("Nintendo - Nintendo Entertainment System (Headered)
// (Parent-Clone)", "Sega - Mega Drive - Genesis"): the longest known system
// name the DAT's name starts with.
func SystemOfDAT(name string) (string, bool) {
	best, bestLen := "", 0
	lower := strings.ToLower(strings.TrimSpace(name))
	for _, s := range profile.KnownSystems() {
		full := strings.ToLower(s.Name)
		if strings.HasPrefix(lower, full) && len(full) > bestLen {
			rest := lower[len(full):]
			if rest == "" || rest[0] == ' ' || rest[0] == '(' {
				best, bestLen = s.ID, len(full)
			}
		}
	}
	return best, best != ""
}
