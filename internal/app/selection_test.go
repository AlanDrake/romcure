package app

import (
	"strings"
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"romcure/internal/store"
)

// selectionApp returns an app with a NES source (Alpha, Beta, Gamma) and the
// path of the source folder.
func selectionApp(t *testing.T) (*App, string) {
	t.Helper()
	a := newApp(t)
	src := filepath.Join(t.TempDir(), "nes")
	writeROMs(t, src)
	os.WriteFile(filepath.Join(src, "Gamma (USA).nes"), []byte("gamma"), 0o644)
	if _, err := a.AddSource(src, "nes"); err != nil {
		t.Fatal(err)
	}
	return a, src
}

func addOutput(t *testing.T, a *App, p store.Profile) (store.Profile, string) {
	t.Helper()
	p.OutputRoot = filepath.Join(t.TempDir(), "out")
	p.Kind = "raw"
	got, err := a.AddProfile(p)
	if err != nil {
		t.Fatal(err)
	}
	return got, p.OutputRoot
}

func doSync(t *testing.T, a *App) {
	t.Helper()
	if _, err := a.Sync(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
}

func outputs(t *testing.T, root string) []string {
	t.Helper()
	entries, _ := os.ReadDir(filepath.Join(root, "nes"))
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	return names
}

func groupID(t *testing.T, a *App, name string) int64 {
	t.Helper()
	groups, err := a.Groups("nes")
	if err != nil {
		t.Fatal(err)
	}
	for _, g := range groups {
		if g.Name == name {
			return g.ID
		}
	}
	t.Fatalf("no game %q", name)
	return 0
}

func equal(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestUntickedGamesAreRemovedAndTickedBack(t *testing.T) {
	a, _ := selectionApp(t)
	p, root := addOutput(t, a, store.Profile{Name: "Plain"})
	doSync(t, a)
	if got := outputs(t, root); !equal(got, []string{"Alpha.nes", "Beta.nes", "Gamma.nes"}) {
		t.Fatalf("default output: %v", got)
	}

	if err := a.DB.SetGameIncluded(p.ID, groupID(t, a, "Beta"), false); err != nil {
		t.Fatal(err)
	}
	doSync(t, a)
	if got := outputs(t, root); !equal(got, []string{"Alpha.nes", "Gamma.nes"}) {
		t.Errorf("after unticking Beta: %v", got)
	}
	rows, _ := a.DB.ListLinkStates(store.OutputKey(p.ID))
	if len(rows) != 2 {
		t.Errorf("link_state rows after unticking: %d, want 2", len(rows))
	}

	if err := a.DB.SetGameIncluded(p.ID, groupID(t, a, "Beta"), true); err != nil {
		t.Fatal(err)
	}
	doSync(t, a)
	if got := outputs(t, root); !equal(got, []string{"Alpha.nes", "Beta.nes", "Gamma.nes"}) {
		t.Errorf("after ticking Beta again: %v", got)
	}
	// Back to the inherited state: no exception row is kept.
	var n int
	a.DB.QueryRow(`SELECT COUNT(*) FROM output_games`).Scan(&n)
	if n != 0 {
		t.Errorf("%d tick rows kept for a game that matches its default", n)
	}
}

func TestDroppingASystemRemovesItsOutputs(t *testing.T) {
	a, _ := selectionApp(t)
	p, root := addOutput(t, a, store.Profile{Name: "Plain"})
	doSync(t, a)
	if err := a.DB.SetSystemIncluded(p.ID, "nes", false); err != nil {
		t.Fatal(err)
	}
	doSync(t, a)
	if got := outputs(t, root); len(got) != 0 {
		t.Errorf("system unticked, still there: %v", got)
	}
	if err := a.DB.SetSystemIncluded(p.ID, "nes", true); err != nil {
		t.Fatal(err)
	}
	doSync(t, a)
	if got := outputs(t, root); len(got) != 3 {
		t.Errorf("system ticked again: %v", got)
	}
}

func TestNewGamesFollowTheirSystemOrTheDefault(t *testing.T) {
	a, src := selectionApp(t)
	all, allRoot := addOutput(t, a, store.Profile{Name: "Everything"})
	bySystem, sysRoot := addOutput(t, a, store.Profile{Name: "By system", StartEmpty: true})
	byHand, handRoot := addOutput(t, a, store.Profile{Name: "By hand", StartEmpty: true})
	_ = all
	doSync(t, a)
	if got := outputs(t, sysRoot); len(got) != 0 {
		t.Fatalf("an output that starts empty has games: %v", got)
	}

	if err := a.DB.SetSystemIncluded(bySystem.ID, "nes", true); err != nil {
		t.Fatal(err)
	}
	if err := a.DB.SetGameIncluded(byHand.ID, groupID(t, a, "Alpha"), true); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(src, "Delta (USA).nes"), []byte("delta"), 0o644)
	doSync(t, a)

	if got := outputs(t, allRoot); len(got) != 4 {
		t.Errorf("default-include output: %v", got)
	}
	if got := outputs(t, sysRoot); len(got) != 4 {
		t.Errorf("ticked system should take the new game: %v", got)
	}
	if got := outputs(t, handRoot); !equal(got, []string{"Alpha.nes"}) {
		t.Errorf("hand-picked output must not take the new game: %v", got)
	}
}

func TestApplyingTwiceWritesNothing(t *testing.T) {
	a, _ := selectionApp(t)
	p, root := addOutput(t, a, store.Profile{Name: "Plain"})
	doSync(t, a)
	a.DB.SetGameIncluded(p.ID, groupID(t, a, "Gamma"), false)
	doSync(t, a)

	before := map[string]time.Time{}
	filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err == nil {
			before[path] = info.ModTime()
		}
		return nil
	})
	time.Sleep(20 * time.Millisecond)
	doSync(t, a)
	filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if b, ok := before[path]; !ok || !b.Equal(info.ModTime()) {
			t.Errorf("%s was written by the second apply", path)
		}
		delete(before, path)
		return nil
	})
	for path := range before {
		t.Errorf("%s disappeared on the second apply", path)
	}
}

