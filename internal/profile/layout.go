package profile

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"romcure/internal/library"
	"romcure/internal/link"
	"romcure/internal/store"
)

// The folder layout every output shares: which games go in, one file per
// game, multi-disc sets with an .m3u, and removing what no longer belongs.

// selectedGames lists systemID's selected games that the output takes. Games
// whose output names would collide get a short code (see disambiguate); the
// second result is how many were renamed.
func selectedGames(db *store.DB, systemID string, opts Options) ([]store.SelectedGame, int, error) {
	games, err := db.SelectedGames(systemID)
	if err != nil {
		return nil, 0, err
	}
	if opts.Include != nil {
		kept := games[:0]
		for _, g := range games {
			if opts.Include(g) {
				kept = append(kept, g)
			}
		}
		games = kept
	}
	games, renamed := disambiguate(games)
	return games, renamed, nil
}

// disambiguate gives games that would be written to the same file name (two
// different games whose names clean to the same text, or differ only by case)
// a short, stable code from their content key: "AC_DC [3fa91c]". Games that do
// not collide keep their plain name, and a code never depends on the other
// game, so adding or removing a namesake never renames a third game. Discs of
// a multi-disc game are left alone: their names are tied together by the
// disc tag.
func disambiguate(games []store.SelectedGame) ([]store.SelectedGame, int) {
	fold := func(g store.SelectedGame) string {
		return strings.ToLower(invalidFileNameChars.ReplaceAllString(g.DisplayName, "_"))
	}
	count := map[string]int{}
	for _, g := range games {
		if _, _, disc := library.ParseDiscTag(g.DisplayName); !disc {
			count[fold(g)]++
		}
	}
	renamed := 0
	out := make([]store.SelectedGame, len(games))
	copy(out, games)
	for i, g := range out {
		if _, _, disc := library.ParseDiscTag(g.DisplayName); disc || count[fold(g)] < 2 {
			continue
		}
		code := g.GameKey
		if len(code) > 6 {
			code = code[:6]
		}
		out[i].DisplayName = g.DisplayName + " [" + code + "]"
		renamed++
	}
	return out, renamed
}

// generateLayout links/copies systemID's selected games into
// OutputRoot/folderName (multi-disc sets into a hidden .discs/ sibling with
// one .m3u each) and returns the playable entries. It is the part shared by
// every folder-based profile; dbName is only used to label .lpl items.
func generateLayout(db *store.DB, systemID, folderName, dbName string, opts Options) (Report, []lplItem, error) {
	games, renamed, err := selectedGames(db, systemID, opts)
	if err != nil {
		return Report{}, nil, fmt.Errorf("generate profile for %s: %w", systemID, err)
	}

	sysDir := filepath.Join(opts.OutputRoot, folderName)
	discsDir := filepath.Join(sysDir, discsDirName)

	opts.failed = prewarmConversions(db, systemID, games, opts)
	singles, multiDiscSets := partitionByDisc(games)

	var report Report
	var items []lplItem
	keepKeys := map[string]bool{} // playlists generated this run
	if renamed > 0 {
		report.Notes = append(report.Notes, fmt.Sprintf("%d games share a name with another game in this system; a short code was added to tell them apart", renamed))
	}

	for _, g := range singles {
		if err := opts.ctx().Err(); err != nil {
			return report, items, err
		}
		if opts.OnItem != nil {
			opts.OnItem(g.DisplayName)
		}
		dstPath, err := linkOneGame(db, sysDir, systemID, g, opts)
		if err != nil {
			report.Errors = append(report.Errors, fmt.Sprintf("%s: %v", g.DisplayName, err))
			continue
		}
		report.Linked = append(report.Linked, LinkedGame{GameKey: g.GameKey, Title: g.DisplayName, Path: dstPath})
		items = append(items, lplItem{Path: dstPath, Label: g.DisplayName, CorePath: "DETECT", CoreName: "DETECT", CRC32: "DETECT", DBName: dbName + ".lpl"})
	}

	for baseTitle, discs := range multiDiscSets {
		if err := opts.ctx().Err(); err != nil {
			return report, items, err
		}
		if opts.OnItem != nil {
			opts.OnItem(baseTitle)
		}
		item, linked, errs := generateMultiDiscSet(db, sysDir, discsDir, systemID, dbName, baseTitle, discs, opts)
		if item != nil {
			keepKeys[m3uKey(baseTitle)] = true
		}
		report.Linked = append(report.Linked, linked...)
		report.Errors = append(report.Errors, errs...)
		if item != nil {
			items = append(items, *item)
		}
	}

	// A cancelled run reports it, instead of listing the interrupted game as a
	// problem, and leaves the existing outputs alone.
	if err := opts.ctx().Err(); err != nil {
		return report, items, err
	}
	if n, err := pruneOrphans(db, systemID, games, keepKeys, opts); err != nil {
		report.Errors = append(report.Errors, "clean up old outputs: "+err.Error())
	} else if n > 0 {
		report.Notes = append(report.Notes, fmt.Sprintf("removed %d output(s) for games no longer in the library or no longer chosen", n))
	}
	return report, items, nil
}

