package library

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"romcure/internal/match"
	"romcure/internal/rules"
	"romcure/internal/scan"
)

func TestReleaseKeyIgnoresDiscTagRevisionAndDumpFlagsButNotCracks(t *testing.T) {
	same := [][]string{
		{"Saga of Stars (USA) (Disc 1)", "Saga of Stars (USA) (Disc 2)", "Saga of Stars (USA) (Disc 2) (Rev 1)"},
		{"Quest of Ages (1988)(Publisher)(Disk 1 of 2 Side A)[a4]", "Quest of Ages (1988)(Publisher)(Disk 1 of 2)(Side B)", "Quest of Ages (1988)(Publisher)(Disk 2 of 2 Side A)[!]"},
		{"Quest of Ages (1988)(Publisher)(Side A)[cr Alpha]", "Quest of Ages (1988)(Publisher)(Side B)[cr Alpha][a2]"},
		// Disks named in words, and a first disk with no label at all.
		{"Hard Trek (1991)(Publisher)(EU)(Install Disk)", "Hard Trek (1991)(Publisher)(EU)(Disk Earth)", "Hard Trek (1991)(Publisher)(EU)(Disk A)", "Hard Trek (1991)(Publisher)(EU)(Savedisk)", "Hard Trek (1991)(Publisher)(EU)(Boot)", "Hard Trek (1991)(Publisher)(EU)(Master B)", "Hard Trek (1991)(Publisher)(EU)(Reel 2)", "Hard Trek (1991)(Publisher)(EU)(b)", "Hard Trek (1991)(Publisher)(EU)"},
	}
	for _, names := range same {
		first, ok := ReleaseKey(names[0])
		if !ok {
			t.Fatalf("no key for %q", names[0])
		}
		for _, n := range names[1:] {
			if k, _ := ReleaseKey(n); k != first {
				t.Errorf("%q -> %q, want %q", n, k, first)
			}
		}
	}
	for _, pair := range [][2]string{
		{"Saga of Stars (USA) (Disc 1)", "Saga of Stars (Europe) (Disc 1)"},
		{"Quest of Ages (1988)(Publisher)(Side A)[cr Alpha]", "Quest of Ages (1988)(Publisher)(Side A)[cr Beta]"},
		{"Quest of Ages (1988)(Publisher)(Side A)", "Quest of Ages (1988)(Publisher)(Side A)[t +2]"},
		// A word in a title that merely starts like "disc" is not a label.
		{"Saga (Discovery Edition)(Disk 1 of 2)", "Saga (Disk 1 of 2)"},
	} {
		a, _ := ReleaseKey(pair[0])
		b, _ := ReleaseKey(pair[1])
		if a == b {
			t.Errorf("%q and %q should be different releases", pair[0], pair[1])
		}
	}
	if _, ok := ReleaseKey(""); ok {
		t.Error("an empty name has no release")
	}
}

// cand is a disc candidate for the alignReleases tests.
func cand(key, region string, penalty int) rules.Candidate {
	return rules.Candidate{GameKey: key, Confidence: match.ConfidenceDATVerified, Region: region, VariantPenalty: penalty}
}

func discs(names map[string]string, perDisc ...[]rules.Candidate) []*discChoice {
	var out []*discChoice
	for i, cs := range perDisc {
		d := &discChoice{disc: i + 1, candidates: cs, names: names}
		d.selected, d.reason = rules.Select(cs, usaFirst)
		out = append(out, d)
	}
	return out
}

var usaFirst = rules.Preferences{RegionPriority: []string{"USA", "Europe"}}

func picks(set []*discChoice) string {
	var s []string
	for _, d := range set {
		s = append(s, d.selected)
	}
	return strings.Join(s, ",")
}

func TestAlignPrefersACompleteReleaseOverMixingRegions(t *testing.T) {
	names := map[string]string{
		"us1": "Saga (USA) (Disc 1)",
		"eu1": "Saga (Europe) (Disc 1)", "eu2": "Saga (Europe) (Disc 2)",
	}
	// USA has only disc 1: per disc, the picks would be us1 + eu2.
	set := discs(names, []rules.Candidate{cand("us1", "USA", 0), cand("eu1", "Europe", 0)}, []rules.Candidate{cand("eu2", "Europe", 0)})
	if got := picks(set); got != "us1,eu2" {
		t.Fatalf("per-disc picks = %s, test premise wrong", got)
	}
	alignReleases(set, usaFirst)
	if got := picks(set); got != "eu1,eu2" {
		t.Errorf("picks = %s, want eu1,eu2", got)
	}
	if !strings.Contains(set[0].reason, "only complete release") {
		t.Errorf("reason = %q", set[0].reason)
	}
}

func TestAlignTakesTheStatedDiscCountAsComplete(t *testing.T) {
	// USA's names say it has 2 disks; the European release has 3. Both are
	// complete, and USA comes first.
	names := map[string]string{
		"us1": "Saga (USA)(Disk 1 of 2)", "us2": "Saga (USA)(Disk 2 of 2)",
		"eu1": "Saga (Europe)(Disk 1 of 3)", "eu2": "Saga (Europe)(Disk 2 of 3)", "eu3": "Saga (Europe)(Disk 3 of 3)",
	}
	set := discs(names,
		[]rules.Candidate{cand("eu1", "Europe", 0), cand("us1", "USA", 0)},
		[]rules.Candidate{cand("eu2", "Europe", 0), cand("us2", "USA", 0)},
		[]rules.Candidate{cand("eu3", "Europe", 0)})
	alignReleases(set, usaFirst)
	// Disc 3 only exists in the European release: it keeps its pick and says so.
	if got := picks(set); got != "us1,us2,eu3" {
		t.Errorf("picks = %s, want us1,us2,eu3", got)
	}
	if !strings.Contains(set[2].reason, "has no disc 3") {
		t.Errorf("disc 3 reason = %q", set[2].reason)
	}
}

