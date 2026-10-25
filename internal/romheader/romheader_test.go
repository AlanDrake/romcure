package romheader

import (
	"bytes"
	"fmt"
	"hash/crc32"
	"testing"
)

func crc(b []byte) string { return fmt.Sprintf("%08x", crc32.ChecksumIEEE(b)) }

func TestHashSkipsKnownHeaders(t *testing.T) {
	rom := bytes.Repeat([]byte{0xA5, 0x5A, 0x01}, 2000)
	snesRom := bytes.Repeat([]byte{7}, 4096)
	cases := []struct {
		name   string
		header []byte
		body   []byte
	}{
		{"Game.nes", append([]byte("NES\x1a"), make([]byte, 12)...), rom},
		{"Game.FDS", append([]byte("FDS\x1a"), make([]byte, 12)...), rom},
		{"Game.a78", append(append([]byte{1}, []byte("ATARI7800")...), make([]byte, 118)...), rom},
		{"Game.lnx", append([]byte("LYNX\x00"), make([]byte, 59)...), rom},
		{"Game.smc", make([]byte, 512), snesRom},
	}
	for _, c := range cases {
		file := append(append([]byte{}, c.header...), c.body...)
		res, ok, err := Hash(c.name, bytes.NewReader(file), int64(len(file)))
		if err != nil || !ok {
			t.Errorf("%s: ok=%v err=%v", c.name, ok, err)
			continue
		}
		if res.CRC32 != crc(c.body) || res.Size != int64(len(c.body)) {
			t.Errorf("%s: got %+v, want crc %s size %d", c.name, res, crc(c.body), len(c.body))
		}
	}
}

func TestHashLeavesHeaderlessFilesAlone(t *testing.T) {
	for name, data := range map[string][]byte{
		"Game.nes": bytes.Repeat([]byte{1}, 4096), // no magic
		"Game.sfc": bytes.Repeat([]byte{1}, 4096), // size a multiple of 1 KiB
		"Game.gb":  append([]byte("NES\x1a"), make([]byte, 100)...),
		"tiny.nes": []byte("NES\x1a"), // header only
	} {
		if _, ok, err := Hash(name, bytes.NewReader(data), int64(len(data))); ok || err != nil {
			t.Errorf("%s: ok=%v err=%v, want no header", name, ok, err)
		}
	}
	if MayHaveHeader("x.gba") || !MayHaveHeader("X.NES") {
		t.Error("MayHaveHeader is wrong")
	}
}