// pruneOrphans removes outputs this profile created earlier for games of
// systemID that are no longer selected (deleted from the source, or another
// version is now picked under a different name), and forgets their
// link_state rows. Only recorded outputs inside the output root are touched,
// and only links or regular files, never directories or the user's files.
func pruneOrphans(db *store.DB, systemID string, selected []store.SelectedGame, extraKeep map[string]bool, opts Options) (int, error) {
	keep := make(map[string]bool, len(selected)+len(extraKeep))
	for k := range extraKeep {
		keep[k] = true
	}
	for _, g := range selected {
		keep[g.GameKey] = true
	}
	rows, err := db.ListLinkStates(profileIDOf(opts))
	if err != nil {
		return 0, err
	}
	var doomed []store.LinkStateRecord
	for _, r := range rows {
		if r.SystemID == systemID && !keep[r.GameKey] {
			doomed = append(doomed, r)
		}
	}
	return removeRecorded(db, doomed, opts.OutputRoot)
}

// RemoveOutputs deletes everything an output created (links and copies it
// recorded, inside its output root) and forgets its link_state rows. The
// user's own files and directories are never touched.
func RemoveOutputs(db *store.DB, profileID, outputRoot string) (int, error) {
	rows, err := db.ListLinkStates(profileID)
	if err != nil {
		return 0, err
	}
	return removeRecorded(db, rows, outputRoot)
}

// removeRecorded removes the recorded outputs in rows from outputRoot and
// drops their link_state rows. Only links or regular files that are not the
// source file itself are deleted.
func removeRecorded(db *store.DB, rows []store.LinkStateRecord, outputRoot string) (int, error) {
	removed := 0
	for _, r := range rows {
		target := filepath.Join(outputRoot, r.TargetPath)
		if rel, err := filepath.Rel(outputRoot, target); err != nil || strings.HasPrefix(rel, "..") {
			continue
		}
		if li, err := os.Lstat(target); err == nil && (li.Mode()&os.ModeSymlink != 0 || li.Mode().IsRegular()) && !sameFile(target, r.SourcePath) {
			if err := link.GuardWrite(target); err != nil {
				return removed, err
			}
			if err := os.Remove(target); err != nil {
				return removed, err
			}
			removed++
		}
		if err := db.DeleteLinkState(r.ProfileID, r.TargetPath); err != nil {
			return removed, err
		}
	}
	return removed, nil
}

// discEntry is one game known to be a disc of a multi-disc set.
type discEntry struct {
	game    store.SelectedGame
	discNum int
}

