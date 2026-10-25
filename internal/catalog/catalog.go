// Package catalog is the in-memory index behind the Games screen: one compact
// row per game (multi-disc games folded into one row), searchable, filterable
// and pageable without touching the database per keystroke. It has no Wails
// dependency. Ticks are not part of the index; they are resolved on the fly
// from a store.Inclusion, so ticking never rebuilds it.
package catalog

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"romcure/internal/library"
	"romcure/internal/store"
)

// Row is one game as the list shows it.
type Row struct {
	Key           string // "g:<group id>" or "set:<system>:<base title>"
	SystemID      string
	Name          string
	Version       string // the picked variant's region/language/revision tags
	Versions      int    // most versions any of its groups has
	Discs         int
	Confidence    string // of the picked variant; the weakest among discs
	VersionPicked bool   // the user chose a version
	Size          int64
	SizeEstimated bool // an archived disc that will be converted: an upper bound

	GroupIDs []int64
	search   string
	sortName string
}

// Catalog is a snapshot of every game of every system.
type Catalog struct {
	rows    []*Row // ordered by name, then system
	byKey   map[string]*Row
	systems []string // in first-seen order
}

type groupRec struct {
	id       int64
	system   string
	name     string
	versions int
	version  string
	conf     string
	picked   bool
	size     int64
	estimate bool
	search   strings.Builder
}

var discExts = map[string]bool{".cue": true, ".gdi": true, ".iso": true, ".bin": true, ".img": true, ".chd": true, ".rvz": true}

