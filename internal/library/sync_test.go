package library

import (
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"hash/crc32"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"romcure/internal/dat"
	"romcure/internal/rules"
	"romcure/internal/scan"
	"romcure/internal/store"
)

func openTestDB(t *testing.T) *store.DB {
	t.Helper()
	db, err := store.Open(filepath.Join(t.TempDir(), "library.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// crcOf scans dir and returns the CRC32 the scanner computed for name.
func crcOf(t *testing.T, db *store.DB, path string) string {
	t.Helper()
	rec, err := db.GetFileByPath(path)
	if err != nil || rec == nil {
		t.Fatalf("GetFileByPath(%s): rec=%v err=%v", path, rec, err)
	}
	return rec.CRC32.String
}

func TestSyncTitleNormalizationGrouping(t *testing.T) {
	root := t.TempDir()
	usaPath := filepath.Join(root, "Game (USA).nes")
	eurPath := filepath.Join(root, "Game (Europe).nes")
	writeFile(t, usaPath, "usa-data")
	writeFile(t, eurPath, "eur-data")

	db := openTestDB(t)
	if err := scan.New(db).Scan(context.Background(), root, nil); err != nil {
		t.Fatalf("Scan: %v", err)
	}

	report, err := Sync(db, root, "nes", rules.Preferences{RegionPriority: []string{"USA", "Europe"}})
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}

	if len(report.Selected) != 1 {
		t.Fatalf("len(Selected) = %d, want 1 (both variants should be in one group)", len(report.Selected))
	}
	if len(report.Excluded) != 1 {
		t.Fatalf("len(Excluded) = %d, want 1", len(report.Excluded))
	}

	wantKey := GameKey([]string{crcOf(t, db, usaPath)})
	if report.Selected[0].GameKey != wantKey {
		t.Errorf("Selected = %+v, want the USA variant's GameKey %s (RegionPriority favors it)", report.Selected[0], wantKey)
	}

	// Confirm it actually persisted: exactly one games row is selected
	// within the group, and the other is present but not selected.
	var selectedCount, totalCount int
	if err := db.QueryRow(`SELECT COUNT(*) FROM games WHERE system_id = 'nes' AND is_selected = 1`).Scan(&selectedCount); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM games WHERE system_id = 'nes'`).Scan(&totalCount); err != nil {
		t.Fatal(err)
	}
	if selectedCount != 1 || totalCount != 2 {
		t.Errorf("games rows: selected=%d total=%d, want 1 and 2", selectedCount, totalCount)
	}
}

func TestSyncIsIdempotent(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "Game (USA).nes"), "usa-data")
	writeFile(t, filepath.Join(root, "Game (Europe).nes"), "eur-data")

	db := openTestDB(t)
	if err := scan.New(db).Scan(context.Background(), root, nil); err != nil {
		t.Fatalf("Scan: %v", err)
	}

	prefs := rules.Preferences{RegionPriority: []string{"USA", "Europe"}}
	if _, err := Sync(db, root, "nes", prefs); err != nil {
		t.Fatalf("first Sync: %v", err)
	}

	var groupID1, gameID1 int64
	if err := db.QueryRow(`SELECT id FROM game_groups`).Scan(&groupID1); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT id FROM games WHERE is_selected = 1`).Scan(&gameID1); err != nil {
		t.Fatal(err)
	}

	if _, err := Sync(db, root, "nes", prefs); err != nil {
		t.Fatalf("second Sync: %v", err)
	}

	var groupCount, gameCount int
	if err := db.QueryRow(`SELECT COUNT(*) FROM game_groups`).Scan(&groupCount); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM games`).Scan(&gameCount); err != nil {
		t.Fatal(err)
	}
	if groupCount != 1 || gameCount != 2 {
		t.Errorf("after re-sync: game_groups=%d games=%d, want 1 and 2 (re-running must not duplicate rows)", groupCount, gameCount)
	}

	var groupID2, gameID2 int64
	if err := db.QueryRow(`SELECT id FROM game_groups`).Scan(&groupID2); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT id FROM games WHERE is_selected = 1`).Scan(&gameID2); err != nil {
		t.Fatal(err)
	}
	if groupID1 != groupID2 || gameID1 != gameID2 {
		t.Errorf("row ids changed across an unchanged re-sync: group %d->%d, game %d->%d", groupID1, groupID2, gameID1, gameID2)
	}
}

