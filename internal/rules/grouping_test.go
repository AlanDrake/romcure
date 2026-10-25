package rules

import "testing"

func TestNormalizeTitle(t *testing.T) {
	tests := []struct {
		title string
		want  string
	}{
		{"Super Mario Bros. (USA)", "super mario bros."},
		{"Super Mario Bros.  (Europe)", "super mario bros."},
		{"SUPER MARIO BROS. (Japan) [!]", "super mario bros."},
		{"Game [b]", "game"},
		{"Plain Title", "plain title"},
		{"  Padded Title  (USA)", "padded title"},
	}
	for _, tt := range tests {
		t.Run(tt.title, func(t *testing.T) {
			if got := NormalizeTitle(tt.title); got != tt.want {
				t.Errorf("NormalizeTitle(%q) = %q, want %q", tt.title, got, tt.want)
			}
		})
	}
}

func TestGroupByDATParentClone(t *testing.T) {
	inputs := []GroupInput{
		{GameKey: "key-usa", DatGameName: "Super Game (USA)", FromParentCloneDAT: true},
		{GameKey: "key-jpn", DatGameName: "Super Game (Japan)", CloneOf: "Super Game (USA)", FromParentCloneDAT: true},
		{GameKey: "key-eur", DatGameName: "Super Game (Europe)", CloneOf: "Super Game (USA)", FromParentCloneDAT: true},
	}

	result := Group(inputs)

	parent, ok := result["key-usa"]
	if !ok {
		t.Fatal("key-usa not grouped")
	}
	if parent.GroupKey != "key-usa" || parent.Source != GroupingDATParentClone {
		t.Errorf("parent assignment = %+v, want GroupKey=key-usa Source=dat_pc", parent)
	}

	for _, cloneKey := range []string{"key-jpn", "key-eur"} {
		clone, ok := result[cloneKey]
		if !ok {
			t.Fatalf("%s not grouped", cloneKey)
		}
		if clone.GroupKey != "key-usa" {
			t.Errorf("%s GroupKey = %q, want %q (the parent's GameKey)", cloneKey, clone.GroupKey, "key-usa")
		}
		if clone.Source != GroupingDATParentClone {
			t.Errorf("%s Source = %v, want %v", cloneKey, clone.Source, GroupingDATParentClone)
		}
	}
}

func TestGroupFallsBackToTitleNormalizationWhenParentMissing(t *testing.T) {
	// The clone's parent isn't in this batch (e.g. filtered out, or
	// simply not scanned yet) — DAT P/C grouping can't place it, so it
	// must fall back to title normalization rather than being dropped.
	inputs := []GroupInput{
		{GameKey: "key-jpn", DatGameName: "Super Game (Japan)", CloneOf: "Super Game (USA)", Title: "Super Game (Japan)"},
	}

	result := Group(inputs)

	got, ok := result["key-jpn"]
	if !ok {
		t.Fatal("key-jpn not grouped at all")
	}
	if got.Source != GroupingTitleNormalization {
		t.Errorf("Source = %v, want %v (fallback)", got.Source, GroupingTitleNormalization)
	}
}

func TestGroupByTitleNormalizationForNonDATMatched(t *testing.T) {
	inputs := []GroupInput{
		{GameKey: "key-a", Title: "Homebrew Thing (USA)"},
		{GameKey: "key-b", Title: "Homebrew Thing (Europe)"},
		{GameKey: "key-c", Title: "Totally Different Game"},
	}

	result := Group(inputs)

	a, b, c := result["key-a"], result["key-b"], result["key-c"]
	if a.Source != GroupingTitleNormalization || b.Source != GroupingTitleNormalization {
		t.Errorf("expected title_normalization source for non-DAT-matched inputs, got a=%v b=%v", a.Source, b.Source)
	}
	if a.GroupKey != b.GroupKey {
		t.Errorf("a.GroupKey (%q) != b.GroupKey (%q), want them grouped together (same normalized title)", a.GroupKey, b.GroupKey)
	}
	if a.GroupKey == c.GroupKey {
		t.Error("unrelated titles got the same GroupKey")
	}
}

func TestGroupEveryInputGetsAnAssignment(t *testing.T) {
	inputs := []GroupInput{
		{GameKey: "a", DatGameName: "Game A"},
		{GameKey: "b", Title: "Game B"},
		{GameKey: "c", DatGameName: "Game C", CloneOf: "Nonexistent Parent", Title: "Game C"},
	}
	result := Group(inputs)
	for _, in := range inputs {
		if _, ok := result[in.GameKey]; !ok {
			t.Errorf("input %s got no group assignment at all", in.GameKey)
		}
	}
}

func TestNormalizeTitleStripsTrailingVersion(t *testing.T) {
	for in, want := range map[string]string{
		"Cannon Fodder v1.2 (1993)(Virgin)": "cannon fodder",
		"Cannon Fodder (1993)(Virgin)":      "cannon fodder",
		"Game Rev 3 (1990)":                 "game",
		"Version Quest (USA)":               "version quest",
	} {
		if got := NormalizeTitle(in); got != want {
			t.Errorf("NormalizeTitle(%q) = %q, want %q", in, got, want)
		}
	}
}

// A plain DAT (no parent/clone data: Redump, TOSEC, most No-Intro downloads)
// says nothing about which entries are versions of one game: its games are
// grouped by title, so 1G1R still picks one of "Super Game (USA)" and
// "Super Game (Europe)".
func TestPlainDATGamesAreGroupedByTitle(t *testing.T) {
	result := Group([]GroupInput{
		{GameKey: "usa", DatGameName: "Super Game (USA)", Title: "Super Game (USA)"},
		{GameKey: "eur", DatGameName: "Super Game (Europe)", Title: "Super Game (Europe)"},
		{GameKey: "other", DatGameName: "Other Game (USA)", Title: "Other Game (USA)"},
	})
	if result["usa"].GroupKey != result["eur"].GroupKey || result["usa"].Source != GroupingTitleNormalization {
		t.Errorf("versions of one game must share a group: %+v", result)
	}
	if result["other"].GroupKey == result["usa"].GroupKey {
		t.Error("different games must not")
	}
}
