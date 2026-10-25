package profile

import (
	"context"

	"romcure/internal/library"
	"romcure/internal/rules"
	"romcure/internal/scan"

	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestGenerateRawUsesSystemFolderAndRawProfileID(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "Game (USA).nes"), "rom")
	db := openTestDB(t)
	syncSystem(t, db, root, "nes")
	out := t.TempDir()
	rep, err := GenerateRaw(db, "nes", Options{OutputRoot: out})
	if err != nil || len(rep.Linked) != 1 || filepath.Base(filepath.Dir(rep.Linked[0].Path)) != "nes" {
		t.Fatalf("err=%v rep=%+v", err, rep)
	}
	var n int
	db.QueryRow(`SELECT COUNT(*) FROM link_state WHERE profile_id = 'raw'`).Scan(&n)
	if n != 1 {
		t.Errorf("raw link_state rows = %d", n)
	}
}

func TestExportToDeviceCopiesIncrementallyAndPlansFirst(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "Game (USA).nes")
	writeFile(t, src, "rom-v1")
	writeZip(t, filepath.Join(root, "Zipped (USA).zip"), map[string]string{"Zipped (USA).nes": "zipped-rom"})
	db := openTestDB(t)
	syncSystem(t, db, root, "nes")

	dev := t.TempDir()
	opts := DeviceExportOptions{Options: Options{OutputRoot: dev}, Frontend: FrontendESDE, Systems: []string{"nes"}}

	plan, err := PlanDeviceExport(db, opts)
	if err != nil || !plan.OK() {
		t.Fatalf("plan=%+v err=%v", plan, err)
	}
	if plan.TotalBytes != int64(len("rom-v1"))+fileSize(t, filepath.Join(root, "Zipped (USA).zip")) || plan.Systems[0].Games != 2 {
		t.Errorf("estimate = %+v", plan)
	}

	plan, reports, err := ExportToDevice(db, opts)
	if err != nil || !plan.OK() || len(reports["nes"].Errors) != 0 || len(reports["nes"].Linked) != 2 {
		t.Fatalf("export: plan=%+v reports=%+v err=%v", plan, reports, err)
	}
	for _, l := range reports["nes"].Linked {
		fi, err := os.Lstat(l.Path)
		if err != nil || !fi.Mode().IsRegular() {
			t.Errorf("%s must be a real copy, mode=%v err=%v", l.Path, fi.Mode(), err)
		}
	}
	var linkType string
	db.QueryRow(`SELECT link_type FROM link_state WHERE profile_id LIKE 'device:%' LIMIT 1`).Scan(&linkType)
	if linkType != "copy" {
		t.Errorf("link_type = %q, want copy", linkType)
	}

	// Second run writes nothing.
	copied := filepath.Join(dev, "nes", "Game.nes")
	if _, err := os.Stat(copied); err != nil {
		t.Fatalf("expected %s: %v", copied, err)
	}
	old := time.Now().Add(-time.Hour)
	os.Chtimes(copied, old, old)
	if _, _, err := ExportToDevice(db, opts); err != nil {
		t.Fatal(err)
	}
	if st, _ := os.Stat(copied); !st.ModTime().Equal(old.Truncate(time.Second)) && st.ModTime().After(old.Add(time.Minute)) {
		t.Error("an up-to-date copy was rewritten")
	}

	// A changed source (different size) is re-copied.
	writeFile(t, src, "rom-v2-longer")
	syncSystem(t, db, root, "nes")
	if _, _, err := ExportToDevice(db, opts); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(copied); string(b) != "rom-v2-longer" {
		t.Errorf("copy not refreshed: %q", b)
	}
}

func fileSize(t *testing.T, p string) int64 {
	st, err := os.Stat(p)
	if err != nil {
		t.Fatal(err)
	}
	return st.Size()
}

func TestForceCopyRejectsFilesOverTheLimit(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "Big (USA).nes"), "0123456789")
	writeFile(t, filepath.Join(root, "Small (USA).nes"), "012")
	db := openTestDB(t)
	syncSystem(t, db, root, "nes")
	rep, err := GenerateESFamily(db, "nes", FrontendESDE, Options{OutputRoot: t.TempDir(), ForceCopy: true, MaxFileBytes: 5})
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Linked) != 1 || len(rep.Errors) != 1 || !strings.Contains(rep.Errors[0], "file limit") {
		t.Errorf("report = %+v", rep)
	}
}

