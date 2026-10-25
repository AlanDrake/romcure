package dat

import (
	"database/sql"
	"fmt"

	"romcure/internal/store"
)

// Import persists a parsed Datafile into db under systemID, recording
// sourcePath for provenance, and returns the new dats.id.
func Import(db *store.DB, df *Datafile, systemID, sourcePath string) (int64, error) {
	datID, err := db.InsertDat(store.DatRecord{
		SystemID:       systemID,
		Format:         string(df.Format),
		SourcePath:     sourcePath,
		HasParentClone: df.HasParentClone,
	})
	if err != nil {
		return 0, fmt.Errorf("insert dats row for %s: %w", sourcePath, err)
	}

	var entries []store.DatEntryRecord
	for _, g := range df.Games {
		for _, r := range g.Roms {
			entries = append(entries, store.DatEntryRecord{
				DatID:    datID,
				SystemID: systemID,
				GameName: g.Name,
				RomName:  r.Name,
				Size:     r.Size,
				CRC32:    nullIfEmpty(r.CRC32),
				SHA1:     nullIfEmpty(r.SHA1),
				MD5:      nullIfEmpty(r.MD5),
				CloneOf:  nullIfEmpty(g.CloneOf),
				Status:   nullIfEmpty(r.Status),
				Serial:   nullIfEmpty(r.Serial),
			})
		}
	}
	if err := db.InsertDatEntries(entries); err != nil {
		return 0, fmt.Errorf("insert dat_entries for %s: %w", sourcePath, err)
	}

	return datID, nil
}

func nullIfEmpty(s string) sql.NullString {
	if s == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: s, Valid: true}
}
