package catalog

import (
	"fmt"
	"strings"
	"testing"

	"romcure/internal/store"
)

func rec(id int64, system, name string, versions int, conf string, size int64, search string) *groupRec {
	g := &groupRec{id: id, system: system, name: name, versions: versions, conf: conf, size: size, version: "USA"}
	g.search.WriteString(Fold(search))
	return g
}

func TestFoldIgnoresCaseAccentsAndPunctuation(t *testing.T) {
	for in, want := range map[string]string{
		"Pokémon - Red Version":                     "pokemon red version",
		"Legend of Zelda, The - A Link to the Past": "legend of zelda the a link to the past",
		"  WarioWare, Inc.  ":                       "warioware inc",
		"Æon Flux ß":                                "aeon flux ss",
	} {
		if got := Fold(in); got != want {
			t.Errorf("Fold(%q) = %q, want %q", in, got, want)
		}
	}
}

func testCatalog() *Catalog {
	return build([]*groupRec{
		rec(1, "snes", "Legend of Zelda, The - A Link to the Past", 3, "dat_verified", 1000, "legend of zelda usa europe japan"),
		rec(2, "snes", "Super Metroid", 1, "heuristic", 500, ""),
		rec(3, "gba", "Pokémon - Emerald Version", 2, "dat_verified", 300, "pokemon emerald usa"),
		rec(4, "psx", "Dragoon Saga (Disc 1)", 1, "dat_verified", 700, ""),
		rec(5, "psx", "Dragoon Saga (Disc 2)", 1, "heuristic", 700, ""),
		rec(6, "psx", "Dragoon Saga (Disc 3)", 2, "dat_verified", 700, ""),
		rec(7, "psx", "Spin Racer (Disc 1)", 1, "dat_verified", 650, ""), // one disc only: not a set
	})
}

func TestMultiDiscGamesFoldIntoOneRow(t *testing.T) {
	c := testCatalog()
	r, ok := c.Row("set:psx:dragoon saga")
	if !ok {
		t.Fatal("no set row")
	}
	if r.Name != "Dragoon Saga" || r.Discs != 3 || len(r.GroupIDs) != 3 {
		t.Errorf("set row: %+v", r)
	}
	if r.Size != 2100 || r.Versions != 2 || r.Confidence != "heuristic" {
		t.Errorf("set aggregates: size=%d versions=%d conf=%s", r.Size, r.Versions, r.Confidence)
	}
	if _, ok := c.Row("g:7"); !ok {
		t.Error("a lone disc stays a plain game row")
	}
	if c.Len() != 5 {
		t.Errorf("%d rows, want 5", c.Len())
	}
}

func names(p Page) string {
	var n []string
	for _, r := range p.Rows {
		n = append(n, r.Name)
	}
	return strings.Join(n, "|")
}

func TestSearchAndFilters(t *testing.T) {
	c := testCatalog()
	page := func(f Filter) Page { return c.Page(f, nil, 0, 100) }

	for _, q := range []string{"zelda past", "past zelda", "ZELDA, the", "link to"} {
		if got := names(page(Filter{Query: q})); got != "Legend of Zelda, The - A Link to the Past" {
			t.Errorf("search %q: %s", q, got)
		}
	}
	if got := names(page(Filter{Query: "pokemon"})); got != "Pokémon - Emerald Version" {
		t.Errorf("accent-insensitive: %s", got)
	}
	if got := names(page(Filter{Query: "europe"})); got != "Legend of Zelda, The - A Link to the Past" {
		t.Errorf("a version's title matches too: %s", got)
	}
	if p := page(Filter{Query: "zelda", Systems: []string{"gba"}}); p.Total != 0 || p.OtherSystems != 1 {
		t.Errorf("hits in other systems: total=%d other=%d", p.Total, p.OtherSystems)
	}
	if p := page(Filter{Systems: []string{"snes"}}); p.Total != 2 || p.OtherSystems != 0 {
		t.Errorf("system filter: %d", p.Total)
	}
	if p := page(Filter{Several: true}); p.Total != 3 { // zelda, pokemon, Saga
		t.Errorf("several versions: %d", p.Total)
	}
	if p := page(Filter{Unverified: true}); p.Total != 2 { // metroid, Saga (weakest disc)
		t.Errorf("not verified: %d", p.Total)
	}
	if got := names(page(Filter{Sort: "size", Desc: true})); !strings.HasPrefix(got, "Dragoon Saga|") {
		t.Errorf("sort by size desc: %s", got)
	}
	if got := names(page(Filter{Sort: "name", Desc: true})); !strings.HasPrefix(got, "Super Metroid|") {
		t.Errorf("sort by name desc: %s", got)
	}
	if p := c.Page(Filter{}, nil, 2, 2); len(p.Rows) != 2 || p.Total != 5 {
		t.Errorf("paging: %d rows of %d", len(p.Rows), p.Total)
	}
}

