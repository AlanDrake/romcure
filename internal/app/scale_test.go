package app

import (
	"fmt"
	"os"
	"testing"
	"time"

	"romcure/internal/catalog"
	"romcure/internal/store"
)

// TestScaleTargets builds a synthetic library of 100k games in 30 systems (no
// real ROMs, HANDOFF rule) and checks the Games screen's speed targets from
// docs/game-selection-plan.md, with headroom for slow machines.
func TestScaleTargets(t *testing.T) {
	// Wall-clock limits: only meaningful on a known machine, so opt in
	// (ROMCURE_SCALE=1 go test ./internal/app -run Scale -v).
	if os.Getenv("ROMCURE_SCALE") == "" {
		t.Skip("set ROMCURE_SCALE=1 to run the 100k-game speed check")
	}
	a := newApp(t)
	p, err := a.DB.AddProfile(store.Profile{Name: "SD", Kind: "raw", OutputRoot: t.TempDir(), StartEmpty: true})
	if err != nil {
		t.Fatal(err)
	}
	const games, systems = 100000, 30
	for i := 0; i < systems; i++ { // sources make Pending walk every system
		if _, err := a.DB.AddSource(fmt.Sprintf("/roms/sys%d", i), fmt.Sprintf("sys%d", i)); err != nil {
			t.Fatal(err)
		}
	}
	tx, err := a.DB.Begin()
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < games; i++ {
		sys := fmt.Sprintf("sys%d", i%systems)
		res, err := tx.Exec(`INSERT INTO game_groups (system_id, group_key, grouping_source, display_name) VALUES (?, ?, 'none', ?)`,
			sys, fmt.Sprintf("k%d", i), fmt.Sprintf("Game number %d of the collection", i))
		if err != nil {
			t.Fatal(err)
		}
		gid, _ := res.LastInsertId()
		for v := 0; v < 1+i%3; v++ {
			if _, err := tx.Exec(`INSERT INTO games (group_id, system_id, game_key, confidence, region, is_selected, ultimate_source_kind, ultimate_source_ref)
				VALUES (?, ?, ?, 'heuristic', 'USA', ?, 'raw_file', ?)`, gid, sys, fmt.Sprintf("g%d-%d", i, v), v == 0, fmt.Sprintf("/roms/%d-%d.zip", i, v)); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}

	timed := func(name string, limit time.Duration, fn func()) {
		t.Helper()
		start := time.Now()
		fn()
		d := time.Since(start)
		t.Logf("%-34s %v", name, d)
		if d > limit {
			t.Errorf("%s took %v, limit %v", name, d, limit)
		}
	}

	timed("catalog load (100k games)", 5*time.Second, func() {
		if _, err := a.catalog(); err != nil {
			t.Fatal(err)
		}
	})
	timed("first page", 300*time.Millisecond, func() {
		pg, err := a.Games(catalog.Filter{OutputID: p.ID}, 0, 200)
		if err != nil || pg.Total != games || len(pg.Rows) != 200 {
			t.Fatalf("page: %v total=%d rows=%d", err, pg.Total, len(pg.Rows))
		}
	})
	timed("search keystroke", 200*time.Millisecond, func() {
		pg, _ := a.Games(catalog.Filter{OutputID: p.ID, Query: "number 4242 coll"}, 0, 200)
		if pg.Total == 0 {
			t.Error("search found nothing")
		}
	})
	timed("system counts", 200*time.Millisecond, func() {
		sc, err := a.SystemCounts(p.ID)
		if err != nil || len(sc.Systems) != systems {
			t.Fatalf("counts: %v %d", err, len(sc.Systems))
		}
	})
	timed("tick 100k games by filter", 2*time.Second, func() {
		ch, err := a.SetIncluded(p.ID, Target{Filter: &catalog.Filter{Query: "number"}}, true)
		if err != nil || ch.Affected != games {
			t.Fatalf("tick: %v affected=%d", err, ch.Affected)
		}
	})
	timed("undo of the big tick", 2*time.Second, func() {
		if _, err := a.Undo(""); err != nil {
			t.Fatal(err)
		}
	})
	// A realistic curation: a couple of thousand hand-picked games.
	ch, err := a.SetIncluded(p.ID, Target{Filter: &catalog.Filter{Query: "number 42"}}, true)
	if err != nil || ch.Affected == 0 || ch.Affected > 5000 {
		t.Fatalf("curation tick: %v affected=%d", err, ch.Affected)
	}
	timed("list after ticking", 300*time.Millisecond, func() {
		a.Games(catalog.Filter{OutputID: p.ID, Show: "included"}, 0, 200)
	})
	timed("pending (cold ticks)", 1500*time.Millisecond, func() {
		x, err := a.Pending(p.ID)
		if err != nil || x.Add != ch.Affected {
			t.Fatalf("pending: %v %+v (want %d to add)", err, x, ch.Affected)
		}
	})
	timed("pending (warm)", 700*time.Millisecond, func() {
		if _, err := a.Pending(p.ID); err != nil {
			t.Fatal(err)
		}
	})
}