func TestAlignWithoutAStatedCountWantsEveryDiscTheGameHas(t *testing.T) {
	// Redump names don't say how many discs there are: USA's two can't be told
	// from a release missing its third disc, so the full European set wins.
	names := map[string]string{
		"us1": "Saga (USA) (Disc 1)", "us2": "Saga (USA) (Disc 2)",
		"eu1": "Saga (Europe) (Disc 1)", "eu2": "Saga (Europe) (Disc 2)", "eu3": "Saga (Europe) (Disc 3)",
	}
	set := discs(names,
		[]rules.Candidate{cand("eu1", "Europe", 0), cand("us1", "USA", 0)},
		[]rules.Candidate{cand("eu2", "Europe", 0), cand("us2", "USA", 0)},
		[]rules.Candidate{cand("eu3", "Europe", 0)})
	alignReleases(set, usaFirst)
	if got := picks(set); got != "eu1,eu2,eu3" {
		t.Errorf("picks = %s, want eu1,eu2,eu3", got)
	}
}

func TestAlignKeepsOneCrackersSidesTogether(t *testing.T) {
	names := map[string]string{
		"a-alpha": "Quest (1988)(Pub)(Side A)[cr Alpha]", "b-alpha": "Quest (1988)(Pub)(Side B)[cr Alpha]",
		"a-beta": "Quest (1988)(Pub)(Side A)[cr Beta]", "b-beta": "Quest (1988)(Pub)(Side B)[cr Beta][a2]",
	}
	// Per side, Beta wins side A (listed first) and Alpha wins side B (Beta's is an alternate).
	set := discs(names,
		[]rules.Candidate{cand("a-beta", "", 2), cand("a-alpha", "", 2)},
		[]rules.Candidate{cand("b-beta", "", 3), cand("b-alpha", "", 2)})
	if got := picks(set); got != "a-beta,b-alpha" {
		t.Fatalf("per-side picks = %s, test premise wrong", got)
	}
	alignReleases(set, usaFirst)
	if got := picks(set); got != "a-alpha,b-alpha" && got != "a-beta,b-beta" {
		t.Errorf("picks = %s, want both sides from one cracker", got)
	}
}

func TestAlignFollowsTheDiscTheUserChose(t *testing.T) {
	names := map[string]string{
		"us1": "Saga (USA) (Disc 1)", "us2": "Saga (USA) (Disc 2)",
		"eu1": "Saga (Europe) (Disc 1)", "eu2": "Saga (Europe) (Disc 2)",
	}
	set := discs(names,
		[]rules.Candidate{cand("us1", "USA", 0), cand("eu1", "Europe", 0)},
		[]rules.Candidate{cand("us2", "USA", 0), cand("eu2", "Europe", 0)})
	set[1].selected, set[1].override = "eu2", true // the user chose the European disc 2
	alignReleases(set, usaFirst)
	if got := picks(set); got != "eu1,eu2" {
		t.Errorf("picks = %s, want eu1,eu2", got)
	}
	if !strings.Contains(set[0].reason, "disc you chose (disc 2)") {
		t.Errorf("reason = %q", set[0].reason)
	}
}

// Through a real sync of files: the TOSEC sides of one cracker's release go
// together even though the other cracker's side B scores better on its own.
func TestSyncChoosesBothSidesFromOneRelease(t *testing.T) {
	root := t.TempDir()
	for name, body := range map[string]string{
		"Quest of Ages (1988)(Publisher)(Side A)[cr Alpha].d64":     "alpha-a",
		"Quest of Ages (1988)(Publisher)(Side B)[cr Alpha][a2].d64": "alpha-b",
		"Quest of Ages (1988)(Publisher)(Side A)[cr Beta][a3].d64":  "beta-a",
		"Quest of Ages (1988)(Publisher)(Side B)[cr Beta].d64":      "beta-b",
	} {
		writeFile(t, filepath.Join(root, name), body)
	}
	db := openTestDB(t)
	if err := scan.New(db).Scan(context.Background(), root, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := Sync(db, root, "c64", rules.Preferences{}); err != nil {
		t.Fatal(err)
	}
	sel, err := db.SelectedGames("c64")
	if err != nil {
		t.Fatal(err)
	}
	if len(sel) != 2 {
		t.Fatalf("selected %d games, want 2", len(sel))
	}
	cracker := map[string]bool{}
	for _, g := range sel {
		for _, c := range []string{"Alpha", "Beta"} {
			if strings.Contains(filepath.Base(g.UltimateSourceRef), "[cr "+c+"]") {
				cracker[c] = true
			}
		}
	}
	if len(cracker) != 1 {
		t.Errorf("sides come from %v, want one cracker", cracker)
	}
}
