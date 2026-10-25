package app

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"romcure/internal/store"
)

// A system with several folders is one library: games from every folder are
// there, the same game in two folders is used from the first, and removing a
// folder forgets its games.
func TestASystemWithSeveralFolders(t *testing.T) {
	a := newApp(t)
	one := filepath.Join(t.TempDir(), "one")
	two := filepath.Join(t.TempDir(), "two")
	writeROMs(t, one) // Alpha (USA), Alpha (Europe), Beta (USA)
	os.MkdirAll(two, 0o755)
	os.WriteFile(filepath.Join(two, "Beta (USA).nes"), []byte("beta"), 0o644) // the same game again
	os.WriteFile(filepath.Join(two, "Gamma (USA).nes"), []byte("gamma"), 0o644)
	src1, err := a.AddSource(one, "nes")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.AddSource(two, "nes"); err != nil {
		t.Fatal(err)
	}
	_, out := addOutput(t, a, store.Profile{Name: "Plain"})

	var steps []Progress
	report, err := a.Sync(context.Background(), func(p Progress) {
		if p.Item == "" {
			steps = append(steps, p)
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := outputs(t, out); !equal(got, []string{"Alpha.nes", "Beta.nes", "Gamma.nes"}) {
		t.Fatalf("output = %v", got)
	}
	if len(report.Systems) != 1 || report.Systems[0].SystemID != "nes" {
		t.Errorf("one report line per system, got %+v", report.Systems)
	}
	// 2 folders scanned + 1 system identified + 1 output generated + verified = 5 steps.
	if len(steps) != 5 || steps[4].Total != 5 || steps[4].Done != 4 {
		t.Errorf("steps = %+v", steps)
	}
	var beta string
	a.DB.QueryRow(`SELECT ultimate_source_ref FROM games WHERE game_key IN (SELECT game_key FROM games WHERE ultimate_source_ref LIKE '%Beta%') LIMIT 1`).Scan(&beta)
	if filepath.Dir(beta) != filepath.Clean(one) && filepath.Dir(beta) != one {
		t.Errorf("the game in both folders should come from the first, got %s", beta)
	}

	// Removing the first folder forgets its games; the next sync brings back
	// what the other folder has.
	if err := a.RemoveSource(src1.ID); err != nil {
		t.Fatal(err)
	}
	var n int
	a.DB.QueryRow(`SELECT COUNT(*) FROM games WHERE ultimate_source_ref LIKE ?`, one+"%").Scan(&n)
	if n != 0 {
		t.Errorf("%d games of the removed folder left", n)
	}
	if _, err := a.Sync(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
	if got := outputs(t, out); !equal(got, []string{"Beta.nes", "Gamma.nes"}) {
		t.Errorf("after removing the first folder: %v", got)
	}
}
