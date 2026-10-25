// Package library holds canonical game records and stable game_key/group_key
// derivation. The canonical library is a DB concept, not a folder tree.
// See spec §2.1, §2.11.
package library

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"
)

// GameKey derives spec §2.11's stable identity for one game variant: a
// hash of the sorted content hashes of its member ROMs/tracks. Pass one
// hash (e.g. a CRC32 hex string) for a single-ROM game; pass every
// member's hash for a multi-track/multi-file game (spec §2.5's BIN/CUE,
// GDI). This same function handles both DAT-matched and heuristic/
// unidentified files — spec §2.11 defines the latter as the trivial
// one-hash case.
//
// Deliberately independent of title/name: No-Intro and Redump rename
// entries over time, and a key derived from the title would orphan
// overrides and links across a DAT update. Sorting before hashing makes
// the result independent of the order tracks/members were supplied in.
func GameKey(contentHashes []string) string {
	sorted := make([]string, len(contentHashes))
	copy(sorted, contentHashes)
	sort.Strings(sorted)

	h := sha256.New()
	for _, s := range sorted {
		h.Write([]byte(s))
		h.Write([]byte{0}) // separator: avoids ["ab","c"] colliding with ["a","bc"]
	}
	return hex.EncodeToString(h.Sum(nil))
}
