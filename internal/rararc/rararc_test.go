package rararc

import (
	"context"
	"hash/crc32"
	"io"
	"os"
	"path/filepath"
	"romcure/internal/rararc/rartest"
	"runtime"
	"strings"
	"testing"
)

// sevenZip finds the development copy of 7-Zip, or skips the test.
func sevenZip(t *testing.T) Tool { return Tool{Path: rartest.SevenZip(t)} }

func TestIsContinuation(t *testing.T) {
	for path, want := range map[string]bool{
		"Game.rar": false, "Game.part1.rar": false, "Game.part01.rar": false,
		"Game.part2.rar": true, "Game.PART03.RAR": true, "Game.part10.rar": true, "part2.rar": false,
	} {
		if got := IsContinuation(path); got != want {
			t.Errorf("IsContinuation(%q) = %v, want %v", path, got, want)
		}
	}
}

func TestParseListSkipsFoldersAndReadsCRCs(t *testing.T) {
	sep := "/"
	if runtime.GOOS == "windows" {
		sep = `\`
	}
	out := "7-Zip 26.05\n\nListing archive: x.rar\n\n--\nPath = x.rar\nType = Rar\n\n----------\n" +
		"Path = dir\nFolder = +\nSize = 0\nCRC =\n\n" +
		"Path = dir" + sep + "Game (USA).nes\nFolder = -\nSize = 40976\nCRC = 00AB12CD\n\n" +
		"Path = empty.txt\nFolder = -\nSize = 0\nCRC =\n\n"
	got := parseList(out)
	if len(got) != 2 {
		t.Fatalf("members = %+v", got)
	}
	if got[0] != (Member{Name: "dir/Game (USA).nes", Size: 40976, CRC32: "00ab12cd"}) {
		t.Errorf("member 0 = %+v", got[0])
	}
	if got[1].Name != "empty.txt" || got[1].CRC32 != "" {
		t.Errorf("member 1 = %+v", got[1])
	}
}

func TestListAndExtractARealArchive(t *testing.T) {
	tool := sevenZip(t)
	dir := t.TempDir()
	rom := []byte(strings.Repeat("ROMDATA", 1000))
	data := map[string][]byte{"Sub Dir/Game (USA).nes": rom, "[Weird] name & 1.txt": []byte("hi")}
	names := []string{"Sub Dir/Game (USA).nes", "[Weird] name & 1.txt"}
	archive := filepath.Join(dir, "set.rar")
	rartest.Write(t, archive, names, data)

	members, err := tool.List(context.Background(), archive)
	if err != nil {
		t.Fatal(err)
	}
	if len(members) != 2 {
		t.Fatalf("members = %+v", members)
	}
	if members[0].Name != "Sub Dir/Game (USA).nes" || members[0].Size != int64(len(rom)) ||
		members[0].CRC32 != strings.ToLower(hex8(crc32.ChecksumIEEE(rom))) {
		t.Errorf("member 0 = %+v", members[0])
	}

	for _, m := range members {
		dest := filepath.Join(dir, "out", filepath.Base(m.Name))
		if err := tool.Extract(context.Background(), archive, m.Name, dest); err != nil {
			t.Fatalf("extract %q: %v", m.Name, err)
		}
		got, _ := os.ReadFile(dest)
		if string(got) != string(data[m.Name]) {
			t.Errorf("%q extracted wrongly (%d bytes)", m.Name, len(got))
		}
	}
	if left, _ := filepath.Glob(filepath.Join(dir, "out", "*.romcure-tmp")); len(left) != 0 {
		t.Errorf("temp files left: %v", left)
	}
}

func hex8(n uint32) string {
	const digits = "0123456789abcdef"
	b := make([]byte, 8)
	for i := 7; i >= 0; i-- {
		b[i] = digits[n&0xf]
		n >>= 4
	}
	return string(b)
}

func TestDamagedArchivesFailCleanly(t *testing.T) {
	tool := sevenZip(t)
	dir := t.TempDir()
	bad := filepath.Join(dir, "bad.rar")
	os.WriteFile(bad, []byte("Rar!\x1a\x07\x01\x00 this is not a real archive"), 0o644)
	if _, err := tool.List(context.Background(), bad); err == nil {
		t.Error("listing a damaged archive should fail")
	}

	// A flipped data byte: the listing works (headers are fine) but extraction reports the CRC error.
	good := filepath.Join(dir, "good.rar")
	rartest.Write(t, good, []string{"a.bin"}, map[string][]byte{"a.bin": []byte(strings.Repeat("x", 500))})
	raw, _ := os.ReadFile(good)
	i := strings.Index(string(raw), strings.Repeat("x", 500))
	raw[i+10] ^= 0xff
	broken := filepath.Join(dir, "broken.rar")
	os.WriteFile(broken, raw, 0o644)
	dest := filepath.Join(dir, "out", "a.bin")
	if err := tool.Extract(context.Background(), broken, "a.bin", dest); err == nil {
		t.Error("extracting corrupt data should fail")
	}
	if _, err := os.Stat(dest); err == nil {
		t.Error("a failed extraction must leave no file")
	}
	if _, err := os.Stat(dest + ".romcure-tmp"); err == nil {
		t.Error("a failed extraction must leave no temp file")
	}
}

func TestNoToolIsAClearError(t *testing.T) {
	if _, err := (Tool{}).List(context.Background(), "x.rar"); err != ErrNoTool {
		t.Errorf("err = %v", err)
	}
	if err := (Tool{}).Extract(context.Background(), "x.rar", "a", "b"); err != ErrNoTool {
		t.Errorf("err = %v", err)
	}
}

func TestCancelStopsExtraction(t *testing.T) {
	tool := sevenZip(t)
	dir := t.TempDir()
	archive := filepath.Join(dir, "a.rar")
	rartest.Write(t, archive, []string{"a.bin"}, map[string][]byte{"a.bin": []byte("abc")})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := tool.Extract(ctx, archive, "a.bin", filepath.Join(dir, "o", "a.bin")); err != context.Canceled {
		t.Errorf("err = %v, want context.Canceled", err)
	}
}

func TestOpenStreamsAMemberAndReportsDamage(t *testing.T) {
	tool := sevenZip(t)
	dir := t.TempDir()
	good := filepath.Join(dir, "g.rar")
	data := []byte(strings.Repeat("y", 3000))
	rartest.Write(t, good, []string{"a.nes"}, map[string][]byte{"a.nes": data})
	rc, err := tool.Open(context.Background(), good, "a.nes")
	if err != nil {
		t.Fatal(err)
	}
	got, _ := io.ReadAll(rc)
	if err := rc.Close(); err != nil || string(got) != string(data) {
		t.Fatalf("read %d bytes, close err %v", len(got), err)
	}

	raw, _ := os.ReadFile(good)
	raw[strings.Index(string(raw), "yyyy")+5] ^= 0xff
	bad := filepath.Join(dir, "b.rar")
	os.WriteFile(bad, raw, 0o644)
	rc, err = tool.Open(context.Background(), bad, "a.nes")
	if err != nil {
		t.Fatal(err)
	}
	io.ReadAll(rc)
	if err := rc.Close(); err == nil {
		t.Error("a CRC error must come back from Close")
	}
}

func TestOpenClosedEarlyStopsWithoutError(t *testing.T) {
	tool := sevenZip(t)
	archive := filepath.Join(t.TempDir(), "big.rar")
	rartest.Write(t, archive, []string{"disc.iso"}, map[string][]byte{"disc.iso": []byte(strings.Repeat("z", 4<<20))})
	rc, err := tool.Open(context.Background(), archive, "disc.iso")
	if err != nil {
		t.Fatal(err)
	}
	head := make([]byte, 64)
	if _, err := io.ReadFull(rc, head); err != nil || string(head[:4]) != "zzzz" {
		t.Fatalf("head = %q, %v", head[:4], err)
	}
	if err := rc.Close(); err != nil {
		t.Errorf("closing after the start only: %v", err)
	}
}