func TestRemovingAnOutputForgetsItsLinksAndTicks(t *testing.T) {
	a, _ := selectionApp(t)
	p, root := addOutput(t, a, store.Profile{Name: "Plain"})
	doSync(t, a)
	a.DB.SetGameIncluded(p.ID, groupID(t, a, "Beta"), false)

	if err := a.RemoveProfile(p.ID, true); err != nil {
		t.Fatal(err)
	}
	if got := outputs(t, root); len(got) != 0 {
		t.Errorf("links left behind: %v", got)
	}
	if rows, _ := a.DB.ListLinkStates(store.OutputKey(p.ID)); len(rows) != 0 {
		t.Errorf("link_state rows left: %d", len(rows))
	}
	var n int
	a.DB.QueryRow(`SELECT COUNT(*) FROM output_games`).Scan(&n)
	if n != 0 {
		t.Errorf("%d tick rows left after removing the output", n)
	}
}

func TestRemovingAnOutputWithoutDeletingLeavesTheFiles(t *testing.T) {
	a, _ := selectionApp(t)
	p, root := addOutput(t, a, store.Profile{Name: "Plain"})
	doSync(t, a)
	if err := a.RemoveProfile(p.ID, false); err != nil {
		t.Fatal(err)
	}
	if got := outputs(t, root); len(got) != 3 {
		t.Errorf("files should stay: %v", got)
	}
}

func TestVersionChoiceFollowsTheGameWhenItIsRegrouped(t *testing.T) {
	a, _ := selectionApp(t)
	addOutput(t, a, store.Profile{Name: "Plain"})
	doSync(t, a)
	groups, _ := a.Groups("nes")
	var alpha, europe string
	for _, g := range groups {
		if g.Name == "Alpha" {
			alpha = g.Name
			for _, v := range g.Variants {
				if v.Region == "Europe" {
					europe = v.GameKey
				}
			}
		}
	}
	if alpha == "" || europe == "" {
		t.Fatal("test data not found")
	}
	if err := a.PickVariant("nes", europe); err != nil {
		t.Fatal(err)
	}
	// A DAT import that changes how games are grouped gives them new group
	// keys; simulate it by renaming the group's key.
	if _, err := a.DB.Exec(`UPDATE game_groups SET group_key = 'old-' || group_key WHERE display_name = 'Alpha'`); err != nil {
		t.Fatal(err)
	}
	doSync(t, a)

	groups, _ = a.Groups("nes")
	for _, g := range groups {
		if g.Name == "Alpha" && (!g.Overridden || g.OverrideKey != europe) {
			t.Errorf("override lost after regrouping: overridden=%v key=%q", g.Overridden, g.OverrideKey)
		}
	}
	var n int
	a.DB.QueryRow(`SELECT COUNT(*) FROM overrides`).Scan(&n)
	if n != 1 {
		t.Errorf("%d overrides, want 1", n)
	}
}