// buildPCDat returns a Logiqx XML DAT with a USA parent and a Japan clone,
// with crc/size patched in from the real scanned files.
func buildPCDat(parentName, cloneName, parentCRC string, parentSize int64, cloneCRC string, cloneSize int64) string {
	tmpl := `<?xml version="1.0"?>
<datafile>
	<header><name>Test System</name></header>
	<game name="PARENT_NAME">
		<rom name="parent.rom" size="PARENT_SIZE" crc="PARENT_CRC"/>
	</game>
	<game name="CLONE_NAME" cloneof="PARENT_NAME">
		<rom name="clone.rom" size="CLONE_SIZE" crc="CLONE_CRC"/>
	</game>
</datafile>`
	r := strings.NewReplacer(
		"PARENT_NAME", parentName, "CLONE_NAME", cloneName,
		"PARENT_SIZE", itoa(parentSize), "PARENT_CRC", parentCRC,
		"CLONE_SIZE", itoa(cloneSize), "CLONE_CRC", cloneCRC,
	)
	return r.Replace(tmpl)
}

func itoa(n int64) string {
	return fmt.Sprint(n)
}

func TestSyncManualOverrideSurvivesDATRename(t *testing.T) {
	root := t.TempDir()
	usaPath := filepath.Join(root, "parent.rom")
	jpnPath := filepath.Join(root, "clone.rom")
	writeFile(t, usaPath, "parent-content")
	writeFile(t, jpnPath, "clone-content-x")

	db := openTestDB(t)
	if err := scan.New(db).Scan(context.Background(), root, nil); err != nil {
		t.Fatalf("Scan: %v", err)
	}

	usaRec, err := db.GetFileByPath(usaPath)
	if err != nil || usaRec == nil {
		t.Fatalf("GetFileByPath(usa): %v, %v", usaRec, err)
	}
	jpnRec, err := db.GetFileByPath(jpnPath)
	if err != nil || jpnRec == nil {
		t.Fatalf("GetFileByPath(jpn): %v, %v", jpnRec, err)
	}

	// --- v1 DAT: original names ---
	v1 := buildPCDat("Super Game (USA)", "Super Game (Japan)", usaRec.CRC32.String, usaRec.Size, jpnRec.CRC32.String, jpnRec.Size)
	df1, err := dat.Parse(strings.NewReader(v1))
	if err != nil {
		t.Fatalf("dat.Parse v1: %v", err)
	}
	v1DatID, err := dat.Import(db, df1, "nes", "/v1.dat")
	if err != nil {
		t.Fatalf("dat.Import v1: %v", err)
	}

	prefs := rules.Preferences{RegionPriority: []string{"USA", "Japan"}}
	if _, err := Sync(db, root, "nes", prefs); err != nil {
		t.Fatalf("Sync (v1): %v", err)
	}

	// Natural 1G1R pick should be the USA parent (region priority).
	usaKey := GameKey([]string{usaRec.CRC32.String})
	jpnKey := GameKey([]string{jpnRec.CRC32.String})

	var groupID int64
	if err := db.QueryRow(`SELECT id FROM game_groups WHERE system_id = 'nes'`).Scan(&groupID); err != nil {
		t.Fatalf("query group id: %v", err)
	}

	var selectedKey string
	if err := db.QueryRow(`SELECT game_key FROM games WHERE group_id = ? AND is_selected = 1`, groupID).Scan(&selectedKey); err != nil {
		t.Fatalf("query selected game: %v", err)
	}
	if selectedKey != usaKey {
		t.Fatalf("natural selection = %s, want the USA parent %s", selectedKey, usaKey)
	}

	// --- Manual override: force the Japan clone instead ---
	if err := db.SetOverride(groupID, jpnKey, "prefer the Japan debug build"); err != nil {
		t.Fatalf("SetOverride: %v", err)
	}
	report, err := Sync(db, root, "nes", prefs)
	if err != nil {
		t.Fatalf("Sync (after override): %v", err)
	}
	if len(report.Overridden) != 1 || report.Overridden[0].GameKey != jpnKey {
		t.Fatalf("Overridden = %+v, want exactly the Japan clone", report.Overridden)
	}

	if err := db.QueryRow(`SELECT game_key FROM games WHERE group_id = ? AND is_selected = 1`, groupID).Scan(&selectedKey); err != nil {
		t.Fatalf("query selected game after override: %v", err)
	}
	if selectedKey != jpnKey {
		t.Fatalf("selected after override = %s, want the overridden Japan clone %s", selectedKey, jpnKey)
	}

	// --- Simulate a DAT update that renames both entries ---
	// Same content (same crc32/size), different game names — exactly the
	// scenario spec §2.11 exists for. Remove the old DAT's entries first
	// (cascades from dats) so MatchByHash doesn't see two ambiguous rows
	// with the same crc32/size.
	if _, err := db.Exec(`DELETE FROM dats WHERE id = ?`, v1DatID); err != nil {
		t.Fatalf("delete v1 dat: %v", err)
	}
	v2 := buildPCDat("Super Great Game (USA)", "Super Great Game (Japan)", usaRec.CRC32.String, usaRec.Size, jpnRec.CRC32.String, jpnRec.Size)
	df2, err := dat.Parse(strings.NewReader(v2))
	if err != nil {
		t.Fatalf("dat.Parse v2: %v", err)
	}
	if _, err := dat.Import(db, df2, "nes", "/v2.dat"); err != nil {
		t.Fatalf("dat.Import v2: %v", err)
	}

	report, err = Sync(db, root, "nes", prefs)
	if err != nil {
		t.Fatalf("Sync (after rename): %v", err)
	}

	// The override must still be in effect: group_key is content-hash
	// based (unaffected by the rename), so the same game_groups row (same
	// id) was upserted again, and the override — keyed by that id — still
	// applies.
	var groupIDAfterRename int64
	if err := db.QueryRow(`SELECT id FROM game_groups WHERE system_id = 'nes'`).Scan(&groupIDAfterRename); err != nil {
		t.Fatalf("query group id after rename: %v", err)
	}
	if groupIDAfterRename != groupID {
		t.Fatalf("group id changed across a DAT rename: %d -> %d, want it stable", groupID, groupIDAfterRename)
	}

	if err := db.QueryRow(`SELECT game_key FROM games WHERE group_id = ? AND is_selected = 1`, groupID).Scan(&selectedKey); err != nil {
		t.Fatalf("query selected game after rename: %v", err)
	}
	if selectedKey != jpnKey {
		t.Fatalf("selected after DAT rename = %s, want the override to still hold (%s)", selectedKey, jpnKey)
	}
	if len(report.Overridden) != 1 || report.Overridden[0].GameKey != jpnKey {
		t.Fatalf("Overridden after rename = %+v, want the override still reported", report.Overridden)
	}
}

