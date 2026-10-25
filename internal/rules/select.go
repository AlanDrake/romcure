package rules

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"romcure/internal/match"
)

// Preferences controls 1G1R selection among a group's candidates
// (spec §2.9). RegionPriority/LanguagePriority are ordered lists —
// earlier entries win; a candidate whose value isn't in the list at all
// sorts after every explicit preference. Revision has no natural
// "priority list" (DAT revision strings are open-ended, not a small
// fixed vocabulary like regions), so it's a single toggle instead:
// PreferLatestRevision prefers the higher of two candidates' extracted
// revision numbers when region and language are tied.
type Preferences struct {
	RegionPriority       []string
	LanguagePriority     []string
	PreferLatestRevision bool
	RequireVerified      bool
	ExcludeBeta          bool
	ExcludeProto         bool
	ExcludeDemo          bool
}

// Candidate is one game variant competing for 1G1R selection within a
// group (spec §2.9's "keep a single preferred variant per game").
type Candidate struct {
	GameKey        string
	Confidence     match.Confidence
	Region         string
	Language       string
	Revision       string
	IsVerifiedDump bool
	IsBeta         bool
	IsProto        bool
	IsDemo         bool
	// VariantPenalty: 0 clean release, 1 alternate dump, 2 modified or
	// damaged (cracked, hacked, bad dump...). Lower is preferred.
	VariantPenalty int
}

// Select picks one candidate from group per prefs, returning its GameKey
// and a human-readable reason (same pattern as internal/link.SelectTier:
// every selection explains itself, spec §5's transparency rule). Exclusion
// rules (ExcludeBeta/Proto/Demo, RequireVerified) are applied first,
// unless doing so would eliminate every candidate — a group is never
// left with nothing selected just because every variant happens to be a
// beta, say. Among what's left, DAT-verified beats heuristic beats
// unidentified, then RegionPriority, then LanguagePriority; ties keep
// the first candidate in group's order (deterministic, not arbitrary).
func Select(group []Candidate, prefs Preferences) (gameKey string, reason string) {
	if len(group) == 0 {
		return "", "empty group"
	}

	filtered := applyExclusions(group, prefs)
	fellBack := len(filtered) == len(group) && anyExcludable(group, prefs)

	best := filtered[0]
	for _, c := range filtered[1:] {
		if less(c, best, prefs) {
			best = c
		}
	}

	return best.GameKey, buildReason(best, len(group), len(filtered), fellBack)
}

func applyExclusions(group []Candidate, prefs Preferences) []Candidate {
	var filtered []Candidate
	for _, c := range group {
		if prefs.ExcludeBeta && c.IsBeta {
			continue
		}
		if prefs.ExcludeProto && c.IsProto {
			continue
		}
		if prefs.ExcludeDemo && c.IsDemo {
			continue
		}
		if prefs.RequireVerified && !c.IsVerifiedDump {
			continue
		}
		filtered = append(filtered, c)
	}
	if len(filtered) == 0 {
		// Never select nothing: if every candidate would be excluded,
		// ignore the exclusion rules for this group rather than leaving
		// it without a selection.
		return group
	}
	return filtered
}

func anyExcludable(group []Candidate, prefs Preferences) bool {
	for _, c := range group {
		if (prefs.ExcludeBeta && c.IsBeta) ||
			(prefs.ExcludeProto && c.IsProto) ||
			(prefs.ExcludeDemo && c.IsDemo) ||
			(prefs.RequireVerified && !c.IsVerifiedDump) {
			return true
		}
	}
	return false
}

func confidenceRank(c match.Confidence) int {
	switch c {
	case match.ConfidenceDATVerified:
		return 0
	case match.ConfidenceHeuristic:
		return 1
	default:
		return 2
	}
}

// priorityRank returns the best (lowest) index in priority of any element
// of value (case-insensitive), or len(priority) if none is listed. value may
// be a comma-separated list such as "USA, Europe" or "En,Fr,De": a
// multi-region release ranks as its most-preferred region.
func priorityRank(value string, priority []string) int {
	best := len(priority)
	for _, part := range strings.Split(value, ",") {
		part = strings.TrimSpace(part)
		for i, p := range priority {
			if i < best && strings.EqualFold(part, p) {
				best = i
			}
		}
	}
	return best
}

// less reports whether a is preferred over b under prefs.
func less(a, b Candidate, prefs Preferences) bool {
	if ra, rb := confidenceRank(a.Confidence), confidenceRank(b.Confidence); ra != rb {
		return ra < rb
	}
	if ra, rb := priorityRank(a.Region, prefs.RegionPriority), priorityRank(b.Region, prefs.RegionPriority); ra != rb {
		return ra < rb
	}
	if ra, rb := priorityRank(a.Language, prefs.LanguagePriority), priorityRank(b.Language, prefs.LanguagePriority); ra != rb {
		return ra < rb
	}
	if a.VariantPenalty != b.VariantPenalty {
		return a.VariantPenalty < b.VariantPenalty
	}
	if prefs.PreferLatestRevision {
		if ra, rb := revisionRank(a.Revision), revisionRank(b.Revision); ra != rb {
			return ra > rb // higher extracted number = later revision = more preferred
		}
	}
	return false
}

var revisionNumberPattern = regexp.MustCompile(`\d+`)

// revisionRank extracts the first run of digits from a revision string
// like "Rev 1" or "v1.1" as a comparable ordinal; -1 if it has none (e.g.
// "Rev A", or no revision tag at all), which always loses to any
// candidate with an actual number when PreferLatestRevision is set.
func revisionRank(revision string) int {
	// Up to three numeric parts are combined so dotted versions compare
	// correctly: "Rev 1" < "v1.1" < "v1.10".
	parts := revisionNumberPattern.FindAllString(revision, 3)
	if len(parts) == 0 {
		return -1
	}
	rank := 0
	for i := 0; i < 3; i++ {
		n := 0
		if i < len(parts) {
			v, err := strconv.Atoi(parts[i])
			if err != nil || v > 999 {
				return -1
			}
			n = v
		}
		rank = rank*1000 + n
	}
	return rank
}

func buildReason(best Candidate, total, afterExclusion int, exclusionsFellBack bool) string {
	// Plain English: this text is shown to users in the library browser and
	// the sync report (spec §5: every selection explains itself).
	var head string
	if total == 1 {
		head = "Only version found"
	} else {
		head = fmt.Sprintf("Best of %d versions", total)
	}
	switch {
	case exclusionsFellBack:
		head += " (every version is a beta, prototype, demo or unverified dump, so those rules were set aside for this game)"
	case afterExclusion < total:
		head += fmt.Sprintf(" (%d left out as beta, prototype, demo or unverified)", total-afterExclusion)
	}

	var why []string
	switch best.Confidence {
	case match.ConfidenceDATVerified:
		why = append(why, "verified against the DAT")
	case match.ConfidenceHeuristic:
		why = append(why, "identified by file name")
	default:
		why = append(why, "not identified")
	}
	if best.Region != "" {
		why = append(why, "region "+best.Region)
	}
	if best.Language != "" {
		why = append(why, "languages "+best.Language)
	}
	if best.Revision != "" {
		why = append(why, best.Revision)
	}
	switch best.VariantPenalty {
	case 1:
		why = append(why, "alternate dump")
	case 2:
		why = append(why, "modified dump (no clean one available)")
	}
	return head + ": " + strings.Join(why, ", ") + "."
}
