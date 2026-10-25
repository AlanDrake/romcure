package match

import (
	"context"
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"

	"romcure/internal/scan"
)

func gcImage(id, title string) []byte {
	b := make([]byte, 0x9000)
	copy(b, id)
	binary.BigEndian.PutUint32(b[0x1C:], 0xC2339F3D)
	copy(b[0x20:], title)
	return b
}

// A GameCube disc with a scene file name is named after its header; a file
// whose name already identifies it keeps that name.
func TestUnidentifiedDiscsAreNamedFromTheirHeader(t *testing.T) {
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "scn-gzlt.gcm"), gcImage("GEXE01", "Example Adventure"), 0o644)
	os.WriteFile(filepath.Join(root, "Mario Party 4 (Europe) (Rev 1).iso"), gcImage("GMPP01", "Mario Party 4"), 0o644)
	db := openTestDB(t)
	if err := scan.New(db).Scan(context.Background(), root, nil); err != nil {
		t.Fatal(err)
	}
	items, _, err := Browse(db, root, "gc")
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]Result{}
	for _, it := range items {
		got[filepath.Base(it.Path)] = it.Result
	}
	if r := got["scn-gzlt.gcm"]; r.Title != "Example Adventure" || r.Region != "USA" || r.Confidence != ConfidenceHeuristic {
		t.Errorf("scene-named disc: %+v", r)
	}
	if r := got["Mario Party 4 (Europe) (Rev 1).iso"]; r.Title != "Mario Party 4" || r.Revision == "" {
		t.Errorf("a well-named file must keep its own tags: %+v", r)
	}
}

// The two discs of a scene-named game must stay two games, not two versions
// of one (1G1R would drop one).
func TestTheDiscsOfOneGameGetDifferentNames(t *testing.T) {
	root := t.TempDir()
	d1 := gcImage("GKBEAF", "Baten Kaitos")
	d2 := gcImage("GKBEAF", "Baten Kaitos")
	d2[6] = 1
	d2[0x100] = 2 // different content
	os.WriteFile(filepath.Join(root, "bk-a.gcm"), d1, 0o644)
	os.WriteFile(filepath.Join(root, "bk-b.gcm"), d2, 0o644)
	db := openTestDB(t)
	if err := scan.New(db).Scan(context.Background(), root, nil); err != nil {
		t.Fatal(err)
	}
	items, _, _ := Browse(db, root, "gc")
	titles := map[string]bool{}
	for _, it := range items {
		titles[it.Result.Title] = true
	}
	if !titles["Baten Kaitos"] || !titles["Baten Kaitos (Disc 2)"] {
		t.Errorf("titles = %v", titles)
	}
}