func TestSyncReportsUnmatched(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "homebrew.nes"), "no-tags-at-all")

	db := openTestDB(t)
	if err := scan.New(db).Scan(context.Background(), root, nil); err != nil {
		t.Fatalf("Scan: %v", err)
	}

	report, err := Sync(db, root, "nes", rules.Preferences{})
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if len(report.Unmatched) != 1 {
		t.Fatalf("len(Unmatched) = %d, want 1", len(report.Unmatched))
	}
}

// A Redump multi-track disc is one DAT game with a .cue plus one .bin per
// track, every file hash-verified. It must still be exactly one library
// game (the .cue), not one per track.
func TestSyncMultiTrackRedumpDiscIsOneGame(t *testing.T) {
	root := t.TempDir()
	cue := filepath.Join(root, "Disc (USA).cue")
	t1 := filepath.Join(root, "Disc (USA) (Track 1).bin")
	t2 := filepath.Join(root, "Disc (USA) (Track 2).bin")
	writeFile(t, t1, "track-one-data")
	writeFile(t, t2, "track-two-audio")
	writeFile(t, cue, "FILE \"Disc (USA) (Track 1).bin\" BINARY\n  TRACK 01 MODE2/2352\n    INDEX 01 00:00:00\nFILE \"Disc (USA) (Track 2).bin\" BINARY\n  TRACK 02 AUDIO\n    INDEX 01 00:00:00\n")

	db := openTestDB(t)
	if err := scan.New(db).Scan(context.Background(), root, nil); err != nil {
		t.Fatal(err)
	}
	rom := func(name, path string, size int64) dat.Rom {
		return dat.Rom{Name: name, Size: size, CRC32: crcOf(t, db, path)}
	}
	df := &dat.Datafile{Format: dat.FormatLogiqxXML, Games: []dat.Game{{
		Name: "Disc (USA)",
		Roms: []dat.Rom{
			rom("Disc (USA).cue", cue, int64(len("x"))), // size fixed up below
			rom("Disc (USA) (Track 1).bin", t1, 14),
			rom("Disc (USA) (Track 2).bin", t2, 15),
		},
	}}}
	st, _ := os.Stat(cue)
	df.Games[0].Roms[0].Size = st.Size()
	if _, err := dat.Import(db, df, "psx", "/redump.dat"); err != nil {
		t.Fatal(err)
	}

	if _, err := Sync(db, root, "psx", rules.Preferences{}); err != nil {
		t.Fatal(err)
	}
	games, err := db.SelectedGames("psx")
	if err != nil {
		t.Fatal(err)
	}
	if len(games) != 1 {
		t.Fatalf("selected %d games, want 1: %+v", len(games), games)
	}
	if !strings.HasSuffix(games[0].UltimateSourceRef, ".cue") {
		t.Errorf("the game should be the .cue, got %s", games[0].UltimateSourceRef)
	}
}

