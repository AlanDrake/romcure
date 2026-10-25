package profile

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"romcure/internal/store"
)

// addGame stores one selected raw-file game with the given display name.
func addGame(t *testing.T, db *store.DB, root, system, display, key, content string) {
	t.Helper()
	src := filepath.Join(root, key+".nes")
	writeFile(t, src, content)
	gid, err := db.UpsertGameGroup(store.GameGroupRecord{SystemID: system, GroupKey: "g-" + key, GroupingSource: "none", DisplayName: display})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.UpsertGame(store.GameRecord{GroupID: gid, SystemID: system, GameKey: key, Confidence: "heuristic",
		IsSelected: true, UltimateSourceKind: "raw_file", UltimateSourceRef: src}); err != nil {
		t.Fatal(err)
	}
}

func listDir(t *testing.T, dir string) []string {
	t.Helper()
	ents, _ := os.ReadDir(dir)
	var names []string
	for _, e := range ents {
		names = append(names, e.Name())
	}
	sort.Strings(names)
	return names
}

func TestGamesWithTheSameCleanedNameGetDistinctStableFiles(t *testing.T) {
	db := openTestDB(t)
	src := t.TempDir()
	// "AC/DC" and "AC:DC" both clean to "AC_DC"; "ac_dc" differs only by case.
	addGame(t, db, src, "nes", "AC/DC", "aaaaaa1111", "one")
	addGame(t, db, src, "nes", "AC:DC", "bbbbbb2222", "two")
	addGame(t, db, src, "nes", "Plain Game", "cccccc3333", "three")
	out := t.TempDir()
	opts := Options{OutputRoot: out}

	rep, err := GenerateRaw(db, "nes", opts)
	if err != nil || len(rep.Errors) != 0 {
		t.Fatalf("err=%v errors=%v", err, rep.Errors)
	}
	want := []string{"AC_DC [aaaaaa].nes", "AC_DC [bbbbbb].nes", "Plain Game.nes"}
	if got := listDir(t, filepath.Join(out, "nes")); strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("files = %v, want %v", got, want)
	}
	if len(rep.Notes) == 0 || !strings.Contains(rep.Notes[0], "2 games share a name") {
		t.Errorf("the rename must be explained: %v", rep.Notes)
	}

	// Run again: nothing changes (no flipping between the two sources).
	before := map[string]string{}
	for _, n := range want {
		b, _ := os.ReadFile(filepath.Join(out, "nes", n))
		before[n] = string(b)
	}
	if _, err := GenerateRaw(db, "nes", opts); err != nil {
		t.Fatal(err)
	}
	for _, n := range want {
		b, _ := os.ReadFile(filepath.Join(out, "nes", n))
		if string(b) != before[n] {
			t.Errorf("%s changed on the second run", n)
		}
	}
	if got := before["AC_DC [aaaaaa].nes"] + before["AC_DC [bbbbbb].nes"]; got != "onetwo" {
		t.Errorf("each file must link its own game: %q", got)
	}

	// A third namesake appears: the first two keep their names.
	addGame(t, db, src, "nes", "ac_dc", "dddddd4444", "four")
	if _, err := GenerateRaw(db, "nes", opts); err != nil {
		t.Fatal(err)
	}
	got := listDir(t, filepath.Join(out, "nes"))
	for _, n := range []string{"AC_DC [aaaaaa].nes", "AC_DC [bbbbbb].nes", "ac_dc [dddddd].nes", "Plain Game.nes"} {
		found := false
		for _, g := range got {
			found = found || g == n
		}
		if !found {
			t.Errorf("missing %s in %v", n, got)
		}
	}
}

func TestDisambiguateLeavesDiscsAndUniqueNamesAlone(t *testing.T) {
	in := []store.SelectedGame{
		{GameKey: "k1aaaaaa", DisplayName: "Game (Disc 1)"},
		{GameKey: "k2bbbbbb", DisplayName: "Game (Disc 1)"},
		{GameKey: "k3cccccc", DisplayName: "Other"},
	}
	out, n := disambiguate(in)
	if n != 0 || out[0].DisplayName != "Game (Disc 1)" || out[2].DisplayName != "Other" {
		t.Errorf("n=%d out=%+v", n, out)
	}
}
