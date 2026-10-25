package profile

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"romcure/internal/plan"
)

func TestMultiDiscPlaylistIsTrackedVerifiedAndRemovedWithItsSet(t *testing.T) {
	root := t.TempDir()
	writeCueBin(t, root, "Epic RPG (USA) (Disc 1)")
	writeCueBin(t, root, "Epic RPG (USA) (Disc 2)")
	db := openTestDB(t)
	syncSystem(t, db, root, "psx")
	out := t.TempDir()
	opts := Options{OutputRoot: out}

	if _, err := GenerateRetroArch(db, "psx", opts); err != nil {
		t.Fatal(err)
	}
	dbName, _ := LibretroDBName("psx")
	m3u := filepath.Join(out, dbName, "Epic RPG.m3u")
	rows, _ := db.ListLinkStates("retroarch")
	var found bool
	for _, r := range rows {
		if r.GameKey == "m3u:Epic RPG" {
			found = true
			if r.SourceKind != "generated" || r.LinkType != "copy" || !strings.HasSuffix(filepath.ToSlash(r.TargetPath), "Epic RPG.m3u") {
				t.Errorf("playlist record = %+v", r)
			}
		}
	}
	if !found {
		t.Fatalf("the playlist is not recorded in link_state: %+v", rows)
	}
	findings, err := plan.VerifyProfile(db, "retroarch", out)
	if err != nil {
		t.Fatal(err)
	}
	if s := plan.Summarize(findings); s.Broken != 0 || s.Stale != 0 {
		t.Errorf("a fresh output must verify clean: %+v %+v", s, findings)
	}

	// Re-running writes nothing: the playlist is not rewritten.
	before, _ := os.Stat(m3u)
	time.Sleep(30 * time.Millisecond)
	if _, err := GenerateRetroArch(db, "psx", opts); err != nil {
		t.Fatal(err)
	}
	after, _ := os.Stat(m3u)
	if !after.ModTime().Equal(before.ModTime()) {
		t.Error("the playlist was rewritten although nothing changed")
	}

	// A deleted playlist is reported by verification.
	os.Remove(m3u)
	findings, _ = plan.VerifyProfile(db, "retroarch", out)
	if s := plan.Summarize(findings); s.Broken != 1 {
		t.Errorf("a missing playlist must show as broken: %+v", s)
	}
	if _, err := GenerateRetroArch(db, "psx", opts); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(m3u); err != nil {
		t.Fatalf("the playlist should be regenerated: %v", err)
	}

	// When the set leaves the output, its playlist goes with it.
	if _, err := db.Exec(`UPDATE games SET is_selected = 0`); err != nil {
		t.Fatal(err)
	}
	if _, err := GenerateRetroArch(db, "psx", opts); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(m3u); err == nil {
		t.Error("the playlist of a set that is no longer wanted must be removed")
	}
	rows, _ = db.ListLinkStates("retroarch")
	if len(rows) != 0 {
		t.Errorf("link_state rows left: %+v", rows)
	}
}