func TestPlanBlocksWhenTooBig(t *testing.T) {
	p := ExportPlan{TotalBytes: 100, FreeBytes: 50}
	p.checkSpace()
	if p.OK() {
		t.Error("100 bytes into 50 free must block")
	}
	p = ExportPlan{TotalBytes: 10, FreeBytes: 50}
	p.checkSpace()
	if !p.OK() {
		t.Error("10 into 50 must pass")
	}
}

// When the 1G1R pick changes but the clean output name stays the same, the
// output must be re-pointed at the new pick, not silently kept.
func TestRegenerateReplacesOutputWhenThePickChanges(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "Alpha (USA).nes"), "usa")
	writeFile(t, filepath.Join(root, "Alpha (Europe).nes"), "eur")
	db := openTestDB(t)
	out := t.TempDir()
	gen := func(region string) {
		t.Helper()
		if err := scan.New(db).Scan(context.Background(), root, nil); err != nil {
			t.Fatal(err)
		}
		if _, err := library.Sync(db, root, "nes", rules.Preferences{RegionPriority: []string{region}}); err != nil {
			t.Fatal(err)
		}
		rep, err := GenerateRaw(db, "nes", Options{OutputRoot: out})
		if err != nil || len(rep.Errors) != 0 {
			t.Fatalf("%v %v", err, rep.Errors)
		}
	}
	gen("USA")
	if got := readFileContent(t, filepath.Join(out, "nes", "Alpha.nes")); got != "usa" {
		t.Fatalf("first pick = %q", got)
	}
	gen("Europe")
	if got := readFileContent(t, filepath.Join(out, "nes", "Alpha.nes")); got != "eur" {
		t.Errorf("after switching preset the output still has %q", got)
	}

	// A file the app never created is left alone.
	foreign := filepath.Join(out, "nes", "Mine.nes")
	writeFile(t, foreign, "user file")
	gen("Europe")
	if got := readFileContent(t, foreign); got != "user file" {
		t.Error("a file the app did not create was touched")
	}
}

// A game that leaves the library takes its output with it; files the app
// did not create stay.
func TestRegenerateRemovesOutputsOfGamesThatLeft(t *testing.T) {
	root := t.TempDir()
	keep := filepath.Join(root, "Keep (USA).nes")
	gone := filepath.Join(root, "Gone (USA).nes")
	writeFile(t, keep, "k")
	writeFile(t, gone, "g")
	db := openTestDB(t)
	out := t.TempDir()
	gen := func() Report {
		t.Helper()
		if err := scan.New(db).Scan(context.Background(), root, nil); err != nil {
			t.Fatal(err)
		}
		if _, err := library.Sync(db, root, "nes", rules.Preferences{}); err != nil {
			t.Fatal(err)
		}
		rep, err := GenerateRaw(db, "nes", Options{OutputRoot: out})
		if err != nil || len(rep.Errors) != 0 {
			t.Fatalf("%v %v", err, rep.Errors)
		}
		return rep
	}
	gen()
	mine := filepath.Join(out, "nes", "Mine.txt")
	writeFile(t, mine, "user")
	if _, err := os.Lstat(filepath.Join(out, "nes", "Gone.nes")); err != nil {
		t.Fatal("setup: Gone.nes not created")
	}

	os.Remove(gone)
	rep := gen()
	if _, err := os.Lstat(filepath.Join(out, "nes", "Gone.nes")); err == nil {
		t.Error("output of a removed game is still there")
	}
	if _, err := os.Lstat(filepath.Join(out, "nes", "Keep.nes")); err != nil {
		t.Error("output of a kept game was removed")
	}
	if _, err := os.Stat(mine); err != nil {
		t.Error("a file the app did not create was removed")
	}
	if len(rep.Notes) != 1 || !strings.Contains(rep.Notes[0], "removed 1") {
		t.Errorf("notes = %v", rep.Notes)
	}
	if rows, _ := db.ListLinkStates("raw"); len(rows) != 1 {
		t.Errorf("link_state rows = %d, want 1", len(rows))
	}
	if _, err := os.Stat(keep); err != nil {
		t.Error("source touched")
	}
}