// partitionByDisc splits games into single-item games and multi-disc
// sets (keyed by the title with its disc tag removed). A title with a
// disc tag but no sibling sharing that base title isn't really a set —
// it's treated as a single game instead.
func partitionByDisc(games []store.SelectedGame) (singles []store.SelectedGame, sets map[string][]discEntry) {
	sets = make(map[string][]discEntry)
	for _, g := range games {
		if num, base, ok := library.ParseDiscTag(g.DisplayName); ok {
			base = library.SetTitle(base)
			sets[base] = append(sets[base], discEntry{game: g, discNum: num})
		} else {
			singles = append(singles, g)
		}
	}
	for base, entries := range sets {
		if len(entries) < 2 {
			singles = append(singles, entries[0].game)
			delete(sets, base)
		}
	}
	return singles, sets
}

// m3uKey is the link_state game key of a multi-disc set's playlist: it
// belongs to the set, not to any one disc.
func m3uKey(baseTitle string) string { return "m3u:" + baseTitle }

// generateMultiDiscSet links every disc in discs into discsDir, writes
// baseTitle's .m3u in sysDir referencing them by relative path, and
// returns the single .lpl item for the set (nil if every disc failed).
func generateMultiDiscSet(db *store.DB, sysDir, discsDir, systemID, dbName, baseTitle string, discs []discEntry, opts Options) (*lplItem, []LinkedGame, []string) {
	sort.Slice(discs, func(i, j int) bool { return discs[i].discNum < discs[j].discNum })

	var linked []LinkedGame
	var errs []string
	var relPaths []string

	for _, d := range discs {
		dstPath, err := linkOneGame(db, discsDir, systemID, d.game, opts)
		if err != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", d.game.DisplayName, err))
			continue
		}
		rel, err := filepath.Rel(sysDir, dstPath)
		if err != nil {
			errs = append(errs, fmt.Sprintf("%s: compute m3u-relative path: %v", d.game.DisplayName, err))
			continue
		}
		relPaths = append(relPaths, rel)
		linked = append(linked, LinkedGame{GameKey: d.game.GameKey, Title: d.game.DisplayName, Path: dstPath})
	}
	if len(relPaths) == 0 {
		return nil, linked, errs
	}

	if err := os.MkdirAll(discsDir, 0o755); err != nil {
		errs = append(errs, fmt.Sprintf("%s: create %s: %v", baseTitle, discsDir, err))
		return nil, linked, errs
	}
	if err := setHiddenAttribute(discsDir); err != nil {
		errs = append(errs, fmt.Sprintf("%s: hide %s: %v", baseTitle, discsDir, err))
		// Not fatal: the dot-prefix alone still keeps it out of most
		// frontends' listings, so still write the m3u.
	}

	m3uPath := filepath.Join(sysDir, invalidFileNameChars.ReplaceAllString(baseTitle, "_")+".m3u")
	content := strings.Join(relPaths, "\n") + "\n"
	// Rewrite only when it changed (re-running must not write anything).
	if old, err := os.ReadFile(m3uPath); err != nil || string(old) != content {
		if err := os.WriteFile(m3uPath, []byte(content), 0o644); err != nil {
			errs = append(errs, fmt.Sprintf("%s: write %s: %v", baseTitle, m3uPath, err))
			return nil, linked, errs
		}
	}
	// Record it, so a set that goes away has its playlist removed and
	// verification checks it (it has no source file: kind "generated").
	relM3U, err := filepath.Rel(opts.OutputRoot, m3uPath)
	if err == nil {
		err = db.UpsertLinkState(store.LinkStateRecord{
			ProfileID: profileIDOf(opts), SystemID: systemID, GameKey: m3uKey(baseTitle), TargetPath: relM3U,
			SourceKind: "generated", LinkType: "copy", LinkStatus: "ok",
		})
	}
	if err != nil {
		errs = append(errs, fmt.Sprintf("%s: record %s: %v", baseTitle, m3uPath, err))
	}

	item := &lplItem{Path: m3uPath, Label: baseTitle, CorePath: "DETECT", CoreName: "DETECT", CRC32: "DETECT", DBName: dbName + ".lpl"}
	return item, linked, errs
}
