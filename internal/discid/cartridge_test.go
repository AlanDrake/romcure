package discid

import (
	"bytes"
	"testing"
)

func TestReadCartridgeN64ByteOrders(t *testing.T) {
	z64 := make([]byte, 0x100)
	copy(z64, []byte{0x80, 0x37, 0x12, 0x40})
	copy(z64[0x20:], "SUPER MARIO 64")
	copy(z64[0x3B:], "NSME")
	v64 := append([]byte(nil), z64...)
	for i := 0; i < 0x40; i += 2 {
		v64[i], v64[i+1] = v64[i+1], v64[i]
	}
	n64 := append([]byte(nil), z64...)
	for i := 0; i < 0x40; i += 4 {
		n64[i], n64[i+1], n64[i+2], n64[i+3] = n64[i+3], n64[i+2], n64[i+1], n64[i]
	}
	for name, data := range map[string][]byte{"a.z64": z64, "a.v64": v64, "a.n64": n64} {
		info, ok := ReadCartridge(name, bytes.NewReader(data))
		if !ok || info.ID != "NSME" || info.Title != "SUPER MARIO 64" {
			t.Errorf("%s: %+v %v", name, info, ok)
		}
	}
	if _, ok := ReadCartridge("a.z64", bytes.NewReader(make([]byte, 0x100))); ok {
		t.Error("a file with no magic word was accepted")
	}
	if _, ok := ReadCartridge("a.bin", bytes.NewReader(z64)); ok {
		t.Error("an unrelated extension was read")
	}
}