func TestSettingsReachTheGenerators(t *testing.T) {
	a := newApp(t)
	a.Tools = Tools{Chdman: "chdman"}
	a.Settings.Parallelism = 3
	a.Settings.CacheDir = filepath.Join(t.TempDir(), "bigcache")
	a.Settings.CHDCodecsCD = "cdzs"
	a.Settings.CHDCodecsDVD = "zstd"
	p, err := a.AddProfile(store.Profile{Name: "x", Kind: "raw", OutputRoot: filepath.Join(t.TempDir(), "o")})
	if err != nil {
		t.Fatal(err)
	}
	opts, err := a.outputOptions(p, "psx")
	if err != nil {
		t.Fatal(err)
	}
	if opts.Transcode.Cache.Dir != filepath.Join(a.Settings.CacheDir, "transcode-cache") || opts.ExtractCacheDir != filepath.Join(a.Settings.CacheDir, "extract-cache") {
		t.Errorf("caches not under the chosen folder: %q, %q", opts.Transcode.Cache.Dir, opts.ExtractCacheDir)
	}
	if opts.Parallelism != 3 {
		t.Errorf("parallelism = %d, want 3", opts.Parallelism)
	}
	cd := opts.Transcode.Profile(0)
	dvd := opts.Transcode.Profile(1)
	if cd.Flags[1] != "cdzs" || dvd.Flags[1] != "zstd" {
		t.Errorf("codecs: CD %v, DVD %v", cd.Flags, dvd.Flags)
	}
}

func TestSyncReportsStepsOfTheWholeJob(t *testing.T) {
	a, _ := selectionApp(t)
	addOutput(t, a, store.Profile{Name: "Plain"})
	var events []Progress
	if _, err := a.Sync(context.Background(), func(p Progress) { events = append(events, p) }); err != nil {
		t.Fatal(err)
	}
	// 1 source, 1 output: scan, identify, generate, verify = 4 steps.
	// Events that name the file or game being handled come in between the steps.
	var stages, items []string
	for _, e := range events {
		if e.Total != 4 {
			t.Errorf("event %s: total %d, want 4", e.Stage, e.Total)
		}
		if e.Item != "" {
			items = append(items, e.Stage+":"+e.Item)
			continue
		}
		if e.Done != len(stages) {
			t.Errorf("event %d (%s): done %d, want %d", len(stages), e.Stage, e.Done, len(stages))
		}
		stages = append(stages, e.Stage)
	}
	if got := strings.Join(stages, ","); got != "scan,identify,generate,verify" {
		t.Errorf("stages = %s", got)
	}
	var sawScan, sawGame bool
	for _, it := range items {
		sawScan = sawScan || strings.HasPrefix(it, "scan:")
		sawGame = sawGame || strings.HasPrefix(it, "generate:")
	}
	if !sawScan || !sawGame {
		t.Errorf("per-item events = %v, want a scanned file and a generated game", items)
	}
}

func TestSyncGoesOnPastAnUnreadableArchiveAndReportsIt(t *testing.T) {
	a, src := selectionApp(t)
	_, out := addOutput(t, a, store.Profile{Name: "Plain"})
	os.WriteFile(filepath.Join(src, "Broken (USA).7z"), []byte("not really a 7z"), 0o644)
	os.WriteFile(filepath.Join(src, "Omega (USA).nes"), []byte("omega"), 0o644)

	report, err := a.Sync(context.Background(), nil)
	if err != nil {
		t.Fatalf("sync must not fail because of one damaged archive: %v", err)
	}
	if len(report.Unreadable) != 1 || filepath.Base(report.Unreadable[0].Path) != "Broken (USA).7z" || report.Unreadable[0].SystemID != "nes" {
		t.Errorf("unreadable = %+v", report.Unreadable)
	}
	if got := outputs(t, out); !equal(got, []string{"Alpha.nes", "Beta.nes", "Gamma.nes", "Omega.nes"}) {
		t.Errorf("everything else must still be synced: %v", got)
	}
}