func TestTicksShowUpAsStateAndFilters(t *testing.T) {
	c := testCatalog()
	in := &store.Inclusion{Default: false, Systems: map[string]bool{"snes": true}, Groups: map[int64]bool{2: false, 4: true}}
	p := c.Page(Filter{}, in, 0, 100)
	state := map[string]GameRow{}
	for _, r := range p.Rows {
		state[r.Name] = r
	}
	if !state["Legend of Zelda, The - A Link to the Past"].Included || state["Legend of Zelda, The - A Link to the Past"].Explicit {
		t.Error("zelda should be included by its system, not explicit")
	}
	if state["Super Metroid"].Included || !state["Super Metroid"].Explicit {
		t.Error("metroid was unticked by hand")
	}
	// A set is included only when every disc is; one ticked disc is not enough.
	if state["Dragoon Saga"].Included || !state["Dragoon Saga"].Explicit {
		t.Errorf("Saga: %+v", state["Dragoon Saga"])
	}
	if p.Included != 1 {
		t.Errorf("included count %d, want 1", p.Included)
	}
	if got := names(c.Page(Filter{Show: "included"}, in, 0, 100)); got != "Legend of Zelda, The - A Link to the Past" {
		t.Errorf("show included: %s", got)
	}
	if n := c.Page(Filter{Show: "excluded"}, in, 0, 100).Total; n != 4 {
		t.Errorf("show excluded: %d", n)
	}
	if n := c.Page(Filter{Choices: true}, in, 0, 100).Total; n != 2 {
		t.Errorf("your choices: %d", n)
	}
	counts := c.Counts(in)
	for _, sc := range counts {
		if sc.SystemID == "snes" && (sc.Total != 2 || sc.Included != 1) {
			t.Errorf("snes counts %+v", sc)
		}
	}
}

func TestBuildScalesToManyGames(t *testing.T) {
	var gs []*groupRec
	for i := 0; i < 100000; i++ {
		gs = append(gs, rec(int64(i+1), fmt.Sprintf("sys%d", i%30), fmt.Sprintf("Game number %d of the collection", i), 1+i%3, "dat_verified", int64(i), "usa europe"))
	}
	c := build(gs)
	if c.Len() != 100000 {
		t.Fatalf("%d rows", c.Len())
	}
	if p := c.Page(Filter{Query: "number 99999 collection"}, nil, 0, 50); p.Total != 1 {
		t.Errorf("search hits: %d", p.Total)
	}
}

func BenchmarkSearch100k(b *testing.B) {
	var gs []*groupRec
	for i := 0; i < 100000; i++ {
		gs = append(gs, rec(int64(i+1), fmt.Sprintf("sys%d", i%30), fmt.Sprintf("Game number %d of the collection", i), 1+i%3, "dat_verified", int64(i), "usa europe"))
	}
	c := build(gs)
	in := &store.Inclusion{Default: true, Systems: map[string]bool{}, Groups: map[int64]bool{}}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c.Page(Filter{Query: "number 7 collection", Show: "included"}, in, 0, 200)
	}
}
