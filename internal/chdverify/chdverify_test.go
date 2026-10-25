package chdverify

import (
	"context"
	"fmt"
	"hash/crc32"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"romcure/internal/store"
	"romcure/internal/transcode"
)

func TestVerifyRealCHDRoundTripsTrackHashes(t *testing.T) {
	exe, _ := filepath.Abs(filepath.Join("..", "..", "tools", "bin", runtime.GOOS+"-"+runtime.GOARCH, "chdman"))
	if runtime.GOOS == "windows" {
		exe += ".exe"
	}
	if _, err := os.Stat(exe); err != nil {
		t.Skip("real chdman not present")
	}
	tool := &transcode.Chdman{Path: exe, Version: "test"}

	dir := t.TempDir()
	bin := make([]byte, 2352*300)
	for i := range bin {
		bin[i] = byte(i * 5)
	}
	os.WriteFile(filepath.Join(dir, "t.bin"), bin, 0o644)
	cue := filepath.Join(dir, "t.cue")
	os.WriteFile(cue, []byte("FILE \"t.bin\" BINARY\n  TRACK 01 MODE1/2352\n    INDEX 01 00:00:00\n"), 0o644)
	chd := filepath.Join(dir, "t.chd")
	if err := tool.Create(context.Background(), transcode.MediaCD, cue, chd, transcode.CodecsCompat[transcode.MediaCD], nil); err != nil {
		t.Fatal(err)
	}

	db, err := store.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	st, _ := os.Stat(chd)
	id, err := db.UpsertFile(store.FileRecord{Path: chd, Size: st.Size(), MtimeUnix: st.ModTime().Unix(), Kind: "raw"})
	if err != nil {
		t.Fatal(err)
	}
	rec := store.FileRecord{ID: id, Path: chd, Size: st.Size(), MtimeUnix: st.ModTime().Unix(), Kind: "raw"}

	v := &Verifier{DB: db, Tool: tool, ScratchDir: t.TempDir()}
	if err := v.Verify(context.Background(), rec); err != nil {
		t.Fatal(err)
	}
	tracks, err := db.CHDTracks(rec)
	if err != nil || len(tracks) != 1 {
		t.Fatalf("tracks=%v err=%v", tracks, err)
	}
	if want := fmt.Sprintf("%08x", crc32.ChecksumIEEE(bin)); tracks[0].CRC32 != want || tracks[0].Size != int64(len(bin)) {
		t.Errorf("track = %+v, want crc %s size %d", tracks[0], want, len(bin))
	}
	if left, _ := os.ReadDir(v.ScratchDir); len(left) != 0 {
		t.Errorf("scratch not cleaned: %v", left)
	}

	// A changed CHD invalidates the stored hashes.
	changed := rec
	changed.Size++
	if tr, _ := db.CHDTracks(changed); tr != nil {
		t.Error("hashes must not apply to a changed file")
	}
}