// Redump names already contain "(Disc N)". Discs of one DAT-verified
// multi-disc game must keep their DAT names (no doubled tag), stay separate
// selections, and sidecar files next to them must not become games.
func TestSyncRedumpMultiDiscKeepsNamesAndIgnoresSidecars(t *testing.T) {
	root := t.TempDir()
	var roms []dat.Game
	db := openTestDB(t)
	for _, n := range []string{"1", "2"} {
		p := filepath.Join(root, "Big Game (Italy) (Disc "+n+").bin")
		cue := filepath.Join(root, "Big Game (Italy) (Disc "+n+").cue")
		writeFile(t, p, "disc-data-"+n)
		writeFile(t, cue, "FILE \"Big Game (Italy) (Disc "+n+").bin\" BINARY\n  TRACK 01 MODE2/2352\n    INDEX 01 00:00:00\n")
		writeFile(t, filepath.Join(root, "Big Game (Italy) (Disc "+n+").sbi"), "SBI")
	}
	if err := scan.New(db).Scan(context.Background(), root, nil); err != nil {
		t.Fatal(err)
	}
	for _, n := range []string{"1", "2"} {
		p := filepath.Join(root, "Big Game (Italy) (Disc "+n+").bin")
		roms = append(roms, dat.Game{Name: "Big Game (Italy) (Disc " + n + ")",
			Roms: []dat.Rom{{Name: "x.bin", Size: int64(len("disc-data-" + n)), CRC32: crcOf(t, db, p)}}})
	}
	if _, err := dat.Import(db, &dat.Datafile{Format: dat.FormatLogiqxXML, Games: roms}, "psx", "/r.dat"); err != nil {
		t.Fatal(err)
	}
	if _, err := Sync(db, root, "psx", rules.Preferences{}); err != nil {
		t.Fatal(err)
	}
	games, _ := db.SelectedGames("psx")
	names := map[string]bool{}
	for _, g := range games {
		names[g.DisplayName] = true
	}
	if len(games) != 2 || !names["Big Game (Italy) (Disc 1)"] || !names["Big Game (Italy) (Disc 2)"] {
		t.Fatalf("selected = %+v", games)
	}
}

func TestSyncPrunesGamesThatDisappear(t *testing.T) {
	root := t.TempDir()
	a, b := filepath.Join(root, "Alpha (USA).nes"), filepath.Join(root, "Beta (USA).nes")
	writeFile(t, a, "a-data")
	writeFile(t, b, "b-data")
	db := openTestDB(t)
	sync := func() {
		if err := scan.New(db).Scan(context.Background(), root, nil); err != nil {
			t.Fatal(err)
		}
		if _, err := Sync(db, root, "nes", rules.Preferences{}); err != nil {
			t.Fatal(err)
		}
	}
	sync()
	if g, _ := db.SelectedGames("nes"); len(g) != 2 {
		t.Fatalf("games = %d, want 2", len(g))
	}
	// Another root's games for the same system must never be pruned.
	other := t.TempDir()
	writeFile(t, filepath.Join(other, "Gamma (USA).nes"), "g-data")
	if err := scan.New(db).Scan(context.Background(), other, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := Sync(db, other, "nes", rules.Preferences{}); err != nil {
		t.Fatal(err)
	}

	os.Remove(b)
	sync()
	games, _ := db.SelectedGames("nes")
	names := map[string]bool{}
	for _, g := range games {
		names[g.DisplayName] = true
	}
	if len(games) != 2 || !names["Alpha"] && !names["Alpha (USA)"] {
		t.Fatalf("after delete games = %+v", games)
	}
	for n := range names {
		if strings.HasPrefix(n, "Beta") {
			t.Errorf("deleted game %q still present", n)
		}
	}
}

// Programs, shortcuts and scripts kept beside the games are not games.
func TestSyncIgnoresToolsAndShortcutsBesideGames(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "Real Game (USA).nes"), "game")
	for _, n := range []string{"chdman.exe", "convert.bat", "playlist.sh", "site.url", ".DS_Store", "._Real Game (USA).nes", "notes.txt"} {
		writeFile(t, filepath.Join(root, n), "x")
	}
	db := openTestDB(t)
	if err := scan.New(db).Scan(context.Background(), root, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := Sync(db, root, "nes", rules.Preferences{}); err != nil {
		t.Fatal(err)
	}
	games, _ := db.SelectedGames("nes")
	if len(games) != 1 || games[0].DisplayName != "Real Game" {
		t.Fatalf("games = %+v, want only Real Game", games)
	}
}

