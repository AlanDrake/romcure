package app

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"romcure/internal/catalog"
	"romcure/internal/store"
)

func rowKeyOf(t *testing.T, a *App, name string) string {
	t.Helper()
	p, err := a.Games(catalog.Filter{Query: name}, 0, 10)
	if err != nil || len(p.Rows) == 0 {
		t.Fatalf("game %q: %v (%d rows)", name, err, len(p.Rows))
	}
	return p.Rows[0].Key
}

func included(t *testing.T, a *App, outputID int64) (map[string]bool, catalog.Page) {
	t.Helper()
	p, err := a.Games(catalog.Filter{OutputID: outputID}, 0, 100)
	if err != nil {
		t.Fatal(err)
	}
	m := map[string]bool{}
	for _, r := range p.Rows {
		m[r.Name] = r.Included
	}
	return m, p
}

func TestTickingByKeyFilterSystemAndAll(t *testing.T) {
	a, _ := selectionApp(t)
	p, _ := addOutput(t, a, store.Profile{Name: "SD", StartEmpty: true})
	doSync(t, a)

	if m, page := included(t, a, p.ID); page.Total != 3 || page.Included != 0 || m["Alpha"] {
		t.Fatalf("an empty output starts with nothing: %+v", m)
	}
	ch, err := a.SetIncluded(p.ID, Target{Keys: []string{rowKeyOf(t, a, "alpha")}}, true)
	if err != nil || ch.Affected != 1 {
		t.Fatalf("tick one: %+v %v", ch, err)
	}
	if m, _ := included(t, a, p.ID); !m["Alpha"] || m["Beta"] {
		t.Errorf("after ticking Alpha: %v", m)
	}
	// Ticking what is already ticked changes nothing.
	if ch, _ := a.SetIncluded(p.ID, Target{Keys: []string{rowKeyOf(t, a, "alpha")}}, true); ch.Affected != 0 {
		t.Errorf("affected %d for a no-op", ch.Affected)
	}

	ch, _ = a.SetIncluded(p.ID, Target{Filter: &catalog.Filter{Query: "a"}}, true) // Alpha, Beta, Gamma all contain "a"
	if ch.Affected != 2 {
		t.Errorf("filter tick affected %d, want 2 (Alpha was already on)", ch.Affected)
	}
	ch, _ = a.SetIncluded(p.ID, Target{SystemID: "nes"}, false)
	if ch.Affected != 3 {
		t.Errorf("system untick affected %d", ch.Affected)
	}
	if m, page := included(t, a, p.ID); page.Included != 0 || m["Alpha"] {
		t.Errorf("system untick: %v", m)
	}
	ch, _ = a.SetIncluded(p.ID, Target{All: true}, true)
	if _, page := included(t, a, p.ID); page.Included != 3 || ch.Affected != 3 {
		t.Errorf("all: included %d affected %d", page.Included, ch.Affected)
	}
	counts, _ := a.SystemCounts(p.ID)
	if !counts.Default || counts.Systems[0].Included != 3 {
		t.Errorf("counts: %+v", counts)
	}
}

func TestUndoRestoresTicksInOrder(t *testing.T) {
	a, _ := selectionApp(t)
	p, _ := addOutput(t, a, store.Profile{Name: "SD", StartEmpty: true})
	doSync(t, a)

	first, _ := a.SetIncluded(p.ID, Target{Keys: []string{rowKeyOf(t, a, "alpha")}}, true)
	second, _ := a.SetIncluded(p.ID, Target{SystemID: "nes"}, true)
	if _, err := a.Undo(first.Token); err == nil {
		t.Error("undoing an older change before a newer one must be refused")
	}
	if label, err := a.Undo(second.Token); err != nil || label != second.Message {
		t.Fatalf("undo: %q %v", label, err)
	}
	if m, _ := included(t, a, p.ID); !m["Alpha"] || m["Beta"] {
		t.Errorf("after undoing the system tick: %v", m)
	}
	if _, err := a.Undo(""); err != nil {
		t.Fatal(err)
	}
	if _, page := included(t, a, p.ID); page.Included != 0 {
		t.Errorf("after undoing everything: %d included", page.Included)
	}
	if _, err := a.Undo(""); err == nil {
		t.Error("nothing left to undo should be an error")
	}
}

