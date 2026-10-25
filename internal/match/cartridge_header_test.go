package match

import (
	"context"
	"database/sql"
	"fmt"
	"hash/crc32"
	"os"
	"path/filepath"
	"testing"

	"romcure/internal/scan"
	"romcure/internal/store"
)

func gbaDump(code, title string, size int, fill byte) []byte {
	b := make([]byte, size)
	for i := range b {
		b[i] = fill
	}
	copy(b[0xA0:], title)
	copy(b[0xAC:], code)
	b[0xB2] = 0x96
	var sum int
	for _, c := range b[0xA0:0xBD] {
		sum += int(c)
	}
	b[0xBD] = byte(-sum - 0x19)
	return b
}

func ndsDump(code, title string, size int) []byte {
	b := make([]byte, size)
	copy(b, title)
	copy(b[0x0C:], code)
	crc := uint16(0xFFFF)
	for _, c := range b[:0x15E] {
		crc ^= uint16(c)
		for i := 0; i < 8; i++ {
			if crc&1 != 0 {
				crc = crc>>1 ^ 0xA001
			} else {
				crc >>= 1
			}
		}
	}
	b[0x15E], b[0x15F] = byte(crc), byte(crc>>8)
	return b
}

// n64V64 builds a byte-swapped (.v64) dump.
func n64V64(code, title string, size int) []byte {
	b := make([]byte, size)
	copy(b, []byte{0x80, 0x37, 0x12, 0x40})
	copy(b[0x20:], title)
	copy(b[0x3B:], code)
	for i := 0; i+1 < 0x40; i += 2 {
		b[i], b[i+1] = b[i+1], b[i]
	}
	return b
}

func seedSerial(t *testing.T, db *store.DB, systemID, game, serial string, size int64) {
	t.Helper()
	datID, err := db.InsertDat(store.DatRecord{SystemID: systemID, Format: "logiqx_xml", SourcePath: "/" + game + ".dat"})
	if err != nil {
		t.Fatal(err)
	}
	err = db.InsertDatEntries([]store.DatEntryRecord{{DatID: datID, SystemID: systemID, GameName: game, RomName: game + ".rom",
		Size: size, CRC32: sql.NullString{String: "deadbeef", Valid: true}, Serial: sql.NullString{String: serial, Valid: true}}})
	if err != nil {
		t.Fatal(err)
	}
}

func browseTitles(t *testing.T, db *store.DB, root, system string) map[string]Result {
	t.Helper()
	if err := scan.New(db).Scan(context.Background(), root, nil); err != nil {
		t.Fatal(err)
	}
	items, _, err := Browse(db, root, system)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]Result{}
	for _, it := range items {
		got[filepath.Base(it.Path)] = it.Result
	}
	return got
}

func TestCartridgeDumpsAreNamedFromTheirHeaderCode(t *testing.T) {
	tests := []struct {
		system, file string
		data         []byte
		code, game   string
	}{
		{"gba", "grp-exq.gba", gbaDump("AXQE", "EXQUEST", 0x400, 1), "AXQE", "Example Quest (USA, Europe)"},
		{"nds", "zzz-mk.nds", ndsDump("AMCE", "MARIOKART", 0x400), "AMCE", "Mario Kart DS (USA)"},
		{"n64", "sm64.v64", n64V64("NSME", "SUPER MARIO 64", 0x400), "NSME", "Super Mario 64 (USA)"},
	}
	for _, tt := range tests {
		t.Run(tt.system, func(t *testing.T) {
			root := t.TempDir()
			os.WriteFile(filepath.Join(root, tt.file), tt.data, 0o644)
			db := openTestDB(t)
			seedSerial(t, db, tt.system, tt.game, tt.code, int64(len(tt.data)))
			r := browseTitles(t, db, root, tt.system)[tt.file]
			want := MatchHeuristic(tt.game + ".rom")
			if r.Title != want.Title || r.Region != want.Region || r.Confidence != ConfidenceHeuristic {
				t.Errorf("result = %+v, want title %q region %q heuristic", r, want.Title, want.Region)
			}
		})
	}
}

func TestCartridgeHeaderWithABadChecksumIsIgnored(t *testing.T) {
	root := t.TempDir()
	gba := gbaDump("AXQE", "EXQUEST", 0x400, 1)
	gba[0xBD]++
	nds := ndsDump("AMCE", "MARIOKART", 0x400)
	nds[0x15E]++
	os.WriteFile(filepath.Join(root, "bad.gba"), gba, 0o644)
	os.WriteFile(filepath.Join(root, "bad.nds"), nds, 0o644)
	db := openTestDB(t)
	seedSerial(t, db, "gba", "Example Quest (USA, Europe)", "AXQE", 0x400)
	seedSerial(t, db, "nds", "Mario Kart DS (USA)", "AMCE", 0x400)
	if r := browseTitles(t, db, root, "gba")["bad.gba"]; r.Title != "bad" || r.Confidence != ConfidenceUnidentified {
		t.Errorf("gba: %+v", r)
	}
	if r := browseTitles(t, db, root, "nds")["bad.nds"]; r.Title != "bad" || r.Confidence != ConfidenceUnidentified {
		t.Errorf("nds: %+v", r)
	}
}

