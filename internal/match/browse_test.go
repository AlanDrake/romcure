package match

import (
	"archive/zip"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"romcure/internal/dat"
	"romcure/internal/scan"
	"romcure/internal/store"
)

const browseTestDAT = `<?xml version="1.0"?>
<datafile>
	<header><name>Nintendo - Nintendo Entertainment System</name></header>
	<game name="Known Game (USA)">
		<rom name="Known Game (USA).nes" size="SIZE" crc="CRC32"/>
	</game>
</datafile>`

func TestBrowse(t *testing.T) {
	root := t.TempDir()

	// A raw file that will be DAT-verified. Its size and CRC32 are read
	// back after scanning and patched into the DAT below, so this content
	// never has to match a hardcoded expectation.
	knownContent := []byte("known!")
	if err := os.WriteFile(filepath.Join(root, "Known Game (USA).nes"), knownContent, 0o644); err != nil {
		t.Fatal(err)
	}
	// A raw file that will only match heuristically (region tag, no DAT entry).
	if err := os.WriteFile(filepath.Join(root, "Unknown Game (Europe).nes"), []byte("unknown-content"), 0o644); err != nil {
		t.Fatal(err)
	}
	// A raw file with no recognizable naming convention at all.
	if err := os.WriteFile(filepath.Join(root, "homebrew.nes"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	// A sibling directory outside root, which Browse must not pick up.
	outsideRoot := t.TempDir()
	if err := os.WriteFile(filepath.Join(outsideRoot, "Should Not Appear.nes"), []byte("y"), 0o644); err != nil {
		t.Fatal(err)
	}

	db := openTestDB(t)
	if err := scan.New(db).Scan(context.Background(), root, nil); err != nil {
		t.Fatalf("Scan: %v", err)
	}

	// Compute the real CRC32 of the known file via the scanner's own
	// cached value, then import a DAT entry that matches it exactly.
	rec, err := db.GetFileByPath(filepath.Join(root, "Known Game (USA).nes"))
	if err != nil || rec == nil {
		t.Fatalf("GetFileByPath: rec=%v err=%v", rec, err)
	}
	datXML := strings.NewReplacer(
		"SIZE", fmt.Sprint(rec.Size),
		"CRC32", rec.CRC32.String,
	).Replace(browseTestDAT)
	df, err := dat.Parse(strings.NewReader(datXML))
	if err != nil {
		t.Fatalf("dat.Parse: %v", err)
	}
	if _, err := dat.Import(db, df, "nes", "/test.dat"); err != nil {
		t.Fatalf("dat.Import: %v", err)
	}

	items, counts, err := Browse(db, root, "nes")
	if err != nil {
		t.Fatalf("Browse: %v", err)
	}

	if len(items) != 3 {
		t.Fatalf("len(items) = %d, want 3", len(items))
	}
	for _, item := range items {
		if !strings.HasPrefix(item.Path, root) {
			t.Errorf("item path %q is outside root %q", item.Path, root)
		}
	}

	want := Counts{DATVerified: 1, Heuristic: 1, Unidentified: 1}
	if counts != want {
		t.Errorf("Counts = %+v, want %+v", counts, want)
	}
	if counts.Total() != 3 {
		t.Errorf("Total() = %d, want 3", counts.Total())
	}
}

func TestBrowseIncludesArchiveMembers(t *testing.T) {
	root := t.TempDir()
	writeTestZipForBrowse(t, filepath.Join(root, "set.zip"), map[string][]byte{
		"Zipped Game (USA).nes": []byte("zipped-content"),
	})

	db := openTestDB(t)
	if err := scan.New(db).Scan(context.Background(), root, nil); err != nil {
		t.Fatalf("Scan: %v", err)
	}

	items, counts, err := Browse(db, root, "nes")
	if err != nil {
		t.Fatalf("Browse: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("len(items) = %d, want 1", len(items))
	}
	if items[0].MemberName != "Zipped Game (USA).nes" {
		t.Errorf("MemberName = %q, want %q", items[0].MemberName, "Zipped Game (USA).nes")
	}
	if items[0].Path != filepath.Join(root, "set.zip") {
		t.Errorf("Path = %q, want the archive's path", items[0].Path)
	}
	if counts.Heuristic != 1 {
		t.Errorf("Counts = %+v, want Heuristic: 1", counts)
	}
}

func writeTestZipForBrowse(t *testing.T, path string, files map[string][]byte) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	zw := zip.NewWriter(f)
	for name, content := range files {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write(content); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestBrowseIgnoresSidecarFilesInsideArchives(t *testing.T) {
	root := t.TempDir()
	zipPath := filepath.Join(root, "set.zip")
	f, _ := os.Create(zipPath)
	zw := zip.NewWriter(f)
	for n, c := range map[string]string{"game.adf": "adf", "readme.txt": "hi"} {
		w, _ := zw.Create(n)
		w.Write([]byte(c))
	}
	zw.Close()
	f.Close()
	db, err := store.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if err := scan.New(db).Scan(context.Background(), root, nil); err != nil {
		t.Fatal(err)
	}
	items, _, err := Browse(db, root, "amiga")
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].MemberName != "game.adf" {
		t.Fatalf("items = %+v, want only game.adf", items)
	}
}