func TestPendingCountsAndApplyDoNotRescan(t *testing.T) {
	a, src := selectionApp(t)
	p, root := addOutput(t, a, store.Profile{Name: "SD", StartEmpty: true})
	doSync(t, a)
	pend := func() Pending {
		t.Helper()
		x, err := a.Pending(p.ID)
		if err != nil {
			t.Fatal(err)
		}
		return x
	}
	if x := pend(); x.Any || x.Count != 0 {
		t.Fatalf("empty output: %+v", x)
	}

	a.SetIncluded(p.ID, Target{Keys: []string{rowKeyOf(t, a, "alpha"), rowKeyOf(t, a, "beta")}}, true)
	if x := pend(); x.Add != 2 || x.Remove != 0 || x.Replace != 0 || !x.Any || x.Count != 2 || x.Bytes == 0 {
		t.Fatalf("two to add: %+v", x)
	}

	// Apply uses the saved choices and does not look at the source folder:
	// a file added since the last sync must not appear.
	os.WriteFile(filepath.Join(src, "Zeta (USA).nes"), []byte("zeta"), 0o644)
	if _, err := a.Apply(context.Background(), []int64{p.ID}, nil); err != nil {
		t.Fatal(err)
	}
	if got := outputs(t, root); !equal(got, []string{"Alpha.nes", "Beta.nes"}) {
		t.Errorf("after apply: %v", got)
	}
	if x := pend(); x.Any {
		t.Errorf("up to date after apply: %+v", x)
	}

	a.SetIncluded(p.ID, Target{Keys: []string{rowKeyOf(t, a, "beta")}}, false)
	if x := pend(); x.Remove != 1 || x.Add != 0 {
		t.Errorf("one to remove: %+v", x)
	}
	a.Apply(context.Background(), []int64{p.ID}, nil)
	if got := outputs(t, root); !equal(got, []string{"Alpha.nes"}) {
		t.Errorf("after removing Beta: %v", got)
	}
}

func TestVersionChoiceReplacesTheOutputAndUndoes(t *testing.T) {
	a, _ := selectionApp(t)
	p, root := addOutput(t, a, store.Profile{Name: "Plain"})
	doSync(t, a)
	if got := readFileContent(t, filepath.Join(root, "nes", "Alpha.nes")); got != "alpha" {
		t.Fatalf("automatic pick (USA first): %q", got)
	}

	key := rowKeyOf(t, a, "alpha")
	d, err := a.Game(key)
	if err != nil || len(d.Groups) != 1 || len(d.Groups[0].Variants) != 2 {
		t.Fatalf("detail: %+v %v", d, err)
	}
	var europe string
	for _, v := range d.Groups[0].Variants {
		if v.Region == "Europe" {
			europe = v.GameKey
		}
	}
	ch, err := a.SetVersion(key, europe)
	if err != nil {
		t.Fatal(err)
	}
	if x, _ := a.Pending(p.ID); x.Replace != 1 || x.Add != 0 || x.Remove != 0 {
		t.Errorf("a version change is one replacement: %+v", x)
	}
	// The saved choice does not touch the folder until Apply.
	if got := readFileContent(t, filepath.Join(root, "nes", "Alpha.nes")); got != "alpha" {
		t.Errorf("folder changed before Apply: %q", got)
	}
	a.Apply(context.Background(), nil, nil)
	if got := readFileContent(t, filepath.Join(root, "nes", "Alpha.nes")); got != "alpha-eu" {
		t.Errorf("after Apply: %q", got)
	}
	if page, _ := a.Games(catalog.Filter{Query: "alpha"}, 0, 5); !page.Rows[0].VersionPicked || page.Rows[0].Version != "Europe" {
		t.Errorf("row after the pick: %+v", page.Rows[0])
	}

	if _, err := a.Undo(ch.Token); err != nil {
		t.Fatal(err)
	}
	if page, _ := a.Games(catalog.Filter{Query: "alpha"}, 0, 5); page.Rows[0].VersionPicked {
		t.Error("undo should clear the version choice")
	}
	if _, err := a.AutoVersion(key); err != nil { // already automatic: harmless
		t.Fatal(err)
	}
	a.Apply(context.Background(), nil, nil)
	if got := readFileContent(t, filepath.Join(root, "nes", "Alpha.nes")); got != "alpha" {
		t.Errorf("back to automatic: %q", got)
	}
}

func TestGameDetailSaysWhyEachOutputHasIt(t *testing.T) {
	a, _ := selectionApp(t)
	all, _ := addOutput(t, a, store.Profile{Name: "Everything"})
	sd, _ := addOutput(t, a, store.Profile{Name: "SD", StartEmpty: true})
	doSync(t, a)
	a.SetIncluded(sd.ID, Target{SystemID: "nes"}, true)
	a.SetIncluded(all.ID, Target{Keys: []string{rowKeyOf(t, a, "beta")}}, false)

	d, err := a.Game(rowKeyOf(t, a, "beta"))
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]OutputState{}
	for _, o := range d.Outputs {
		got[o.Name] = o
	}
	if o := got["Everything"]; o.Included || o.Source != "game" {
		t.Errorf("Everything: %+v", o)
	}
	if o := got["SD"]; !o.Included || o.Source != "system" {
		t.Errorf("SD: %+v", o)
	}
}

