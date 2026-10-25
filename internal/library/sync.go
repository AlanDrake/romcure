package library

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"strings"

	"romcure/internal/discsheet"
	"romcure/internal/match"
	"romcure/internal/rules"
	"romcure/internal/store"
)

// discDataTrackExtensions are raw disc data/audio track files that are
// never a game in their own right — they're referenced by a sibling
// sheet (.cue, .gdi) that names the actual game (spec §2.5: "every
// member file must be linked ... never link the .cue alone", which only
// makes sense if the .cue, not a .bin, is the thing 1G1R selects).
// Without this, a .bin and its .cue would independently match the same
// heuristic title and compete head-to-head for 1G1R selection, with no
// guarantee the .cue wins. Only applies when nothing matched by DAT
// hash — a DAT that explicitly names one of these as the primary ROM
// still gets to pick it.
var discDataTrackExtensions = map[string]bool{
	".bin": true, ".raw": true, ".sub": true, ".img": true,
}

// itemExtension returns the extension of the actual matched unit: the
// archive member's name for an archive, the file's own path otherwise.
func itemExtension(it match.Item) string {
	if it.MemberName != "" {
		return strings.ToLower(filepath.Ext(it.MemberName))
	}
	return strings.ToLower(filepath.Ext(it.Path))
}

// originalFilenameStem returns the matched unit's own filename (the
// archive member's name for an archive, the file's own path otherwise)
// with its extension removed — the original name before any heuristic
// tag-stripping, used to recover tags (like a disc number) that
// match.Result.Title deliberately doesn't carry.
func originalFilenameStem(it match.Item) string {
	name := it.MemberName
	if name == "" {
		name = filepath.Base(it.Path)
	}
	return strings.TrimSuffix(name, filepath.Ext(name))
}

// ReportEntry is one line of a sync Report.
type ReportEntry struct {
	GameKey  string
	GroupKey string
	Title    string
	Reason   string
}

// Report summarizes one Sync run (spec M2 bullet 5): every group's
// selected variant, every variant that lost its group's 1G1R pick, every
// unidentified file, and every group a manual override changed the
// outcome for.
type Report struct {
	Selected   []ReportEntry
	Excluded   []ReportEntry
	Unmatched  []ReportEntry
	Overridden []ReportEntry
}

// Sync runs the Rule Engine for one system: browses root (spec §2.6),
// groups the results (rules.Group), applies 1G1R preferences and any
// manual override (rules.Select, store.GetOverride), and upserts the
// outcome into game_groups/games. Upserting by (system_id, group_key)
// and (system_id, game_key) — never delete-then-recreate — is what makes
// re-running with unchanged inputs a no-op and keeps overrides (which
// reference a group's DB id) valid across a DAT update, even one that
// renames entries (spec M2 exit criterion): group_key and game_key are
// content-hash based (library.GameKey), not name based, so a rename
// never changes them.
func Sync(db *store.DB, root, systemID string, prefs rules.Preferences) (Report, error) {
	return SyncRoots(db, []string{root}, systemID, prefs)
}

