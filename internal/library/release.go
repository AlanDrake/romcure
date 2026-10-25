package library

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"romcure/internal/rules"
)

// A multi-disc game is chosen as one release (spec M7): every disc from the
// same dump set and region, rather than each disc picking its own best
// version, which could mix a USA disc 1 with a European disc 2, or one
// cracker's side A with another's side B.

// dumpQualityFlags are TOSEC flags that describe one dump of a release, not
// the release: [!] verified, [a2] alternate, [b] bad, [o] overdump. The sides
// of one release are often different alternates.
var dumpQualityFlags = regexp.MustCompile(`(?i)\s*\[(?:!|a\d*|b\d*|o\d*)(?:\s[^\]]*)?\]`)

// revisionTag is "(Rev 1)", "(Rev A)": Redump sometimes revises a single disc of
// a set, so the revision doesn't split a release.
var revisionTag = regexp.MustCompile(`(?i)\s*\(rev\s+[^)]*\)`)

// diskLabel is a TOSEC tag that names one disk of a set in words rather than a
// number: "(Disk A)", "(Install Disk)", "(Savedisk)", "(Boot)", "(Master A)",
// "(Reel 2)", or a lone letter "(b)".
var diskLabel = regexp.MustCompile(`(?i)\s*\((?:(?:[^()]*\s)?(?:\w*dis[ck]|boot|master|reel)\b[^()]*|[a-z])\)`)

// ReleaseKey names the release a disc belongs to, from its DAT name or file
// name: the name without its disc or side tag or disk label, revision and
// dump-quality flags, case and spacing folded. Discs with the same key are one
// release. It is meant for the discs of one game (a single-disc game is its
// own release); ok is false only for an empty name.
func ReleaseKey(name string) (key string, ok bool) {
	base := name
	if _, b, tagged := ParseDiscTag(name); tagged {
		base = b
	}
	base = diskLabel.ReplaceAllString(base, "")
	base = dumpQualityFlags.ReplaceAllString(base, "")
	base = revisionTag.ReplaceAllString(base, "")
	key = strings.ToLower(strings.Join(strings.Fields(base), " "))
	return key, key != ""
}

// discChoice is one disc group of a sync, as alignReleases sees it.
type discChoice struct {
	disc       int
	candidates []rules.Candidate
	names      map[string]string // game key -> DAT or file name (what ReleaseKey reads)
	selected   string
	reason     string
	override   bool // the user chose this disc's version
}

// release is the discs one release has, by disc number. total is the number
// of discs its names state ("Disk 1 of 2"), 0 when they don't say.
type release struct {
	key   string
	discs map[int][]rules.Candidate
	total int
}

// complete reports whether the release has all its discs: 1 to the total its
// names state, or else every disc the game has in any release (setLast).
func (r release) complete(setLast int) bool {
	last := setLast
	if r.total > 0 {
		last = r.total
	}
	for n := 1; n <= last; n++ {
		if len(r.discs[n]) == 0 {
			return false
		}
	}
	return true
}

var discOfTotal = regexp.MustCompile(`(?i)\((?:dis[ck]|tape)\s+\d+\s+of\s+(\d+)\)`)

// discTotal is N in a "(Disk 1 of N)" tag, 0 when there is none. Sides number
// the parts past N, so a name with a side gives no total.
func discTotal(name string) int {
	if strings.Contains(strings.ToLower(name), "side ") {
		return 0
	}
	m := discOfTotal.FindStringSubmatch(name)
	if m == nil {
		return 0
	}
	n, _ := strconv.Atoi(m[1])
	return n
}

// alignReleases makes the discs of one multi-disc game come from one release.
// The release is the one the user chose a disc from, if any; otherwise the
// best by the usual rules among the complete releases (every disc from 1 to
// the total its names state, else every disc the game has), or among those
// with the most discs when none is complete. Each disc then takes that
// release's best dump of it; a disc the release lacks, or one the user chose,
// keeps its own pick. Picks and reasons are updated in place.
func alignReleases(set []*discChoice, prefs rules.Preferences) {
	if len(set) < 2 {
		return
	}
	byKey := map[string]*release{}
	keyOf := map[string]string{} // game key -> release key
	for _, d := range set {
		for _, c := range d.candidates {
			k, ok := ReleaseKey(d.names[c.GameKey])
			if !ok {
				continue
			}
			keyOf[c.GameKey] = k
			r := byKey[k]
			if r == nil {
				r = &release{key: k, discs: map[int][]rules.Candidate{}}
				byKey[k] = r
			}
			r.discs[d.disc] = append(r.discs[d.disc], c)
			if n := discTotal(d.names[c.GameKey]); n > r.total {
				r.total = n
			}
		}
	}
	if len(byKey) < 2 {
		return // one release: the per-disc picks already agree
	}

	var chosen *release
	why := ""
	// The user's choice on any disc decides, lowest disc first.
	sort.Slice(set, func(i, j int) bool { return set[i].disc < set[j].disc })
	for _, d := range set {
		if d.override {
			if r := byKey[keyOf[d.selected]]; r != nil {
				chosen, why = r, fmt.Sprintf("the release of the disc you chose (disc %d)", d.disc)
				break
			}
		}
	}
	if chosen == nil {
		setLast := 0
		for _, d := range set {
			if d.disc > setLast {
				setLast = d.disc
			}
		}
		chosen, why = bestRelease(byKey, setLast, prefs)
	}

	for _, d := range set {
		if d.override {
			continue
		}
		cands := chosen.discs[d.disc]
		if len(cands) == 0 {
			d.reason += fmt.Sprintf(" The release used for the other discs has no disc %d, so this one comes from another release.", d.disc)
			continue
		}
		pick, _ := rules.Select(cands, prefs)
		if pick != d.selected {
			d.selected = pick
		}
		d.reason = "Every disc from one release: " + why + "."
	}
}

// bestRelease picks among releases: complete ones first (or those with the
// most discs when none is complete), then by the usual rules applied to each
// release's best first disc.
func bestRelease(byKey map[string]*release, setLast int, prefs rules.Preferences) (*release, string) {
	var pool []*release
	for _, r := range byKey {
		if r.complete(setLast) {
			pool = append(pool, r)
		}
	}
	why := "the preferred complete release"
	if len(pool) == 0 {
		most := 0
		for _, r := range byKey {
			if n := len(r.discs); n > most {
				most, pool = n, nil
			}
			if len(r.discs) == most {
				pool = append(pool, r)
			}
		}
		why = "no release has every disc; this one has the most"
	}
	// Stable input for Select, whose ties go to the first candidate.
	sort.Slice(pool, func(i, j int) bool { return pool[i].key < pool[j].key })
	reps := make([]rules.Candidate, 0, len(pool))
	owner := map[string]*release{}
	for _, r := range pool {
		first := r.discs[lowestDisc(r)]
		key, _ := rules.Select(first, prefs)
		for _, c := range first {
			if c.GameKey == key {
				reps = append(reps, c)
				owner[c.GameKey] = r
			}
		}
	}
	key, _ := rules.Select(reps, prefs)
	if len(pool) == 1 && why == "the preferred complete release" {
		why = "the only complete release"
	}
	return owner[key], why
}

func lowestDisc(r *release) int {
	low := 0
	for n := range r.discs {
		if low == 0 || n < low {
			low = n
		}
	}
	return low
}
