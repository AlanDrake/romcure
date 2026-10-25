package profile

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// RetroArch's own playlist, with a default core the user picked and an entry of theirs.
const userPlaylist = `{
  "version": "1.5",
  "default_core_path": "/cores/mgba_libretro.dll",
  "default_core_name": "Nintendo - Game Boy Advance (mGBA)",
  "label_display_mode": 0,
  "right_thumbnail_mode": 0,
  "left_thumbnail_mode": 0,
  "sort_mode": 2,
  "scan_content_dir": "/their/games",
  "items": [
    {"path": "/their/games/Own Game.gba", "label": "Own Game", "core_path": "/cores/mgba_libretro.dll", "core_name": "mGBA", "crc32": "ABCD1234|crc", "db_name": "Nintendo - Game Boy Advance.lpl"}
  ]
}`

func oursIn(root string, names ...string) []lplItem {
	var out []lplItem
	for _, n := range names {
		out = append(out, lplItem{Path: filepath.Join(root, "gba", n+".gba"), Label: n, CorePath: "DETECT", CoreName: "DETECT", CRC32: "DETECT", DBName: "Nintendo - Game Boy Advance.lpl"})
	}
	return out
}

func readItems(t *testing.T, path string) (map[string]json.RawMessage, []map[string]string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var head map[string]json.RawMessage
	if err := json.Unmarshal(data, &head); err != nil {
		t.Fatal(err)
	}
	var items []map[string]string
	if err := json.Unmarshal(head["items"], &items); err != nil {
		t.Fatal(err)
	}
	return head, items
}

func TestMergeKeepsTheUsersEntriesAndHeaderAndReplacesOurs(t *testing.T) {
	dir, root := t.TempDir(), filepath.Join(t.TempDir(), "RomCure")
	pl := filepath.Join(dir, "Nintendo - Game Boy Advance.lpl")
	if err := os.WriteFile(pl, []byte(userPlaylist), 0o644); err != nil {
		t.Fatal(err)
	}

	if changed, err := mergePlaylist(pl, root, oursIn(root, "Alpha", "Beta"), nil); err != nil || !changed {
		t.Fatalf("first merge: changed=%v err=%v", changed, err)
	}
	// A later run drops Beta and adds Gamma: Beta must go, the user's entry must stay.
	if changed, err := mergePlaylist(pl, root, oursIn(root, "Alpha", "Gamma"), nil); err != nil || !changed {
		t.Fatalf("second merge: changed=%v err=%v", changed, err)
	}

	head, items := readItems(t, pl)
	var labels []string
	for _, it := range items {
		labels = append(labels, it["label"])
	}
	if got := strings.Join(labels, ","); got != "Own Game,Alpha,Gamma" {
		t.Errorf("entries = %s, want Own Game,Alpha,Gamma", got)
	}
	if items[0]["crc32"] != "ABCD1234|crc" || items[0]["core_name"] != "mGBA" {
		t.Errorf("the user's entry changed: %v", items[0])
	}
	if string(head["default_core_name"]) != `"Nintendo - Game Boy Advance (mGBA)"` || string(head["sort_mode"]) != "2" || string(head["scan_content_dir"]) != `"/their/games"` {
		t.Errorf("header fields were not kept: %s", head)
	}
}

func TestMergeWritesNothingWhenNothingChanged(t *testing.T) {
	dir, root := t.TempDir(), filepath.Join(t.TempDir(), "RomCure")
	pl := filepath.Join(dir, "x.lpl")
	if err := os.WriteFile(pl, []byte(userPlaylist), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := mergePlaylist(pl, root, oursIn(root, "Alpha"), nil); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-time.Hour)
	if err := os.Chtimes(pl, old, old); err != nil {
		t.Fatal(err)
	}
	if changed, err := mergePlaylist(pl, root, oursIn(root, "Alpha"), nil); err != nil || changed {
		t.Fatalf("second run: changed=%v err=%v", changed, err)
	}
	if st, _ := os.Stat(pl); !st.ModTime().Equal(old.Truncate(time.Second)) && st.ModTime().After(old.Add(time.Minute)) {
		t.Error("the playlist was rewritten although nothing changed")
	}
}

func TestMergeCreatesAPlaylistOnlyWhenThereAreGames(t *testing.T) {
	dir, root := t.TempDir(), filepath.Join(t.TempDir(), "RomCure")
	pl := filepath.Join(dir, "playlists", "new.lpl")
	if changed, err := mergePlaylist(pl, root, nil, nil); err != nil || changed {
		t.Fatalf("no games: changed=%v err=%v", changed, err)
	}
	if _, err := os.Stat(pl); err == nil {
		t.Fatal("an empty playlist was created")
	}
	if changed, err := mergePlaylist(pl, root, oursIn(root, "Alpha"), nil); err != nil || !changed {
		t.Fatalf("with a game: changed=%v err=%v", changed, err)
	}
	head, items := readItems(t, pl)
	if len(items) != 1 || string(head["version"]) != `"1.5"` {
		t.Errorf("new playlist: %s", head)
	}
}

