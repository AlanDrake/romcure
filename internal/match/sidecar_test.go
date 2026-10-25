package match

import (
	"context"
	"path/filepath"
	"sort"
	"testing"

	"romcure/internal/scan"
)

func TestIsSidecarKeepsGamesAndDropsEverythingElse(t *testing.T) {
	for _, c := range []struct {
		system, name string
		want         bool
	}{
		{"psx", "chdman.exe", true},
		{"psx", "CUE-GDI-ISO to CHD.bat", true},
		{"psx", "playlist.sh", true},
		{"psx", "CDRomance.url", true},
		{"psx", "Spin Racer (USA).sbi", true},
		{"psx", "readme.TXT", true},
		{"psx", "._Spin Racer.chd", true}, // macOS resource fork
		{"psx", ".DS_Store", true},
		{"psx", "Thumbs.db", true},
		{"psx", "dir/sub/notes.md", true},
		{"psx", "Spin Racer (USA).chd", false},
		{"psx", "Spin Racer (USA).cue", false},
		{"psx", "Spin Racer (USA).bin", false},
		{"psx", "game.7z", false},
		{"nes", "Mario (USA).nes", false},
		{"nes", "Mario (USA).zip", false},
		{"dos", "GAME.EXE", false}, // on DOS the programs are the games
		{"dos", "GAME.COM", false},
		{"dos", "notes.txt", true},
		{"msx", "game.com", false},
	} {
		if got := isSidecar(c.system, c.name); got != c.want {
			t.Errorf("isSidecar(%s, %q) = %v, want %v", c.system, c.name, got, c.want)
		}
	}
}

func TestStrayArchiveMembersAreNotGames(t *testing.T) {
	root := t.TempDir()
	writeTestZipForBrowse(t, filepath.Join(root, "exq.zip"), map[string][]byte{
		"grp-exq.gba": []byte("rom"), "info.f01": []byte("n"), "__tmp__": []byte("f"),
		"GRPTAG": []byte("q"), "fix.ips": []byte("p"),
	})
	// No ROM among the members: nothing is filtered, an odd extension still shows up.
	writeTestZipForBrowse(t, filepath.Join(root, "odd.zip"), map[string][]byte{"game.xyz": []byte("g")})
	db := openTestDB(t)
	if err := scan.New(db).Scan(context.Background(), root, nil); err != nil {
		t.Fatal(err)
	}
	items, _, err := Browse(db, root, "gba")
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, it := range items {
		got = append(got, it.MemberName)
	}
	sort.Strings(got)
	if len(got) != 2 || got[0] != "game.xyz" || got[1] != "grp-exq.gba" {
		t.Errorf("members = %q", got)
	}
}

func TestHasROMExt(t *testing.T) {
	for _, c := range []struct {
		system, name string
		want         bool
	}{
		{"c64", "00.P00", true}, {"c64", "LOADER.p00", true}, {"c64", "a.d64", true}, {"c64", "a.txt", false},
		{"gba", "x.GBA", true}, {"gba", "x.p00", false}, {"psx", "x.bin", false},
	} {
		if got := hasROMExt(c.system, c.name); got != c.want {
			t.Errorf("hasROMExt(%s, %q) = %v", c.system, c.name, got)
		}
	}
}
