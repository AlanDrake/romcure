package profile

import "romcure/internal/store"

// GenerateRaw generates the generic "raw" profile: one folder per system
// (named by systemID) holding cleanly named files, with multi-disc .m3u
// sets but no frontend-specific files. Useful as a neutral tree for
// emulators and tools that just want a folder of games.
func GenerateRaw(db *store.DB, systemID string, opts Options) (Report, error) {
	if opts.ProfileID == "" {
		opts.ProfileID = "raw"
	}
	report, _, err := generateLayout(db, systemID, systemID, systemID, opts)
	return report, err
}