func TestMergeRefusesAFileThatIsNotAPlaylist(t *testing.T) {
	dir, root := t.TempDir(), filepath.Join(t.TempDir(), "RomCure")
	pl := filepath.Join(dir, "x.lpl")
	if err := os.WriteFile(pl, []byte("not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := mergePlaylist(pl, root, oursIn(root, "Alpha"), nil); err == nil {
		t.Fatal("expected an error")
	}
	if data, _ := os.ReadFile(pl); string(data) != "not json" {
		t.Error("the file was overwritten")
	}
}

func TestRemoveFromPlaylistsTakesOnlyOurEntriesOut(t *testing.T) {
	dir, root := t.TempDir(), filepath.Join(t.TempDir(), "RomCure")
	mixed, other := filepath.Join(dir, "a.lpl"), filepath.Join(dir, "b.lpl")
	for _, p := range []string{mixed, other} {
		if err := os.WriteFile(p, []byte(userPlaylist), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := mergePlaylist(mixed, root, oursIn(root, "Alpha"), nil); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(other)

	if n, err := removeFromPlaylists(dir, root); err != nil || n != 1 {
		t.Fatalf("removed from %d playlists, err=%v", n, err)
	}
	if _, items := readItems(t, mixed); len(items) != 1 || items[0]["label"] != "Own Game" {
		t.Errorf("after removal: %v", items)
	}
	if after, _ := os.ReadFile(other); string(after) != string(before) {
		t.Error("a playlist with none of our entries was rewritten")
	}
}

func TestInsideMatchesBothSlashesAndNotSiblingFolders(t *testing.T) {
	for _, c := range []struct {
		item, folder string
		want         bool
	}{
		{`C:\Out\gba\a.gba`, `C:\Out`, true},
		{`C:/Out/gba/a.gba`, `C:\Out`, true},
		{`C:\Out2\gba\a.gba`, `C:\Out`, false},
		{`C:\Out`, `C:\Out`, false},
	} {
		if got := inside(c.item, c.folder); got != c.want {
			t.Errorf("inside(%q, %q) = %v", c.item, c.folder, got)
		}
	}
}

func TestGenerateRetroArchMergesIntoRetroArchsOwnPlaylistAndIsIdempotent(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "Super Game (USA).nes"), "raw-rom-content")
	db := openTestDB(t)
	syncSystem(t, db, root, "nes")

	dbName, _ := LibretroDBName("nes")
	playlists := t.TempDir()
	pl := filepath.Join(playlists, dbName+".lpl")
	theirs := `{"version": "1.5", "default_core_path": "/cores/fceumm.dll", "items": [{"path": "/their/Own.nes", "label": "Own", "core_path": "DETECT", "core_name": "DETECT", "crc32": "1234ABCD|crc", "db_name": "` + dbName + `.lpl"}]}`
	writeFile(t, pl, theirs)

	out := t.TempDir()
	opts := Options{OutputRoot: out, PlaylistDir: playlists}
	if _, err := GenerateRetroArch(db, "nes", opts); err != nil {
		t.Fatal(err)
	}
	head, items := readItems(t, pl)
	if len(items) != 2 || items[0]["label"] != "Own" || items[1]["label"] != "Super Game" {
		t.Fatalf("items = %v", items)
	}
	if !strings.HasPrefix(items[1]["path"], out) || string(head["default_core_path"]) != `"/cores/fceumm.dll"` {
		t.Errorf("our item or the default core is wrong: %v %s", items[1], head["default_core_path"])
	}

	first, _ := os.ReadFile(pl)
	if _, err := GenerateRetroArch(db, "nes", opts); err != nil {
		t.Fatal(err)
	}
	if again, _ := os.ReadFile(pl); string(again) != string(first) {
		t.Error("a second run changed RetroArch's playlist")
	}
}

func TestDefaultCoreIsSetOnlyWhereThePlaylistHasNone(t *testing.T) {
	root := t.TempDir()
	out := filepath.Join(root, "RomCure")
	core := defaultCore{Path: "/cores/mgba_libretro.dll", Name: "Nintendo - Game Boy Advance (mGBA)"}

	// A new playlist gets it.
	fresh := filepath.Join(root, "fresh.lpl")
	if _, err := mergePlaylist(fresh, out, oursIn(out, "Alpha"), &core); err != nil {
		t.Fatal(err)
	}
	head, _ := readItems(t, fresh)
	if string(head["default_core_path"]) != `"/cores/mgba_libretro.dll"` || string(head["default_core_name"]) != `"Nintendo - Game Boy Advance (mGBA)"` {
		t.Errorf("new playlist: %s %s", head["default_core_path"], head["default_core_name"])
	}
	// So does one RetroArch wrote with "DETECT", and a second run changes nothing.
	detect := filepath.Join(root, "detect.lpl")
	if err := os.WriteFile(detect, []byte(`{"version":"1.5","default_core_path":"DETECT","default_core_name":"DETECT","items":[]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := mergePlaylist(detect, out, oursIn(out, "Alpha"), &core); err != nil {
		t.Fatal(err)
	}
	if head, _ := readItems(t, detect); string(head["default_core_path"]) != `"/cores/mgba_libretro.dll"` {
		t.Errorf("DETECT playlist: %s", head["default_core_path"])
	}
	if changed, err := mergePlaylist(detect, out, oursIn(out, "Alpha"), &core); err != nil || changed {
		t.Errorf("second run: changed=%v err=%v", changed, err)
	}
	// A core the user picked is kept.
	own := filepath.Join(root, "own.lpl")
	if err := os.WriteFile(own, []byte(userPlaylist), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := mergePlaylist(own, out, oursIn(out, "Alpha"), &defaultCore{Path: "/cores/gpsp_libretro.dll", Name: "gpSP"}); err != nil {
		t.Fatal(err)
	}
	if head, _ := readItems(t, own); string(head["default_core_path"]) != `"/cores/mgba_libretro.dll"` || string(head["default_core_name"]) != `"Nintendo - Game Boy Advance (mGBA)"` {
		t.Errorf("the user's core: %s", head["default_core_path"])
	}
}

func TestGenerateRetroArchAsksForACoreByContentTypeAndReportsWhy(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "Super Game (USA).nes"), "raw-rom-content")
	db := openTestDB(t)
	syncSystem(t, db, root, "nes")

	var asked []string
	opts := Options{OutputRoot: t.TempDir(), PlaylistDir: t.TempDir(),
		ChooseCore: func(exts []string) (string, string, string, bool) {
			asked = exts
			return "/cores/mesen_libretro.dll", "Nintendo - NES / Famicom (Mesen)", "the recommended core", true
		}}
	rep, err := GenerateRetroArch(db, "nes", opts)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(asked, ",") != "nes" {
		t.Errorf("asked with %v, want [nes]", asked)
	}
	if len(rep.Notes) != 1 || !strings.Contains(rep.Notes[0], "Mesen): the recommended core") {
		t.Errorf("notes = %v", rep.Notes)
	}
	// The core is in the playlist now, so a second run says nothing.
	if rep, _ := GenerateRetroArch(db, "nes", opts); len(rep.Notes) != 0 {
		t.Errorf("second run notes = %v", rep.Notes)
	}
}

func TestARerunKeepsTheCoreRetroArchRecordedOnOurEntry(t *testing.T) {
	dir, root := t.TempDir(), filepath.Join(t.TempDir(), "RomCure")
	pl := filepath.Join(dir, "x.lpl")
	if _, err := mergePlaylist(pl, root, oursIn(root, "Alpha", "Beta"), nil); err != nil {
		t.Fatal(err)
	}
	// The user launches Alpha with mGBA; RetroArch records it on the entry and
	// saves the file in its own layout, with a field of its own.
	head, items := readItems(t, pl)
	items[0]["core_path"], items[0]["core_name"], items[0]["crc32"] = "/cores/mgba_libretro.dll", "mGBA", "1A2B3C4D|crc"
	items[0]["subsystem_ident"] = ""
	raw, _ := json.Marshal(items)
	head["items"] = raw
	saved, _ := json.Marshal(head) // compact, unlike ours
	if err := os.WriteFile(pl, saved, 0o644); err != nil {
		t.Fatal(err)
	}

	// The same games again: only the layout differs, so nothing is written.
	if changed, err := mergePlaylist(pl, root, oursIn(root, "Alpha", "Beta"), nil); err != nil || changed {
		t.Fatalf("same games: changed=%v err=%v", changed, err)
	}
	// Beta goes: Alpha keeps the core RetroArch recorded, and its extra field.
	if changed, err := mergePlaylist(pl, root, oursIn(root, "Alpha"), nil); err != nil || !changed {
		t.Fatalf("Beta removed: changed=%v err=%v", changed, err)
	}
	_, items = readItems(t, pl)
	if len(items) != 1 || items[0]["core_path"] != "/cores/mgba_libretro.dll" || items[0]["core_name"] != "mGBA" ||
		items[0]["crc32"] != "1A2B3C4D|crc" || items[0]["label"] != "Alpha" {
		t.Errorf("after rerun: %v", items)
	}
	if _, ok := items[0]["subsystem_ident"]; !ok {
		t.Error("RetroArch's own field was dropped")
	}
}