// SyncRoots is Sync for a system whose games live in several folders: they are
// browsed together, so the same game in two folders is one game (the first
// folder listed supplies the file) and 1G1R picks across all of them.
func SyncRoots(db *store.DB, roots []string, systemID string, prefs rules.Preferences) (Report, error) {
	where := strings.Join(roots, ", ")
	var items []match.Item
	for _, root := range roots {
		got, _, err := match.Browse(db, root, systemID)
		if err != nil {
			return Report{}, fmt.Errorf("sync %s/%s: %w", systemID, where, err)
		}
		items = append(items, got...)
	}

	type itemInfo struct {
		title string
		item  match.Item
	}
	byGameKey := make(map[string]itemInfo, len(items))

	inputs := make([]rules.GroupInput, 0, len(items))
	var report Report
	// Tracks named by a sibling raw .cue/.gdi belong to that sheet's game,
	// whether or not the DAT verified them individually (Redump lists every
	// track .bin as its own hash-verified ROM).
	claimed := sheetClaimedTracks(items)

	for _, it := range items {
		if claimed[trackKey(it)] {
			continue
		}
		if it.Result.Confidence != match.ConfidenceDATVerified && discDataTrackExtensions[itemExtension(it)] {
			continue
		}

		gameKey := GameKey([]string{it.ContentHash})
		if _, dup := byGameKey[gameKey]; dup {
			continue // the same content again (another copy or folder): the first one is used
		}

		// A multi-disc game's discs are not variants competing for a
		// single 1G1R pick — every disc is required (spec §2.5). The
		// heuristic Title has already dropped every parenthesized tag,
		// disc number included, so two discs of the same game would
		// otherwise normalize to the identical title and only one would
		// survive selection. Re-derive the disc number from the original
		// filename (still intact there — Title isn't) and fold it back
		// into the display name (parens, so internal/profile's
		// ParseDiscTag can find it again downstream) and, separately,
		// into the grouping title: NormalizeTitle cuts at the first "("
		// or "[", so a parenthesized disc suffix there would just get
		// stripped right back out — it has to be bracket-free to survive.
		displayTitle := it.Result.Title
		groupingTitle := it.Result.Title
		if discNum, base, ok := ParseDiscTag(displayTitle); ok {
			// A DAT game name (Redump) already carries its disc tag; keep it
			// as the display name and make the grouping title bracket-free
			// (normalization cuts at the first bracket, and a DAT name has its
			// region tags before the disc tag).
			groupingTitle = fmt.Sprintf("%s disc%d", rules.NormalizeTitle(base), discNum)
		} else if discNum, _, ok := ParseDiscTag(originalFilenameStem(it)); ok {
			displayTitle = fmt.Sprintf("%s (Disc %d)", displayTitle, discNum)
			groupingTitle = fmt.Sprintf("%s disc%d", groupingTitle, discNum)
		}
		byGameKey[gameKey] = itemInfo{title: displayTitle, item: it}

		datGameName := ""
		if it.Result.Confidence == match.ConfidenceDATVerified {
			datGameName = it.Result.Title
		}
		inputs = append(inputs, rules.GroupInput{
			GameKey:            gameKey,
			DatGameName:        datGameName,
			CloneOf:            it.Result.CloneOf,
			FromParentCloneDAT: it.Result.ParentCloneDAT,
			Title:              groupingTitle,
		})

		if it.Result.Confidence == match.ConfidenceUnidentified {
			reason := "no DAT match and no recognizable naming convention"
			if it.Bundled {
				reason = "a program in several files; named from its archive"
			}
			report.Unmatched = append(report.Unmatched, ReportEntry{
				GameKey: gameKey, Title: it.Result.Title, Reason: reason,
			})
		}
	}

	assignments := rules.Group(inputs)

	// Collect candidates per group.
	groupKeys := make(map[string][]string) // groupKey -> []gameKey, in first-seen order
	groupSource := make(map[string]rules.GroupingSource)
	for _, in := range inputs {
		a := assignments[in.GameKey]
		groupKeys[a.GroupKey] = append(groupKeys[a.GroupKey], in.GameKey)
		groupSource[a.GroupKey] = a.Source
	}

	for groupKey, gameKeys := range groupKeys {
		candidates := make([]rules.Candidate, 0, len(gameKeys))
		for _, gk := range gameKeys {
			res := byGameKey[gk].item.Result
			candidates = append(candidates, rules.Candidate{
				GameKey:        gk,
				Confidence:     res.Confidence,
				Region:         res.Region,
				Language:       res.Language,
				Revision:       res.Revision,
				IsVerifiedDump: res.IsVerifiedDump,
				IsBeta:         res.IsBeta,
				IsProto:        res.IsProto,
				IsDemo:         res.IsDemo,
				VariantPenalty: res.VariantPenalty,
			})
		}

		// The group's display name follows whichever variant ends up
		// selected, computed below — but the group row has to exist
		// first since overrides reference its id. Use the first
		// candidate's title as a placeholder and correct it once the
		// pick is known.
		groupID, err := db.UpsertGameGroup(store.GameGroupRecord{
			SystemID:       systemID,
			GroupKey:       groupKey,
			GroupingSource: string(groupSource[groupKey]),
			DisplayName:    byGameKey[gameKeys[0]].title,
		})
		if err != nil {
			return Report{}, fmt.Errorf("sync %s/%s: %w", systemID, where, err)
		}

		overrideKey, overrideReason, hasOverride, err := db.GetOverride(groupID)
		if err != nil {
			return Report{}, fmt.Errorf("sync %s/%s: %w", systemID, where, err)
		}
		if !hasOverride {
			// A DAT import can regroup games (title groups becoming P/C
			// groups), leaving the override on a group that no longer holds
			// its game. The override is keyed by content, so follow the game.
			if oldGroup, found, err := db.FindMovedOverride(systemID, gameKeys); err != nil {
				return Report{}, fmt.Errorf("sync %s/%s: %w", systemID, where, err)
			} else if found && oldGroup != groupID {
				if err := db.MoveOverride(oldGroup, groupID); err != nil {
					return Report{}, fmt.Errorf("sync %s/%s: move override: %w", systemID, where, err)
				}
				overrideKey, overrideReason, hasOverride, err = db.GetOverride(groupID)
				if err != nil {
					return Report{}, fmt.Errorf("sync %s/%s: %w", systemID, where, err)
				}
			}
		}

		var selectedKey, reason string
		overrideApplied := false
		if hasOverride && containsGameKey(candidates, overrideKey) {
			selectedKey, reason = overrideKey, "Your choice ("+overrideReason+")."
			overrideApplied = true
		} else {
			selectedKey, reason = rules.Select(candidates, prefs)
		}

		if _, err := db.UpsertGameGroup(store.GameGroupRecord{
			SystemID:         systemID,
			GroupKey:         groupKey,
			GroupingSource:   string(groupSource[groupKey]),
			DisplayName:      byGameKey[selectedKey].title,
			IsManualOverride: overrideApplied,
			SelectionReason:  reason,
		}); err != nil {
			return Report{}, fmt.Errorf("sync %s/%s: refresh display name: %w", systemID, where, err)
		}

		for _, c := range candidates {
			it := byGameKey[c.GameKey].item
			isSelected := c.GameKey == selectedKey

			ultimateKind := "raw_file"
			if it.MemberName != "" {
				ultimateKind = "archive"
			}

			var datEntryID sql.NullInt64
			if it.Result.DatEntryID != 0 {
				datEntryID = sql.NullInt64{Int64: it.Result.DatEntryID, Valid: true}
			}

			if _, err := db.UpsertGame(store.GameRecord{
				GroupID:            groupID,
				SystemID:           systemID,
				GameKey:            c.GameKey,
				DatEntryID:         datEntryID,
				Confidence:         string(c.Confidence),
				Region:             c.Region,
				Language:           c.Language,
				Revision:           c.Revision,
				IsVerifiedDump:     c.IsVerifiedDump,
				IsBetaProtoDemo:    c.IsBeta || c.IsProto || c.IsDemo,
				IsSelected:         isSelected,
				UltimateSourceKind: ultimateKind,
				UltimateSourceRef:  it.Path,
				MemberName:         it.MemberName,
			}); err != nil {
				return Report{}, fmt.Errorf("sync %s/%s: %w", systemID, where, err)
			}

			entry := ReportEntry{GameKey: c.GameKey, GroupKey: groupKey, Title: byGameKey[c.GameKey].title, Reason: reason}
			if isSelected {
				report.Selected = append(report.Selected, entry)
				if overrideApplied {
					report.Overridden = append(report.Overridden, entry)
				}
			} else {
				report.Excluded = append(report.Excluded, entry)
			}
		}
	}

	// Forget games that no longer exist under this root (deleted files, or an
	// identity change such as a DAT update verifying a disc under a new key).
	keep := make(map[string]bool, len(byGameKey))
	for k := range byGameKey {
		keep[k] = true
	}
	for _, root := range roots {
		if _, err := db.PruneGamesUnderRoot(systemID, root, keep); err != nil {
			return Report{}, fmt.Errorf("sync %s/%s: prune stale games: %w", systemID, where, err)
		}
	}

	return report, nil
}