// A disc inside an archive (cue plus one bin per track, every file listed by
// Redump) is one game: the sheet, not the sheet and each track.
func TestSyncDiscInsideAnArchiveIsOneGame(t *testing.T) {
	root := t.TempDir()
	cueText := "FILE \"Disc (USA) (Track 1).bin\" BINARY\n  TRACK 01 MODE2/2352\n    INDEX 01 00:00:00\nFILE \"Disc (USA) (Track 2).bin\" BINARY\n  TRACK 02 AUDIO\n    INDEX 01 00:00:00\n"
	members := map[string]string{
		"Disc (USA).cue":           cueText,
		"Disc (USA) (Track 1).bin": "track-one-data",
		"Disc (USA) (Track 2).bin": "track-two-audio",
	}
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, content := range members {
		w, _ := zw.Create(name)
		w.Write([]byte(content))
	}
	zw.Close()
	if err := os.WriteFile(filepath.Join(root, "Disc (USA).zip"), buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}

	db := openTestDB(t)
	if err := scan.New(db).Scan(context.Background(), root, nil); err != nil {
		t.Fatal(err)
	}
	var roms []dat.Rom
	for name, content := range members {
		roms = append(roms, dat.Rom{Name: name, Size: int64(len(content)), CRC32: fmt.Sprintf("%08x", crc32.ChecksumIEEE([]byte(content)))})
	}
	df := &dat.Datafile{Format: dat.FormatLogiqxXML, Games: []dat.Game{{Name: "Disc (USA)", Roms: roms}}}
	if _, err := dat.Import(db, df, "psx", "/redump.dat"); err != nil {
		t.Fatal(err)
	}

	if _, err := Sync(db, root, "psx", rules.Preferences{}); err != nil {
		t.Fatal(err)
	}
	groups, err := db.ListGroups("psx")
	if err != nil {
		t.Fatal(err)
	}
	if len(groups) != 1 || len(groups[0].Variants) != 1 {
		t.Fatalf("groups = %+v, want one game with one version", groups)
	}
	if v := groups[0].Variants[0]; !strings.HasSuffix(v.MemberName, ".cue") || !v.Selected {
		t.Errorf("the game should be the .cue, got %+v", v)
	}
	games, _ := db.SelectedGames("psx")
	if len(games) != 1 || games[0].MemberName != "Disc (USA).cue" {
		t.Errorf("selected = %+v", games)
	}
}

// The two sides of a C64 disk are two parts of one game: neither loses the
// 1G1R pick to the other.
func TestSyncSelectsBothSidesOfADisk(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "Quest of Ages (1987)(Publisher)(Disk 1 of 2 Side A)[a4].d64"), "side-a")
	writeFile(t, filepath.Join(root, "Quest of Ages (1987)(Publisher)(Disk 1 of 2)(Side B).d64"), "side-b")
	db := openTestDB(t)
	if err := scan.New(db).Scan(context.Background(), root, nil); err != nil {
		t.Fatal(err)
	}
	report, err := Sync(db, root, "c64", rules.Preferences{})
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Selected) != 2 || len(report.Excluded) != 0 {
		t.Fatalf("selected %d, excluded %d; want 2 and 0: %+v", len(report.Selected), len(report.Excluded), report)
	}
	titles := map[string]bool{}
	for _, e := range report.Selected {
		titles[e.Title] = true
	}
	if !titles["Quest of Ages (Disc 1)"] || !titles["Quest of Ages (Disc 2)"] {
		t.Errorf("titles = %v", titles)
	}
}

func TestSyncGivesABundledArchiveItsOwnUnmatchedReason(t *testing.T) {
	root := t.TempDir()
	zipPath := filepath.Join(root, "Some_Game.Pub.zip")
	f, _ := os.Create(zipPath)
	zw := zip.NewWriter(f)
	for _, n := range []string{"A.P00", "B.P00"} {
		w, _ := zw.Create(n)
		w.Write([]byte(n))
	}
	zw.Close()
	f.Close()
	db := openTestDB(t)
	if err := scan.New(db).Scan(context.Background(), root, nil); err != nil {
		t.Fatal(err)
	}
	report, err := Sync(db, root, "c64", rules.Preferences{})
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Unmatched) != 1 || report.Unmatched[0].Title != "Some Game" || !strings.Contains(report.Unmatched[0].Reason, "several files") {
		t.Errorf("unmatched = %+v", report.Unmatched)
	}
}
