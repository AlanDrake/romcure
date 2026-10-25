// Package rules implements grouping (P/C, clone lists, title normalization)
// and 1G1R selection from user preferences and overrides. See spec §2.9.
package rules

import (
	"crypto/sha256"
	"encoding/hex"
	"regexp"
	"strings"
)

// GroupingSource records which method established a grouping, mirroring
// the game_groups.grouping_source column. Spec §2.9's priority order is
// DAT P/C first, then an imported clone list, then title normalization —
// GroupCloneList has no implementation yet (needs a Retool clone-list
// importer, not built yet); Group below skips straight from DAT P/C to
// title normalization until it exists.
type GroupingSource string

const (
	GroupingDATParentClone     GroupingSource = "dat_pc"
	GroupingCloneList          GroupingSource = "clone_list"
	GroupingTitleNormalization GroupingSource = "title_normalization"
)

// GroupInput is one game variant's data needed for grouping, independent
// of which method ends up placing it.
type GroupInput struct {
	// GameKey is this variant's library.GameKey. Required, and must be
	// unique within the batch passed to Group.
	GameKey string
	// DatGameName is this variant's DAT <game name="..."> — the key other
	// entries' CloneOf refers to. "" if this variant isn't DAT-matched.
	DatGameName string
	// CloneOf is this variant's DAT cloneof target (another entry's
	// DatGameName), "" if it isn't a clone (or isn't DAT-matched at all).
	CloneOf string
	// FromParentCloneDAT is true when the DAT that matched this variant has
	// parent/clone data; only then does an empty CloneOf make it a game of its
	// own. A plain DAT's games are grouped by title like unmatched ones.
	FromParentCloneDAT bool
	// Title is the best-known display title, used as the title-
	// normalization fallback's grouping input when DAT P/C data can't
	// place this variant.
	Title string
}

// GroupAssignment is the result of grouping one GroupInput.
type GroupAssignment struct {
	GroupKey string
	Source   GroupingSource
}

// Group assigns every input a GroupAssignment, trying DAT parent/clone
// data first and falling back to title normalization for anything it
// can't place (spec §2.9). The map is keyed by GameKey.
func Group(inputs []GroupInput) map[string]GroupAssignment {
	result := groupByDATParentClone(inputs)

	var remaining []GroupInput
	for _, in := range inputs {
		if _, ok := result[in.GameKey]; !ok {
			remaining = append(remaining, in)
		}
	}
	for key, assignment := range groupByTitleNormalization(remaining) {
		result[key] = assignment
	}

	return result
}

// groupByDATParentClone groups inputs using DAT cloneof relationships: a
// clone's group key is its parent's GameKey; a standalone or parent
// game's group key is its own GameKey. An input whose CloneOf target
// isn't present in this batch is left unassigned (returned map has no
// entry for its GameKey) so the caller can fall back to another method.
func groupByDATParentClone(inputs []GroupInput) map[string]GroupAssignment {
	byDatName := make(map[string]GroupInput, len(inputs))
	for _, in := range inputs {
		if in.DatGameName != "" {
			byDatName[in.DatGameName] = in
		}
	}

	result := make(map[string]GroupAssignment)
	for _, in := range inputs {
		if in.DatGameName == "" || (in.CloneOf == "" && !in.FromParentCloneDAT) {
			continue
		}
		if in.CloneOf == "" {
			result[in.GameKey] = GroupAssignment{GroupKey: in.GameKey, Source: GroupingDATParentClone}
			continue
		}
		if parent, ok := byDatName[in.CloneOf]; ok {
			result[in.GameKey] = GroupAssignment{GroupKey: parent.GameKey, Source: GroupingDATParentClone}
		}
	}
	return result
}

// groupByTitleNormalization groups inputs by their normalized title.
// Fallback when no DAT P/C (or, eventually, clone-list) data placed a
// variant — labeled heuristic in the UI since titles can coincidentally
// collide or legitimately differ across regions for the same game
// (spec §2.9).
func groupByTitleNormalization(inputs []GroupInput) map[string]GroupAssignment {
	result := make(map[string]GroupAssignment, len(inputs))
	for _, in := range inputs {
		result[in.GameKey] = GroupAssignment{
			GroupKey: titleGroupKey(NormalizeTitle(in.Title)),
			Source:   GroupingTitleNormalization,
		}
	}
	return result
}

func titleGroupKey(normalizedTitle string) string {
	sum := sha256.Sum256([]byte(normalizedTitle))
	return "title:" + hex.EncodeToString(sum[:])
}

// NormalizeTitle strips parenthesized/bracketed tags (region, language,
// revision, dump-status flags, ...) and normalizes whitespace and case,
// so e.g. "Super Mario Bros. (USA).nes"-derived and "super mario bros.
// (Europe)"-derived titles both normalize to the same grouping key.
func NormalizeTitle(title string) string {
	cut := len(title)
	if i := strings.IndexAny(title, "(["); i >= 0 {
		cut = i
	}
	t := strings.TrimSpace(title[:cut])
	t = strings.Join(strings.Fields(t), " ")
	// TOSEC puts the version in the title ("Game v1.2 (1993)..."); versions
	// of one game must group together.
	t = trailingVersion.ReplaceAllString(t, "")
	return strings.ToLower(t)
}

var trailingVersion = regexp.MustCompile(`(?i)\s+(?:v\d[\w.]*|rev\s?\d[\w.]*)$`)
