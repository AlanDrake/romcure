package match

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
	"romcure/internal/scan"
)

// No-Intro lists NES ROMs by the CRC of the data after the 16-byte iNES
// header. A headered dump, raw or zipped, must still be DAT-verified.
func TestHeaderedDumpsMatchTheHeaderlessDATEntry(t *testing.T) {
	root := t.TempDir()
	body := bytes.Repeat([]byte{0x20, 0x10, 0xC0}, 3000)
	nes := append(append([]byte("NES\x1a"), make([]byte, 12)...), body...)
	os.WriteFile(filepath.Join(root, "Mario Bros. (World).nes"), nes, 0o644)

	zf, _ := os.Create(filepath.Join(root, "Mario Bros. (Japan).zip"))
	zw := zip.NewWriter(zf)
	w, _ := zw.Create("Mario Bros. (Japan).nes")
	w.Write(append(append([]byte("NES\x1a"), make([]byte, 12)...), body...))
	zw.Close()
	zf.Close()

	db := openTestDB(t)
	if err := scan.New(db).Scan(context.Background(), root, nil); err != nil {
		t.Fatal(err)
	}
	datXML := fmt.Sprintf(`<?xml version="1.0"?>
<datafile><header><name>Nintendo - Nintendo Entertainment System</name></header>
<game name="Mario Bros. (World)"><rom name="Mario Bros. (World).nes" size="%d" crc="%08x"/></game>
</datafile>`, len(body), crc32.ChecksumIEEE(body))
	df, err := dat.Parse(strings.NewReader(datXML))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := dat.Import(db, df, "nes", "/nes.dat"); err != nil {
		t.Fatal(err)
	}

	items, _, err := Browse(db, root, "nes")
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 {
		t.Fatalf("items = %+v", items)
	}
	for _, it := range items {
		if it.Result.Confidence != ConfidenceDATVerified || it.Result.Title != "Mario Bros. (World)" {
			t.Errorf("%s %s: %s %q, want DAT-verified Mario Bros. (World)", it.Path, it.MemberName, it.Result.Confidence, it.Result.Title)
		}
	}
}
