package discid

import (
	"bytes"
	"encoding/binary"
	"testing"
)

// header builds a 0x80-byte disc header.
func header(id, title string, wii bool) []byte {
	h := make([]byte, 0x80)
	copy(h, id)
	if wii {
		binary.BigEndian.PutUint32(h[0x18:], wiiMagic)
	} else {
		binary.BigEndian.PutUint32(h[0x1C:], gcMagic)
	}
	copy(h[0x20:], title)
	return h
}

func at(off int, h []byte, prefix string) []byte {
	b := make([]byte, off+len(h)+64)
	copy(b, prefix)
	copy(b[off:], h)
	return b
}

func TestReadFindsTheHeaderInEveryContainer(t *testing.T) {
	gc := header("GEXE01", "Example Adventure", false)
	wii := header("RMCP01", "Mario Kart Wii", true)
	cases := map[string]struct {
		data []byte
		want Info
	}{
		"plain GC":  {at(0, gc, ""), Info{"GEXE01", "Example Adventure", 1}},
		"plain Wii": {at(0, wii, ""), Info{"RMCP01", "Mario Kart Wii", 1}},
		"WBFS":      {at(0x200, wii, "WBFS"), Info{"RMCP01", "Mario Kart Wii", 1}},
		"RVZ":       {at(0x58, gc, "RVZ\x01"), Info{"GEXE01", "Example Adventure", 1}},
		"WIA":       {at(0x58, wii, "WIA\x01"), Info{"RMCP01", "Mario Kart Wii", 1}},
		"CISO":      {at(0x8000, gc, "CISO"), Info{"GEXE01", "Example Adventure", 1}},
	}
	for name, c := range cases {
		got, ok := Read(bytes.NewReader(c.data))
		if !ok || got != c.want {
			t.Errorf("%s: got %+v %v, want %+v", name, got, ok, c.want)
		}
	}
}

func TestReadRejectsOtherDiscs(t *testing.T) {
	psx := make([]byte, 0x9000)
	copy(psx[0x8001:], "CD001") // an ISO 9660 image (PlayStation, PC...)
	for name, data := range map[string][]byte{
		"ISO 9660":  psx,
		"too short": []byte("GEXE01"),
		"bad id":    at(0, header("gz2e01", "x", false), ""),
	} {
		if got, ok := Read(bytes.NewReader(data)); ok {
			t.Errorf("%s: got %+v", name, got)
		}
	}
}

func TestNameAndRegion(t *testing.T) {
	if n := (Info{ID: "GEXE01", Title: "Adventure"}).Name(); n != "Adventure (USA)" {
		t.Errorf("name = %q", n)
	}
	if n := (Info{ID: "RMCP01", Title: "Mario Kart Wii"}).Name(); n != "Mario Kart Wii (Europe)" {
		t.Errorf("name = %q", n)
	}
	if n := (Info{ID: "GZ2J01"}).Name(); n != "" {
		t.Errorf("no title must give no name: %q", n)
	}
	// A Shift-JIS title is dropped, the ID is kept.
	jp := header("GZ2J01", "\x83\x5b\x83\x8b\x83\x5f", false)
	if got, ok := Read(bytes.NewReader(at(0, jp, ""))); !ok || got.Title != "" || got.ID != "GZ2J01" {
		t.Errorf("Shift-JIS title: %+v %v", got, ok)
	}
	if !MayHave("x.RVZ") || MayHave("x.chd") {
		t.Error("MayHave wrong")
	}
}

func TestDiscNumber(t *testing.T) {
	h := header("GKBEAF", "Baten Kaitos", false)
	h[6] = 1 // the second disc
	got, ok := Read(bytes.NewReader(at(0, h, "")))
	if !ok || got.Disc != 2 || got.DiscTag() != " (Disc 2)" {
		t.Errorf("disc 2: %+v %q", got, got.DiscTag())
	}
	if tag := (Info{Disc: 1}).DiscTag(); tag != "" {
		t.Errorf("the first disc has no tag: %q", tag)
	}
}
