// Package match matches scanned files against DAT entries with three
// confidence tiers: DAT-verified, heuristic, unidentified. See spec §2.6.
package match

import (
	"database/sql"
	"fmt"

	"romcure/internal/store"
)

// Confidence is how sure the app is about a file's identity.
type Confidence string

const (
	ConfidenceDATVerified  Confidence = "dat_verified"
	ConfidenceHeuristic    Confidence = "heuristic"
	ConfidenceUnidentified Confidence = "unidentified"
)

// Result is the outcome of matching one scanned file (or archive member)
// against loaded DAT entries, or falling back to filename heuristics for
// zero-DAT operation (spec §2.6). Region/Language/Revision/IsBeta/
// IsProto/IsDemo are derived the same way (scene-release tag parsing)
// regardless of Confidence: for a DAT-verified match they come from the
// DAT's own game name, which carries the same tags in a well-formed DAT.
type Result struct {
	Confidence Confidence
	Title      string
	Region     string // "" if unknown or not applicable
	Language   string
	Revision   string
	IsBeta     bool
	IsProto    bool
	IsDemo     bool
	// VariantPenalty ranks equal variants: 0 clean, 1 alternate dump, 2
	// modified/damaged (TOSEC flags). Lower is preferred.
	VariantPenalty int
	IsVerifiedDump bool // only meaningful when Confidence == ConfidenceDATVerified
	// DatEntryID and CloneOf are set only when Confidence ==
	// ConfidenceDATVerified. CloneOf is the DAT's cloneof target game
	// name, "" if this entry isn't a clone (or the DAT has no P/C data
	// at all — spec §2.9).
	DatEntryID int64
	CloneOf    string
	// ParentCloneDAT is true when the matching DAT carries parent/clone data,
	// so an empty CloneOf really means "a game of its own" (spec §2.9). A plain
	// DAT (Redump, TOSEC, a non-P/C No-Intro DAT) says nothing about grouping.
	ParentCloneDAT bool
}

// MatchByHash looks up a CRC32+size pair against the dat_entries loaded
// for systemID. Matching on CRC32 alone risks a collision across
// unrelated files; requiring size too, matching how No-Intro/Redump
// identify dumps, makes that effectively impossible for real ROM sizes.
// ok is false if nothing matched.
func MatchByHash(db *store.DB, systemID, crc32 string, size int64) (Result, bool, error) {
	var id int64
	var gameName string
	var status, cloneOf sql.NullString
	var pc bool
	err := db.QueryRow(
		`SELECT e.id, e.game_name, e.status, e.clone_of, d.has_parent_clone FROM dat_entries e
		 JOIN dats d ON d.id = e.dat_id
		 WHERE e.system_id = ? AND e.crc32 = ? AND e.size = ? LIMIT 1`,
		systemID, crc32, size,
	).Scan(&id, &gameName, &status, &cloneOf, &pc)
	if err == sql.ErrNoRows {
		return Result{}, false, nil
	}
	if err != nil {
		return Result{}, false, fmt.Errorf("match by hash (system=%s crc32=%s size=%d): %w", systemID, crc32, size, err)
	}

	info := classify(gameName)

	return Result{
		Confidence:     ConfidenceDATVerified,
		Title:          gameName,
		Region:         info.region,
		Language:       info.language,
		Revision:       info.revision,
		IsBeta:         info.isBeta,
		IsProto:        info.isProto,
		IsDemo:         info.isDemo,
		VariantPenalty: info.penalty,
		IsVerifiedDump: status.String != "baddump" && status.String != "nodump",
		DatEntryID:     id,
		CloneOf:        cloneOf.String,
		ParentCloneDAT: pc,
	}, true, nil
}

// MatchFile matches one file by CRC32+size against systemID's loaded DAT
// entries, falling back to MatchHeuristic (filename only) when there's no
// DAT loaded for this system or nothing matched — the zero-DAT path
// (spec §2.6).
func MatchFile(db *store.DB, systemID, crc32 string, size int64, filename string) (Result, error) {
	return matchFile(db, systemID, filename, hashOf{crc32, size})
}

// hashOf is one CRC a file can be looked up by, with the size it covers.
type hashOf struct {
	crc32 string
	size  int64
}

// matchFile tries each hash in order against the DATs, then the file name.
// A headered cartridge dump is tried by its whole-file CRC and then by the
// CRC after the header, which is what No-Intro DATs list (romheader).
func matchFile(db *store.DB, systemID, filename string, hashes ...hashOf) (Result, error) {
	for _, h := range hashes {
		if h.crc32 == "" {
			continue
		}
		res, ok, err := MatchByHash(db, systemID, h.crc32, h.size)
		if err != nil {
			return Result{}, err
		}
		if ok {
			return res, nil
		}
	}
	return MatchHeuristic(filename), nil
}
