package match

import (
	"archive/zip"
	"context"
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"

	"romcure/internal/scan"
	"romcure/internal/store"
)

// psBin builds a raw (2352-byte sector) PlayStation track whose SYSTEM.CNF
// boots the given executable.
func psBin(exe string) []byte {
	sec := func(n int) []byte { return make([]byte, 2048) }
	data := make([][]byte, 24)
	for i := range data {
		data[i] = sec(i)
	}
	copy(data[16], "\x01CD001")
	binary.LittleEndian.PutUint32(data[16][156+2:], 20)
	binary.LittleEndian.PutUint32(data[16][156+10:], 2048)
	dir := data[20]
	dir[0] = 46
	binary.LittleEndian.PutUint32(dir[2:], 21)
	dir[32] = 12
	copy(dir[33:], "SYSTEM.CNF;1")
	copy(data[21], "BOOT = cdrom:\\"+exe+";1\r\n")
	var out []byte
	for _, s := range data {
		raw := make([]byte, 2352)
		copy(raw[24:], s)
		out = append(out, raw...)
	}
	return out
}

func TestPlayStationDiscsAreNamedFromTheirSerial(t *testing.T) {
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "ff7d1.cue"), []byte("FILE \"ff7d1.bin\" BINARY\n  TRACK 01 MODE2/2352\n    INDEX 01 00:00:00\n"), 0o644)
	os.WriteFile(filepath.Join(root, "ff7d1.bin"), psBin("SCUS_941.63"), 0o644)
	zf, _ := os.Create(filepath.Join(root, "crash.zip"))
	zw := zip.NewWriter(zf)
	w, _ := zw.Create("crash.cue")
	w.Write([]byte("FILE \"crash.bin\" BINARY\n  TRACK 01 MODE2/2352\n    INDEX 01 00:00:00\n"))
	w, _ = zw.Create("crash.bin")
	w.Write(psBin("SCUS_949.00"))
	zw.Close()
	zf.Close()

	db := openTestDB(t)
	if err := scan.New(db).Scan(context.Background(), root, nil); err != nil {
		t.Fatal(err)
	}
	// Without a serial list the discs stay as they were.
	items, _, _ := Browse(db, root, "psx")
	for _, it := range items {
		if it.Result.Confidence != ConfidenceUnidentified {
			t.Errorf("no list yet: %s %s = %+v", it.Path, it.MemberName, it.Result)
		}
	}

	db.ReplaceSerials("psx", "test", []store.SerialEntry{
		{Serial: "SCUS-94163", Name: "Final Fantasy VII (USA) (Disc 1)"},
		{Serial: "SCUS-94900", Name: "Crash Bandicoot (USA)"},
	})
	items, _, _ = Browse(db, root, "psx")
	got := map[string]Result{}
	for _, it := range items {
		got[filepath.Base(it.Path)+"|"+it.MemberName] = it.Result
	}
	if r := got["ff7d1.cue|"]; r.Title != "Final Fantasy VII (Disc 1)" || r.Region != "USA" || r.Confidence != ConfidenceHeuristic {
		t.Errorf("raw cue: %+v", r)
	}
	if r := got["crash.zip|crash.cue"]; r.Title != "Crash Bandicoot" || r.Region != "USA" {
		t.Errorf("zipped cue: %+v", r)
	}
}
