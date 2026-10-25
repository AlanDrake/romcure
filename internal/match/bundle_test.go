package match

import (
	"context"
	"path/filepath"
	"sort"
	"testing"

	"romcure/internal/scan"
)

func TestArchiveTitle(t *testing.T) {
	for in, want := range map[string]string{
		"d/Castle_Quest.Publisher.zip":   "Castle Quest",
		"spgame.zip":                     "spgame",
		"Star Cab (1984)(Publisher).zip": "Star Cab (1984)(Publisher)",
		"a_b.zip":                        "a b",
	} {
		if got := archiveTitle(in); got != want {
			t.Errorf("archiveTitle(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestC64ProgramFilesOfOneArchiveAreOneGame(t *testing.T) {
	prgA, prgB := []byte("program a"), []byte("program b")
	tests := []struct {
		name    string
		files   map[string][]byte
		verify  [][]byte // members the DAT lists
		members []string // wanted MemberName of each item, "" = the archive
	}{
		{"three p00 files", map[string][]byte{"LOADER.P00": []byte("l"), "00.P00": []byte("0"), "01.P00": []byte("1")}, nil, []string{""}},
		{"one p00 file", map[string][]byte{"LOADER.P00": []byte("l"), "readme.txt": []byte("r")}, nil, []string{"LOADER.P00"}},
		{"two verified prg files", map[string][]byte{"a.prg": prgA, "b.prg": prgB}, [][]byte{prgA, prgB}, []string{"a.prg", "b.prg"}},
		{"disk image stays apart", map[string][]byte{"x.d64": []byte("disk"), "1.prg": []byte("1"), "2.prg": []byte("2")}, nil, []string{"", "x.d64"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			writeTestZipForBrowse(t, filepath.Join(root, "Castle_Quest.Publisher.zip"), tt.files)
			db := openTestDB(t)
			for i, v := range tt.verify {
				name := string(rune('a'+i)) + ".prg"
				seedDatEntry(t, db, "c64", "Verified "+name, name, crcHex(v), int64(len(v)))
			}
			if err := scan.New(db).Scan(context.Background(), root, nil); err != nil {
				t.Fatal(err)
			}
			items, counts, err := Browse(db, root, "c64")
			if err != nil {
				t.Fatal(err)
			}
			var got []string
			for _, it := range items {
				got = append(got, it.MemberName)
				if it.MemberName == "" && (it.Result.Title != "Castle Quest" || it.ContentHash == "") {
					t.Errorf("bundle item: %+v", it)
				}
			}
			sort.Strings(got)
			if len(got) != len(tt.members) || counts.Total() != len(tt.members) {
				t.Fatalf("members = %q, want %q", got, tt.members)
			}
			for i := range got {
				if got[i] != tt.members[i] {
					t.Fatalf("members = %q, want %q", got, tt.members)
				}
			}
		})
	}
}

// Re-zipping with another order or compression, or renaming members, must not
// change the game key: it is derived from the member CRCs only.
func TestBundleContentHashIgnoresMemberOrder(t *testing.T) {
	hash := func(files map[string][]byte) string {
		root := t.TempDir()
		writeTestZipForBrowse(t, filepath.Join(root, "g.zip"), files)
		db := openTestDB(t)
		scan.New(db).Scan(context.Background(), root, nil)
		items, _, _ := Browse(db, root, "c64")
		if len(items) != 1 {
			t.Fatalf("items = %d", len(items))
		}
		return items[0].ContentHash
	}
	a := hash(map[string][]byte{"a.p00": []byte("1"), "b.p00": []byte("2")})
	b := hash(map[string][]byte{"z.p00": []byte("2"), "y.p00": []byte("1")})
	c := hash(map[string][]byte{"a.p00": []byte("1"), "b.p00": []byte("3")})
	if a != b || a == c {
		t.Errorf("hashes %s %s %s", a, b, c)
	}
}
