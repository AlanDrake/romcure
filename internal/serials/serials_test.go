package serials

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const sample = `clrmamepro (
	name "Sony - PlayStation 2"
	version "2026.10.07"
)

game (
	name "Example Kart (Europe) (En,Fr,De,Es,It,Nl)"
	region "Europe"
	serial "SLES-12345"
	rom ( name "Example Kart (Europe) (En,Fr,De,Es,It,Nl).iso" size 1 crc 00000000 serial "SLES-12345" )
)
game (
	name "Two Serials (USA)"
	serial "SLUS-20001, SLUS-20002"
	rom ( name "x.iso" size 1 crc 00000000 )
)
game (
	name "No Serial (Japan)"
	rom ( name "y.iso" size 1 crc 00000000 )
)
`

func TestParse(t *testing.T) {
	l, err := Parse(strings.NewReader(sample))
	if err != nil {
		t.Fatal(err)
	}
	if l.Version != "2026.10.07" {
		t.Errorf("version = %q", l.Version)
	}
	want := []Entry{
		{"SLES-12345", "Example Kart (Europe) (En,Fr,De,Es,It,Nl)"},
		{"SLUS-20001", "Two Serials (USA)"},
		{"SLUS-20002", "Two Serials (USA)"},
	}
	if len(l.Entries) != len(want) {
		t.Fatalf("entries = %+v", l.Entries)
	}
	for i := range want {
		if l.Entries[i] != want[i] {
			t.Errorf("entry %d = %+v, want %+v", i, l.Entries[i], want[i])
		}
	}
}

func TestFetch(t *testing.T) {
	var asked string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		asked = r.URL.Path
		w.Write([]byte(sample))
	}))
	defer srv.Close()
	l, err := Fetch(context.Background(), srv.Client(), srv.URL+"/", "ps2")
	if err != nil || len(l.Entries) != 3 {
		t.Fatalf("%+v %v", l, err)
	}
	if asked != "/Sony - PlayStation 2.dat" {
		t.Errorf("asked for %q", asked)
	}
	if _, err := Fetch(context.Background(), srv.Client(), srv.URL+"/", "nes"); err == nil {
		t.Error("nes has no serial list")
	}
	if !Has("psx") || Has("gc") {
		t.Error("Has wrong")
	}
}
