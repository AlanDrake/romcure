package profile

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"romcure/internal/store"
)

// RetroArch output: the shared layout plus an .lpl playlist per system.

type lplItem struct {
	Path     string `json:"path"`
	Label    string `json:"label"`
	CorePath string `json:"core_path"`
	CoreName string `json:"core_name"`
	CRC32    string `json:"crc32"`
	DBName   string `json:"db_name"`
}

type lplFile struct {
	Version            string    `json:"version"`
	DefaultCorePath    string    `json:"default_core_path"`
	DefaultCoreName    string    `json:"default_core_name"`
	LabelDisplayMode   int       `json:"label_display_mode"`
	RightThumbnailMode int       `json:"right_thumbnail_mode"`
	LeftThumbnailMode  int       `json:"left_thumbnail_mode"`
	SortMode           int       `json:"sort_mode"`
	Items              []lplItem `json:"items"`
}

const (
	retroArchProfileID = "retroarch"
	discsDirName       = ".discs"
)

// GenerateRetroArch generates (or incrementally updates) a RetroArch
// profile for systemID from the canonical library's selected games,
// linking each directly to its ultimate source (spec §2.1) and writing
// an .lpl playlist. Games whose display name carries a "(Disc N)" tag
// are grouped into multi-disc sets: each disc links into a sibling
// .discs/ folder (dot-prefixed and, on Windows, given the hidden
// attribute — spec §2.5) and one .m3u playlist is generated per set,
// referencing discs by relative path, with a single corresponding .lpl
// entry (not one per disc).
//
// Re-running leaves outputs that are still right alone, replaces the ones
// whose game now has another source, and removes those no longer wanted
// (see linkFile and pruneOrphans). Checking outputs against the disk is
// internal/plan's job.
func GenerateRetroArch(db *store.DB, systemID string, opts Options) (Report, error) {
	dbName, ok := LibretroDBName(systemID)
	if !ok {
		dbName = systemID
	}
	report, items, err := generateLayout(db, systemID, dbName, dbName, opts)
	if err != nil {
		return report, err
	}

	playlist := lplFile{Version: "1.5", Items: items}
	data, err := json.MarshalIndent(playlist, "", "  ")
	if err != nil {
		return report, fmt.Errorf("marshal playlist for %s: %w", systemID, err)
	}

	if err := os.MkdirAll(opts.OutputRoot, 0o755); err != nil {
		return report, fmt.Errorf("create profile output root %s: %w", opts.OutputRoot, err)
	}
	playlistPath := filepath.Join(opts.OutputRoot, dbName+".lpl")
	if err := os.WriteFile(playlistPath, data, 0o644); err != nil {
		return report, fmt.Errorf("write playlist %s: %w", playlistPath, err)
	}

	if opts.PlaylistDir != "" {
		if _, err := mergePlaylist(filepath.Join(opts.PlaylistDir, dbName+".lpl"), opts.OutputRoot, items,
			defaultCore{opts.DefaultCorePath, opts.DefaultCoreName}); err != nil {
			return report, err
		}
	}

	return report, nil
}