func containsGameKey(candidates []rules.Candidate, gameKey string) bool {
	for _, c := range candidates {
		if c.GameKey == gameKey {
			return true
		}
	}
	return false
}

// trackKey identifies a file, or an archive member, for track-claim lookups.
func trackKey(it match.Item) string {
	k := strings.ToLower(filepath.Clean(it.Path))
	if it.MemberName != "" {
		k += "|" + strings.ToLower(it.MemberName)
	}
	return k
}

// sheetClaimedTracks returns the files referenced by raw .cue/.gdi sheets
// among items, and the track files that sit in the same archive as a
// .cue/.gdi member (the archive holds one disc: its sheet is the game, the
// tracks are not). A sheet that can't be read claims nothing.
func sheetClaimedTracks(items []match.Item) map[string]bool {
	claimed := map[string]bool{}
	archivesWithSheet := map[string]bool{}
	for _, it := range items {
		ext := itemExtension(it)
		if ext != ".cue" && ext != ".gdi" {
			continue
		}
		if it.MemberName != "" {
			archivesWithSheet[strings.ToLower(filepath.Clean(it.Path))] = true
			continue
		}
		files, err := discsheet.ReferencedFiles(it.Path)
		if err != nil {
			continue
		}
		for _, f := range files {
			claimed[strings.ToLower(filepath.Clean(filepath.Join(filepath.Dir(it.Path), f)))] = true
		}
	}
	for _, it := range items {
		if it.MemberName != "" && archivesWithSheet[strings.ToLower(filepath.Clean(it.Path))] && discDataTrackExtensions[itemExtension(it)] {
			claimed[trackKey(it)] = true
		}
	}
	return claimed
}
