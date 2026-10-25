package plan

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"romcure/internal/library"
	"romcure/internal/profile"
	"romcure/internal/rules"
	"romcure/internal/scan"
	"romcure/internal/store"
)

type fixture struct {
	t    *testing.T
	db   *store.DB
	src  string
	out  string
	game string
}

func newFixture(t *testing.T) *fixture {
	db, err := store.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	f := &fixture{t: t, db: db, src: t.TempDir(), out: t.TempDir()}
	f.game = filepath.Join(f.src, "Game (USA).nes")
	os.WriteFile(f.game, []byte("rom-v1"), 0o644)
	os.WriteFile(filepath.Join(f.src, "Other (USA).nes"), []byte("other"), 0o644)
	f.sync()
	return f
}

// sync rescans, re-selects and regenerates the profile.
func (f *fixture) sync() {
	f.t.Helper()
	if err := scan.New(f.db).Scan(context.Background(), f.src, nil); err != nil {
		f.t.Fatal(err)
	}
	if _, err := library.Sync(f.db, f.src, "nes", rules.Preferences{}); err != nil {
		f.t.Fatal(err)
	}
	rep, err := profile.GenerateRaw(f.db, "nes", profile.Options{OutputRoot: f.out})
	if err != nil || len(rep.Errors) != 0 {
		f.t.Fatalf("generate: %v %v", err, rep.Errors)
	}
}

func (f *fixture) verify() map[string]Finding {
	f.t.Helper()
	fs, err := VerifyProfile(f.db, "raw", f.out)
	if err != nil {
		f.t.Fatal(err)
	}
	m := map[string]Finding{}
	for _, x := range fs {
		m[filepath.Base(x.TargetPath)] = x
	}
	return m
}

func TestVerifyHealthyProfileIsAllOK(t *testing.T) {
	f := newFixture(t)
	fs := f.verify()
	if len(fs) != 2 {
		t.Fatalf("findings = %+v", fs)
	}
	for n, x := range fs {
		if x.Status != "ok" {
			t.Errorf("%s: %s %s", n, x.Status, x.Reason)
		}
	}
	var status string
	f.db.QueryRow(`SELECT link_status FROM link_state WHERE profile_id = 'raw' LIMIT 1`).Scan(&status)
	if status != "ok" {
		t.Errorf("link_status = %q", status)
	}
}

func TestVerifyDetectsMissingOutputAndMissingSource(t *testing.T) {
	f := newFixture(t)
	os.Remove(filepath.Join(f.out, "nes", "Other.nes"))
	os.Remove(f.game)
	fs := f.verify()
	if fs["Other.nes"].Status != "broken" || !strings.Contains(fs["Other.nes"].Reason, "missing") {
		t.Errorf("Other: %+v", fs["Other.nes"])
	}
	if fs["Game.nes"].Status != "broken" {
		t.Errorf("Game (source deleted): %+v", fs["Game.nes"])
	}
}

func TestVerifyDetectsSourceChangedAfterLinking(t *testing.T) {
	f := newFixture(t)
	os.WriteFile(f.game, []byte("rom-v2-different"), 0o644)
	if err := scan.New(f.db).Scan(context.Background(), f.src, nil); err != nil { // rescan only, no regenerate
		t.Fatal(err)
	}
	x := f.verify()["Game.nes"]
	if x.Status != "stale" {
		t.Errorf("changed source: %+v", x)
	}
}

func TestVerifyDetectsRedirectedSymlink(t *testing.T) {
	f := newFixture(t)
	target := filepath.Join(f.out, "nes", "Other.nes")
	if li, _ := os.Lstat(target); li.Mode()&os.ModeSymlink == 0 {
		t.Skip("symlinks unavailable here; profile fell back to copies")
	}
	os.Remove(target)
	if err := os.Symlink(f.game, target); err != nil {
		t.Skip("cannot create symlink: " + err.Error())
	}
	if x := f.verify()["Other.nes"]; x.Status != "stale" || !strings.Contains(x.Reason, "points at") {
		t.Errorf("redirected: %+v", x)
	}
}

func TestVerifyDetectsStaleCopy(t *testing.T) {
	f := newFixture(t)
	dev := t.TempDir()
	if _, err := profile.GenerateRaw(f.db, "nes", profile.Options{OutputRoot: dev, ProfileID: "usb", ForceCopy: true}); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(dev, "nes", "Other.nes"), []byte("truncated"), 0o644)
	os.WriteFile(filepath.Join(dev, "nes", "Other.nes"), []byte("x"), 0o644)
	fs, err := VerifyProfile(f.db, "usb", dev)
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, x := range fs {
		if strings.HasSuffix(x.TargetPath, "Other.nes") {
			found = true
			if x.Status != "stale" || !strings.Contains(x.Reason, "bytes") {
				t.Errorf("copy: %+v", x)
			}
		}
	}
	if !found {
		t.Fatal("no finding for Other.nes")
	}
}

func TestRepairRemovesOnlyFlaggedOutputsAndSyncRestoresThem(t *testing.T) {
	f := newFixture(t)
	os.WriteFile(f.game, []byte("rom-v2-different"), 0o644)
	scan.New(f.db).Scan(context.Background(), f.src, nil)
	fs, _ := VerifyProfile(f.db, "raw", f.out)

	n, err := Repair(f.out, fs)
	if err != nil || n != 1 {
		t.Fatalf("repaired %d err=%v, want 1", n, err)
	}
	if _, err := os.Lstat(filepath.Join(f.out, "nes", "Game.nes")); err == nil {
		t.Error("flagged output not removed")
	}
	if _, err := os.Lstat(filepath.Join(f.out, "nes", "Other.nes")); err != nil {
		t.Error("healthy output was removed")
	}
	if b, _ := os.ReadFile(f.game); string(b) != "rom-v2-different" {
		t.Error("the source must never be touched")
	}

	f.sync()
	for n, x := range f.verify() {
		if x.Status != "ok" {
			t.Errorf("after repair+sync %s: %s %s", n, x.Status, x.Reason)
		}
	}
}

func TestRepairRefusesPathsOutsideRoot(t *testing.T) {
	root := t.TempDir()
	victim := filepath.Join(t.TempDir(), "keep.txt")
	os.WriteFile(victim, []byte("x"), 0o644)
	rel, _ := filepath.Rel(root, victim)
	if _, err := Repair(root, []Finding{{TargetPath: rel, Status: "broken"}}); err == nil {
		t.Error("expected refusal for a path outside the output root")
	}
	if _, err := os.Stat(victim); err != nil {
		t.Error("victim file was removed")
	}
}

func TestSummarize(t *testing.T) {
	s := Summarize([]Finding{{Status: "ok"}, {Status: "ok"}, {Status: "broken"}, {Status: "stale"}})
	if s != (Summary{OK: 2, Broken: 1, Stale: 1}) {
		t.Errorf("%+v", s)
	}
}
