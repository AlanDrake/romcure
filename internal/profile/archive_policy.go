package profile

import "romcure/internal/store"

// ArchivePolicy decides how a profile handles an archived ROM (spec §2.8).
type ArchivePolicy string

const (
	// PolicyPassthrough links the archive itself, renamed to the clean
	// game name. Only valid when the archive contains exactly the
	// matched ROM and nothing else.
	PolicyPassthrough ArchivePolicy = "passthrough"
	// PolicyExtract extracts the single matched ROM out of the archive
	// and links the extracted file instead.
	PolicyExtract ArchivePolicy = "extract"
	// PolicyTranscode extracts, converts, and verifies into the
	// transcode cache (M4 — not implemented by this package).
	PolicyTranscode ArchivePolicy = "transcode"
)

// DecideArchivePolicy implements spec §2.8's RetroArch defaults for an
// archived ROM (kind "zip", "sevenzip" or "rar" — raw files have no archive
// policy, callers should link them directly without consulting this):
//
//   - Disc systems always transcode (deferred to M4 regardless of
//     archive kind).
//   - A zip containing exactly the matched ROM passes through.
//   - A zip containing more than that (isOnlyMember false) must extract:
//     passthrough would hand the target the whole archive, which isn't
//     "just this ROM" to it.
//   - 7z and RAR always extract: RetroArch cores generally don't read them.
func DecideArchivePolicy(kind string, isOnlyMember, isDiscSystem bool) ArchivePolicy {
	if isDiscSystem {
		return PolicyTranscode
	}
	switch kind {
	case "zip":
		if isOnlyMember {
			return PolicyPassthrough
		}
		return PolicyExtract
	default: // "sevenzip", "rar"
		return PolicyExtract
	}
}

// archivePolicyFor is DecideArchivePolicy for a scanned archive, with one more
// rule: a zip holding a member compressed with an old method (implode, shrink)
// is never passed through, because RetroArch cannot read it either; its ROM is
// extracted (with 7-Zip) instead. The method is the one the scan
// recorded: linking and planning never open the zip (NULL = not looked at yet,
// treated as fine).
func archivePolicyFor(rec store.FileRecord, isOnlyMember bool, systemID string) ArchivePolicy {
	p := DecideArchivePolicy(rec.Kind, isOnlyMember, IsDiscSystem(systemID))
	if p == PolicyPassthrough && rec.ZipMethod.String != "" {
		return PolicyExtract
	}
	return p
}