func readFileContent(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestOutputSummariesShowSizeAndWaitingChanges(t *testing.T) {
	a, _ := selectionApp(t)
	all, _ := addOutput(t, a, store.Profile{Name: "Everything"})
	sd, _ := addOutput(t, a, store.Profile{Name: "SD", StartEmpty: true})
	doSync(t, a)
	a.SetIncluded(sd.ID, Target{Keys: []string{rowKeyOf(t, a, "alpha")}}, true)

	sums, err := a.OutputSummaries()
	if err != nil || len(sums) != 2 {
		t.Fatalf("summaries: %v %v", sums, err)
	}
	by := map[int64]OutputSummary{}
	for _, s := range sums {
		by[s.ID] = s
	}
	if s := by[all.ID]; s.Included != 3 || s.Total != 3 || s.Pending.Any {
		t.Errorf("everything: %+v", s)
	}
	if s := by[sd.ID]; s.Included != 1 || s.Total != 3 || s.Pending.Add != 1 {
		t.Errorf("sd: %+v", s)
	}
}

func TestEditingAnOutputMovesItAndCleansTheOldFolder(t *testing.T) {
	a, _ := selectionApp(t)
	p, oldRoot := addOutput(t, a, store.Profile{Name: "Plain"})
	doSync(t, a)
	if got := outputs(t, oldRoot); len(got) != 3 {
		t.Fatalf("before: %v", got)
	}
	newRoot := filepath.Join(t.TempDir(), "moved")

	if err := a.UpdateProfile(p.ID, "Renamed", newRoot, true); err != nil {
		t.Fatal(err)
	}
	if got := outputs(t, oldRoot); len(got) != 0 {
		t.Errorf("old folder should be cleaned: %v", got)
	}
	if x, _ := a.Pending(p.ID); x.Add != 3 {
		t.Errorf("the new folder starts empty, so 3 games are waiting: %+v", x)
	}
	doSync(t, a)
	if got := outputs(t, newRoot); len(got) != 3 {
		t.Errorf("new folder after Apply: %v", got)
	}
	profs, _ := a.DB.Profiles()
	if profs[0].Name != "Renamed" || profs[0].OutputRoot != newRoot {
		t.Errorf("profile = %+v", profs[0])
	}
	// Renaming alone leaves the files alone.
	if err := a.UpdateProfile(p.ID, "Again", newRoot, true); err != nil {
		t.Fatal(err)
	}
	if got := outputs(t, newRoot); len(got) != 3 {
		t.Errorf("a rename must not touch the files: %v", got)
	}
	if err := a.UpdateProfile(p.ID, "  ", newRoot, false); err == nil {
		t.Error("an empty name must be refused")
	}
}

func TestEditingAnOutputKeepsTheOldFilesWhenAsked(t *testing.T) {
	a, _ := selectionApp(t)
	p, oldRoot := addOutput(t, a, store.Profile{Name: "Plain"})
	doSync(t, a)
	if err := a.UpdateProfile(p.ID, "Plain", filepath.Join(t.TempDir(), "elsewhere"), false); err != nil {
		t.Fatal(err)
	}
	if got := outputs(t, oldRoot); len(got) != 3 {
		t.Errorf("the old files are the user's to keep: %v", got)
	}
}

func TestOutputProblemsAndRepair(t *testing.T) {
	a, _ := selectionApp(t)
	p, root := addOutput(t, a, store.Profile{Name: "Plain"})
	doSync(t, a)
	if probs, err := a.OutputProblems(p.ID); err != nil || len(probs) != 0 {
		t.Fatalf("fresh output: %v %v", probs, err)
	}
	if err := os.Remove(filepath.Join(root, "nes", "Beta.nes")); err != nil {
		t.Fatal(err)
	}
	probs, err := a.OutputProblems(p.ID)
	if err != nil || len(probs) != 1 || probs[0].Name != "Beta.nes" || probs[0].Status != "broken" {
		t.Fatalf("problems = %+v %v", probs, err)
	}
	if _, err := a.RepairOutput(p.ID); err != nil {
		t.Fatal(err)
	}
	doSync(t, a)
	if got := outputs(t, root); !equal(got, []string{"Alpha.nes", "Beta.nes", "Gamma.nes"}) {
		t.Errorf("after repair and apply: %v", got)
	}
	if probs, _ := a.OutputProblems(p.ID); len(probs) != 0 {
		t.Errorf("still problems: %+v", probs)
	}
}
