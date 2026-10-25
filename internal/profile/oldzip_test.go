package profile

import (
	"archive/zip"
	"context"
	"database/sql"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"romcure/internal/store"
)

// writeOldMethodZip writes a zip whose member claims PKZIP method 6 (implode):
// the bytes are stored, so only the header is what Go cannot decompress.
func writeOldMethodZip(t *testing.T, path, member string) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	zw := zip.NewWriter(f)
	zw.RegisterCompressor(6, func(w io.Writer) (io.WriteCloser, error) { return nopCloser{w}, nil })
	w, err := zw.CreateHeader(&zip.FileHeader{Name: member, Method: 6})
	if err != nil {
		t.Fatal(err)
	}
	w.Write([]byte("program data"))
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
}

type nopCloser struct{ io.Writer }

func (nopCloser) Close() error { return nil }

func TestOldZipMethodIsNotPassedThroughAndExtractionSaysWhy(t *testing.T) {
	dir := t.TempDir()
	old := filepath.Join(dir, "old.zip")
	writeOldMethodZip(t, old, "game.prg")
	plain := filepath.Join(dir, "plain.zip")
	writeZip(t, plain, map[string]string{"game.prg": "data"})

	// The policy reads the method the scan recorded; the path need not exist.
	for name, c := range map[string]struct {
		method sql.NullString
		want   ArchivePolicy
	}{
		"old":         {sql.NullString{String: "implode", Valid: true}, PolicyExtract},
		"fine":        {sql.NullString{Valid: true}, PolicyPassthrough},
		"not checked": {sql.NullString{}, PolicyPassthrough},
	} {
		got := archivePolicyFor(store.FileRecord{Path: filepath.Join(dir, "missing.zip"), Kind: "zip", ZipMethod: c.method}, true, "c64")
		if got != c.want {
			t.Errorf("%s: policy %s, want %s", name, got, c.want)
		}
	}
	_ = plain

	err := extractMember(context.Background(), "", old, "game.prg", filepath.Join(dir, "out", "game.prg"))
	if err == nil || !strings.Contains(err.Error(), "implode") || !strings.Contains(err.Error(), "7-Zip") {
		t.Errorf("error = %v, want it to name the method and 7-Zip", err)
	}
	if _, statErr := os.Stat(filepath.Join(dir, "out", "game.prg")); statErr == nil {
		t.Error("a file was written despite the error")
	}
}