// Load builds the catalog from the database (the state after the last sync).
func Load(db *store.DB) (*Catalog, error) {
	rows, err := db.Query(`
		SELECT gg.id, gg.system_id, gg.display_name, COALESCE(o.override_game_key, ''),
		       g.game_key, COALESCE(e.game_name, ''), g.confidence, COALESCE(g.region, ''),
		       COALESCE(g.language, ''), COALESCE(g.revision, ''), g.is_selected,
		       g.ultimate_source_kind, g.ultimate_source_ref, COALESCE(g.member_name, ''),
		       COALESCE(am.size, f.size, 0)
		FROM game_groups gg
		JOIN games g ON g.group_id = gg.id
		LEFT JOIN dat_entries e ON e.id = g.dat_entry_id
		LEFT JOIN overrides o ON o.group_id = gg.id
		LEFT JOIN files f ON f.path = g.ultimate_source_ref
		LEFT JOIN archive_members am ON am.file_id = f.id AND am.member_name = g.member_name
		ORDER BY gg.id, g.is_selected DESC, g.game_key`)
	if err != nil {
		return nil, fmt.Errorf("load catalog: %w", err)
	}
	defer rows.Close()

	var groups []*groupRec
	var cur *groupRec
	for rows.Next() {
		var (
			gid                                     int64
			sys, name, override, key, datName, conf string
			region, lang, rev                       string
			selected                                bool
			kind, ref, member                       string
			size                                    int64
		)
		if err := rows.Scan(&gid, &sys, &name, &override, &key, &datName, &conf, &region, &lang, &rev,
			&selected, &kind, &ref, &member, &size); err != nil {
			return nil, err
		}
		if cur == nil || cur.id != gid {
			cur = &groupRec{id: gid, system: sys, name: name, picked: override != ""}
			groups = append(groups, cur)
		}
		title := datName
		if title == "" {
			title = member
			if title == "" {
				title = filepath.Base(ref)
			}
		}
		cur.versions++
		cur.search.WriteString(Fold(title))
		cur.search.WriteByte(' ')
		if selected {
			cur.version = joinTags(region, lang, rev)
			cur.conf = conf
			cur.size = size
			ext := strings.ToLower(filepath.Ext(member))
			if member == "" {
				ext = strings.ToLower(filepath.Ext(ref))
			}
			cur.estimate = kind == "archive" && discExts[ext]
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return build(groups), nil
}

func joinTags(parts ...string) string {
	var out []string
	for _, p := range parts {
		if p != "" {
			out = append(out, p)
		}
	}
	return strings.Join(out, " · ")
}

// confRank orders confidence from weakest to strongest.
func confRank(c string) int {
	switch c {
	case "unidentified":
		return 0
	case "heuristic":
		return 1
	}
	return 2
}

func build(groups []*groupRec) *Catalog {
	c := &Catalog{byKey: make(map[string]*Row, len(groups))}
	seenSys := map[string]bool{}

	// Fold the discs of one game (same system, same title without the disc
	// tag) into one row, the way profile.partitionByDisc builds .m3u sets.
	type setKey struct{ sys, base string }
	sets := map[setKey][]*groupRec{}
	baseOf := map[*groupRec]string{}
	for _, g := range groups {
		if _, base, ok := library.ParseDiscTag(g.name); ok {
			base = library.SetTitle(base)
			k := setKey{g.system, strings.ToLower(base)}
			sets[k] = append(sets[k], g)
			baseOf[g] = base
		}
	}
	emitted := map[setKey]bool{}
	for _, g := range groups {
		if !seenSys[g.system] {
			seenSys[g.system] = true
			c.systems = append(c.systems, g.system)
		}
		if base, ok := baseOf[g]; ok {
			k := setKey{g.system, strings.ToLower(base)}
			if len(sets[k]) > 1 {
				if emitted[k] {
					continue
				}
				emitted[k] = true
				c.add(setRow(base, sets[k]))
				continue
			}
		}
		c.add(&Row{
			Key: fmt.Sprintf("g:%d", g.id), SystemID: g.system, Name: g.name, Version: g.version,
			Versions: g.versions, Discs: 1, Confidence: g.conf, VersionPicked: g.picked,
			Size: g.size, SizeEstimated: g.estimate, GroupIDs: []int64{g.id},
			search: Fold(g.name) + " " + g.search.String(),
		})
	}
	sort.SliceStable(c.rows, func(i, j int) bool {
		a, b := c.rows[i], c.rows[j]
		if a.sortName != b.sortName {
			return a.sortName < b.sortName
		}
		return a.SystemID < b.SystemID
	})
	return c
}

func setRow(base string, discs []*groupRec) *Row {
	r := &Row{
		Key:      fmt.Sprintf("set:%s:%s", discs[0].system, strings.ToLower(base)),
		SystemID: discs[0].system, Name: base, Version: discs[0].version, Discs: len(discs),
		Confidence: discs[0].conf,
	}
	var sb strings.Builder
	sb.WriteString(Fold(base))
	sb.WriteByte(' ')
	for _, d := range discs {
		r.GroupIDs = append(r.GroupIDs, d.id)
		if d.versions > r.Versions {
			r.Versions = d.versions
		}
		if d.picked {
			r.VersionPicked = true
		}
		if confRank(d.conf) < confRank(r.Confidence) {
			r.Confidence = d.conf
		}
		r.Size += d.size
		r.SizeEstimated = r.SizeEstimated || d.estimate
		sb.WriteString(d.search.String())
	}
	r.search = sb.String()
	return r
}

func (c *Catalog) add(r *Row) {
	r.sortName = strings.ToLower(r.Name)
	c.rows = append(c.rows, r)
	c.byKey[r.Key] = r
}

// Row returns a row by its key.
func (c *Catalog) Row(key string) (*Row, bool) {
	r, ok := c.byKey[key]
	return r, ok
}

// Len is the number of games.
func (c *Catalog) Len() int { return len(c.rows) }

// Systems lists the systems that have games, in first-seen order.
func (c *Catalog) Systems() []string { return c.systems }

// Fold lowercases s, strips accents and turns punctuation into single
// spaces, so "Pokémon - Red" and "pokemon red" compare equal.
func Fold(s string) string {
	var b strings.Builder
	space := true
	for _, r := range strings.ToLower(s) {
		if rep, ok := foldMap[r]; ok {
			b.WriteString(rep)
			space = false
			continue
		}
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			space = false
			continue
		}
		if r > 127 && !strings.ContainsRune("‘’“”–—", r) {
			// Other letters (CJK, Cyrillic...) are kept as they are.
			b.WriteRune(r)
			space = false
			continue
		}
		if !space {
			b.WriteByte(' ')
			space = true
		}
	}
	return strings.TrimSpace(b.String())
}

var foldMap = func() map[rune]string {
	m := map[rune]string{}
	for letters, to := range map[string]string{
		"àáâãäåāăą": "a", "çćĉċč": "c", "ďđ": "d", "èéêëēĕėęě": "e", "ĝğġģ": "g", "ĥħ": "h",
		"ìíîïĩīĭįı": "i", "ĵ": "j", "ķ": "k", "ĺļľŀł": "l", "ñńņň": "n", "òóôõöøōŏő": "o",
		"ŕŗř": "r", "śŝşš": "s", "ţťŧ": "t", "ùúûüũūŭůűų": "u", "ŵ": "w", "ýÿŷ": "y", "źżž": "z",
		"æ": "ae", "œ": "oe", "ß": "ss",
	} {
		for _, r := range letters {
			m[r] = to
		}
	}
	return m
}()
