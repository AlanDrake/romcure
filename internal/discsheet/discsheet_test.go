package discsheet

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCueReferencedFiles(t *testing.T) {
	dir := t.TempDir()
	cuePath := filepath.Join(dir, "game.cue")
	content := `FILE "Game (Track 1).bin" BINARY
  TRACK 01 MODE2/2352
    INDEX 01 00:00:00
FILE "Game (Track 2).bin" BINARY
  TRACK 02 AUDIO
    INDEX 00 00:00:00
    INDEX 01 00:02:00
`
	if err := os.WriteFile(cuePath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := cueReferencedFiles(cuePath)
	if err != nil {
		t.Fatalf("cueReferencedFiles: %v", err)
	}
	want := []string{"Game (Track 1).bin", "Game (Track 2).bin"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("got[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestGdiReferencedFiles(t *testing.T) {
	dir := t.TempDir()
	gdiPath := filepath.Join(dir, "game.gdi")
	content := `3
1 0 4 2352 track01.bin 0
2 0 0 2352 track02.raw 0
3 600 4 2352 track03.bin 0
`
	if err := os.WriteFile(gdiPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := gdiReferencedFiles(gdiPath)
	if err != nil {
		t.Fatalf("gdiReferencedFiles: %v", err)
	}
	want := []string{"track01.bin", "track02.raw", "track03.bin"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("got[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestReferencedFilesNonSheetReturnsNil(t *testing.T) {
	got, err := ReferencedFiles("game.nes")
	if err != nil {
		t.Fatalf("ReferencedFiles: %v", err)
	}
	if got != nil {
		t.Errorf("got %v, want nil for a non-sheet extension", got)
	}
}
