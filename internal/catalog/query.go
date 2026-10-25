package catalog

import (
	"sort"
	"strings"

	"romcure/internal/store"
)

// Filter selects and orders games. A zero Filter lists every game.
type Filter struct {
	OutputID   int64    // 0 = no output context (ticks are ignored)
	Systems    []string // empty = all
	Query      string
	Show       string // "" or "all" | "included" | "excluded"
	Several    bool   // only games with more than one version
	Choices    bool   // only games with a tick of yours on this output or a version you picked
	Unverified bool   // only games the DAT did not verify
	Sort       string // "name" (default) | "system" | "size"
	Desc       bool
}

// GameRow is one game with its state on one output.
type GameRow struct {
	Key           string // "g:<group id>" or "set:<system>:<base title>"
	SystemID      string
	Name          string
	Version       string // the picked variant's region/language/revision tags
	Versions      int
	Discs         int
	Confidence    string // dat_verified | heuristic | unidentified
	VersionPicked bool   // the user chose the version
	Size          int64
	SizeEstimated bool
	Included      bool
	Explicit      bool // the tick is the user's, not inherited
}

// Page is one slice of a filtered list.
type Page struct {
	Total        int // games matching the filter
	Included     int // of those, on the output
	OtherSystems int // search hits outside Filter.Systems
	Rows         []GameRow
}

// GroupRef names a game group to tick, with its system.
type GroupRef = store.GroupRef

// state reports whether a row is on the output (all of its discs) and whether
// any of its discs carries a tick of the user's.
func state(in *store.Inclusion, r *Row) (included, explicit bool) {
	if in == nil {
		return false, false
	}
	included = true
	for _, g := range r.GroupIDs {
		if !in.Includes(r.SystemID, g) {
			included = false
		}
		if _, ok := in.Groups[g]; ok {
			explicit = true
		}
	}
	return included, explicit
}

// match returns the rows that pass the filter's conditions, in catalog (name)
// order, plus the number of rows that match everything except the system
// restriction when a search is active.
func (c *Catalog) match(f Filter, in *store.Inclusion) (out []*Row, other int) {
	var toks []string
	if q := Fold(f.Query); q != "" {
		toks = strings.Split(q, " ")
	}
	var systems map[string]bool
	if len(f.Systems) > 0 {
		systems = make(map[string]bool, len(f.Systems))
		for _, s := range f.Systems {
			systems[s] = true
		}
	}
	for _, r := range c.rows {
		hit := true
		for _, t := range toks {
			if !strings.Contains(r.search, t) {
				hit = false
				break
			}
		}
		if !hit {
			continue
		}
		if systems != nil && !systems[r.SystemID] {
			if len(toks) > 0 {
				other++
			}
			continue
		}
		if f.Several && r.Versions < 2 {
			continue
		}
		if f.Unverified && r.Confidence == "dat_verified" {
			continue
		}
		if f.Choices {
			_, explicit := state(in, r)
			if !explicit && !r.VersionPicked {
				continue
			}
		}
		if (f.Show == "included" || f.Show == "excluded") && in != nil {
			if inc, _ := state(in, r); inc != (f.Show == "included") {
				continue
			}
		}
		out = append(out, r)
	}
	return out, other
}

func sortRows(rows []*Row, f Filter) {
	if f.Sort == "" || f.Sort == "name" {
		if f.Desc {
			for i, j := 0, len(rows)-1; i < j; i, j = i+1, j-1 {
				rows[i], rows[j] = rows[j], rows[i]
			}
		}
		return
	}
	sort.SliceStable(rows, func(i, j int) bool {
		a, b := rows[i], rows[j]
		var less, eq bool
		switch f.Sort {
		case "size":
			less, eq = a.Size < b.Size, a.Size == b.Size
		default: // system
			less, eq = a.SystemID < b.SystemID, a.SystemID == b.SystemID
		}
		if eq {
			return a.sortName < b.sortName
		}
		return less != f.Desc
	})
}

// Page lists one slice of the filtered games with their state on the output
// that in describes (nil = none).
func (c *Catalog) Page(f Filter, in *store.Inclusion, offset, limit int) Page {
	rows, other := c.match(f, in)
	sortRows(rows, f)
	p := Page{Total: len(rows), OtherSystems: other}
	if in != nil {
		for _, r := range rows {
			if inc, _ := state(in, r); inc {
				p.Included++
			}
		}
	}
	if offset < 0 {
		offset = 0
	}
	end := offset + limit
	if end > len(rows) {
		end = len(rows)
	}
	for i := offset; i < end; i++ {
		inc, explicit := state(in, rows[i])
		r := rows[i]
		p.Rows = append(p.Rows, GameRow{Key: r.Key, SystemID: r.SystemID, Name: r.Name, Version: r.Version,
			Versions: r.Versions, Discs: r.Discs, Confidence: r.Confidence, VersionPicked: r.VersionPicked,
			Size: r.Size, SizeEstimated: r.SizeEstimated, Included: inc, Explicit: explicit})
	}
	return p
}

// SystemCount is one system's share of the library on an output.
type SystemCount struct {
	SystemID string
	Total    int
	Included int
}

// Counts returns games and included games per system.
func (c *Catalog) Counts(in *store.Inclusion) []SystemCount {
	idx := map[string]int{}
	var out []SystemCount
	for _, s := range c.systems {
		idx[s] = len(out)
		out = append(out, SystemCount{SystemID: s})
	}
	for _, r := range c.rows {
		sc := &out[idx[r.SystemID]]
		sc.Total++
		if inc, _ := state(in, r); inc {
			sc.Included++
		}
	}
	return out
}

// Groups returns the game groups of the given rows (every disc of a set).
func (c *Catalog) Groups(rows []*Row) []GroupRef {
	var refs []GroupRef
	for _, r := range rows {
		for _, g := range r.GroupIDs {
			refs = append(refs, GroupRef{ID: g, SystemID: r.SystemID})
		}
	}
	return refs
}

// Matching returns the rows a filter matches (sorted as the list is).
func (c *Catalog) Matching(f Filter, in *store.Inclusion) []*Row {
	rows, _ := c.match(f, in)
	sortRows(rows, f)
	return rows
}

// ByKeys returns the rows with the given keys; unknown keys are skipped.
func (c *Catalog) ByKeys(keys []string) []*Row {
	var rows []*Row
	for _, k := range keys {
		if r, ok := c.byKey[k]; ok {
			rows = append(rows, r)
		}
	}
	return rows
}

// CountIncluded returns how many of rows are on the output.
func CountIncluded(rows []*Row, in *store.Inclusion) int {
	n := 0
	for _, r := range rows {
		if inc, _ := state(in, r); inc {
			n++
		}
	}
	return n
}

// IncludedBytes sums the size of the rows on the output and reports whether
// any figure is an upper bound.
func (c *Catalog) IncludedBytes(in *store.Inclusion) (bytes int64, estimated bool, count int) {
	for _, r := range c.rows {
		if inc, _ := state(in, r); inc {
			bytes += r.Size
			count++
			estimated = estimated || r.SizeEstimated
		}
	}
	return
}