// A DAT name replaces scene tags (heuristic), but not a verified identity; a
// code the DAT does not list changes nothing.
func TestCartridgeCodeRenamesHeuristicButNotVerifiedOrUnknown(t *testing.T) {
	root := t.TempDir()
	scene := gbaDump("AXQE", "EXQUEST", 0x400, 1)
	os.WriteFile(filepath.Join(root, "Example Quest (Europe) (Scene Tag).gba"), scene, 0o644)
	os.WriteFile(filepath.Join(root, "unknown.gba"), gbaDump("ZZZZ", "NOPE", 0x400, 2), 0o644)
	verified := gbaDump("AXQE", "EXQUEST", 0x400, 3)
	os.WriteFile(filepath.Join(root, "Verified Name (USA).gba"), verified, 0o644)
	db := openTestDB(t)
	seedSerial(t, db, "gba", "Example Quest (USA, Europe)", "AXQE", 0x400)
	datID, _ := db.InsertDat(store.DatRecord{SystemID: "gba", Format: "logiqx_xml", SourcePath: "/v.dat"})
	db.InsertDatEntries([]store.DatEntryRecord{{DatID: datID, SystemID: "gba", GameName: "Verified Name (USA)", RomName: "v.gba", Size: 0x400,
		CRC32: sql.NullString{String: crcHex(verified), Valid: true}}})
	got := browseTitles(t, db, root, "gba")
	if r := got["Example Quest (Europe) (Scene Tag).gba"]; r.Title != "Example Quest" || r.Region != "USA, Europe" {
		t.Errorf("heuristic: %+v", r)
	}
	if r := got["unknown.gba"]; r.Title != "unknown" || r.Confidence != ConfidenceUnidentified {
		t.Errorf("unknown code: %+v", r)
	}
	if r := got["Verified Name (USA).gba"]; r.Confidence != ConfidenceDATVerified || r.Title != "Verified Name (USA)" {
		t.Errorf("verified: %+v", r)
	}
}

func TestPickSerialName(t *testing.T) {
	e := func(name string, size int64) store.DatSerialEntry {
		return store.DatSerialEntry{GameName: name, Size: size}
	}
	tests := []struct {
		name    string
		entries []store.DatSerialEntry
		size    int64
		want    string
	}{
		{"size first", []store.DatSerialEntry{e("A (USA)", 10), e("B (USA)", 20)}, 20, "B (USA)"},
		{"virtual console loses", []store.DatSerialEntry{e("A (USA) (Virtual Console)", 10), e("B (USA)", 10)}, 10, "B (USA)"},
		{"proto loses", []store.DatSerialEntry{e("A (USA) (Proto)", 10), e("B (USA)", 10)}, 10, "B (USA)"},
		{"revision loses", []store.DatSerialEntry{e("A (USA) (Rev 1)", 10), e("B (USA)", 10)}, 10, "B (USA)"},
		{"size beats tags", []store.DatSerialEntry{e("A (USA)", 10), e("B (USA) (Beta)", 20)}, 20, "B (USA) (Beta)"},
		{"alphabetical", []store.DatSerialEntry{e("B (USA)", 10), e("A (USA)", 10)}, 10, "A (USA)"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := pickSerialName(tt.entries, tt.size); got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func crcHex(b []byte) string { return fmt.Sprintf("%08x", crc32.ChecksumIEEE(b)) }

// A hack or translation carries its base game's header code but is not that game.
func TestCartridgeCodeIgnoresHacksAndTranslations(t *testing.T) {
	for name, renamed := range map[string]bool{
		"Example Quest Remix [h].gba":    false,
		"Game [h1].gba":                  false,
		"Game [h2C].gba":                 false,
		"Game [hI].gba":                  false,
		"Game [h Somebody].gba":          false,
		"Game [T+Ita].gba":               false,
		"Game [T-Eng].gba":               false,
		"Game [tr Fre].gba":              false,
		"Game (Hack).gba":                false,
		"Game (Translation Italian).gba": false,
		"Game (HACK).gba":                false,
		"grp-exq.gba":                    true,
		"Example Quest [t +2 Group].gba": true, // a trainer tag is not a hack tag
	} {
		root := t.TempDir()
		os.WriteFile(filepath.Join(root, name), gbaDump("AXQE", "EXQUEST", 0x400, 1), 0o644)
		db := openTestDB(t)
		seedSerial(t, db, "gba", "Example Quest (USA, Europe)", "AXQE", 0x400)
		r := browseTitles(t, db, root, "gba")[name]
		if got := r.Title == "Example Quest"; got != renamed {
			t.Errorf("%s: title %q, renamed to the DAT game = %v, want %v", name, r.Title, got, renamed)
		}
	}
}
