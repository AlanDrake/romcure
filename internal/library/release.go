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
// cracker's side A with another's side B. Regions are never mixed (owner's
// rule): a disc that only exists in another region is left out.

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

// sameRegion reports whether discs of two regions may be used together: their
// regions share one ("USA, Europe" and "USA"), one of them is "World", or a
// region is not known (most TOSEC names give none).
func sameRegion(a, b string) bool {
	as, bs := regionSet(a), regionSet(b)
	if len(as) == 0 || len(bs) == 0 || as["world"] || bs["world"] {
		return true
	}
	for r := range as {
		if bs[r] {
			return true
		}
	}
	return false
}

func regionSet(s string) map[string]bool {
	m := map[string]bool{}
	for _, p := range strings.Split(s, ",") {
		if p = strings.ToLower(strings.TrimSpace(p)); p != "" {
			m[p] = true
		}
	}
	return m
}

// discChoice is one disc group of a sync, as alignReleases sees it. A disc
// left out (no disc of a compatible region) has selected "".
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
	key    string
	region string
	discs  map[int][]rules.Candidate
	total  int
}

func (r *release) last(setLast int) int {
	if r.total > 0 {
		return r.total
	}
	return setLast
}

// has reports whether disc n can come from r or, when fill is set, from
// another release of the same region.
func (r *release) has(n int, fill []*release) bool {
	if len(r.discs[n]) > 0 {
		return true
	}
	for _, o := range fill {
		if len(o.discs[n]) > 0 {
			return true
		}
	}
	return false
}

// coverage counts the discs 1..last that r has, with fill's help.
func (r *release) coverage(last int, fill []*release) int {
	n := 0
	for i := 1; i <= last; i++ {
		if r.has(i, fill) {
			n++
		}
	}
	return n
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

// alignReleases makes the discs of one multi-disc game come from one release,
// and never from two regions. The release is the one the user chose a disc
// from, if any; otherwise the best by the usual rules among, in order: the
// complete releases (every disc from 1 to the total their names state, else
// every disc the game has); those a release of the same region completes;
// those with the most discs. Each disc takes the release's best dump of it; a
// disc the release lacks comes from another release of the same region (a
// different crack, say), or is left out. A disc the user chose keeps its pick.
// Picks and reasons are updated in place.
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
				r = &release{key: k, region: c.Region, discs: map[int][]rules.Candidate{}}
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
	// fillers lists, per release, the other releases its missing discs may come from.
	fillers := func(r *release) []*release {
		var out []*release
		for _, o := range byKey {
			if o != r && sameRegion(r.region, o.region) {
				out = append(out, o)
			}
		}
		sort.Slice(out, func(i, j int) bool { return out[i].key < out[j].key })
		return out
	}

	sort.Slice(set, func(i, j int) bool { return set[i].disc < set[j].disc })
	setLast := set[len(set)-1].disc
	var chosen *release
	why := ""
	// The user's choice on any disc decides, lowest disc first.
	for _, d := range set {
		if d.override {
			if r := byKey[keyOf[d.selected]]; r != nil {
				chosen, why = r, fmt.Sprintf("the release of the disc you chose (disc %d)", d.disc)
				break
			}
		}
	}
	if chosen == nil {
		chosen, why = bestRelease(byKey, setLast, fillers, prefs)
	}

	fill := fillers(chosen)
	for _, d := range set {
		if d.override {
			continue
		}
		if cands := chosen.discs[d.disc]; len(cands) > 0 {
			d.selected, _ = rules.Select(cands, prefs)
			d.reason = "Every disc from one release: " + why + "."
			continue
		}
		var cands []rules.Candidate
		for _, o := range fill {
			cands = append(cands, o.discs[d.disc]...)
		}
		if len(cands) > 0 {
			d.selected, _ = rules.Select(cands, prefs)
			d.reason = fmt.Sprintf("The release used for the other discs (%s) has no disc %d, so it comes from another release of the same region.", why, d.disc)
			continue
		}
		d.selected = ""
		d.reason = fmt.Sprintf("Left out: the release used for the other discs (%s) has no disc %d, and no other release of its region does. A disc from another region is never used.", why, d.disc)
	}
}

// bestRelease picks among releases: complete ones first, then those that
// releases of the same region complete, then those covering the most discs;
// among those, by the usual rules applied to each release's best first disc.
func bestRelease(byKey map[string]*release, setLast int, fillers func(*release) []*release, prefs rules.Preferences) (*release, string) {
	var all []*release
	for _, r := range byKey {
		all = append(all, r)
	}
	// Stable input for Select, whose ties go to the first candidate.
	sort.Slice(all, func(i, j int) bool { return all[i].key < all[j].key })

	tiers := []struct {
		why  string
		keep func(r *release) bool
	}{
		{"the preferred complete release", func(r *release) bool {
			return r.coverage(r.last(setLast), nil) == r.last(setLast)
		}},
		{"no release has every disc; this one does with another release of the same region", func(r *release) bool {
			return r.coverage(r.last(setLast), fillers(r)) == r.last(setLast)
		}},
	}
	var pool []*release
	why := ""
	for _, t := range tiers {
		for _, r := range all {
			if t.keep(r) {
				pool = append(pool, r)
			}
		}
		if len(pool) > 0 {
			why = t.why
			break
		}
	}
	if len(pool) == 0 {
		most := 0
		for _, r := range all {
			n := r.coverage(setLast, fillers(r))
			if n > most {
				most, pool = n, nil
			}
			if n == most {
				pool = append(pool, r)
			}
		}
		why = "no release of any region has every disc; this one has the most"
	}

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
