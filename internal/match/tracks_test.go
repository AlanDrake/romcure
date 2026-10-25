package match

import (
	"path/filepath"
	"testing"

	"romcure/internal/dat"
	"romcure/internal/store"
)

func tracksDB(t *testing.T) *store.DB {
	db, err := store.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	df := &dat.Datafile{Format: dat.FormatLogiqxXML, Games: []dat.Game{
		{Name: "Two Track (USA)", Roms: []dat.Rom{
			{Name: "Two Track (USA).cue", Size: 100, CRC32: "00000001"},
			{Name: "Two Track (USA) (Track 1).bin", Size: 1000, CRC32: "aaaaaaaa", SHA1: "1111111111111111111111111111111111111111"},
			{Name: "Two Track (USA) (Track 2).bin", Size: 500, CRC32: "bbbbbbbb"},
		}},
		{Name: "Other (USA)", Roms: []dat.Rom{
			{Name: "Other (USA).bin", Size: 1000, CRC32: "aaaaaaaa", SHA1: "1111111111111111111111111111111111111111"},
		}},
	}}
	if _, err := dat.Import(db, df, "psx", "/x.dat"); err != nil {
		t.Fatal(err)
	}
	return db
}

func TestMatchTracksRequiresEveryTrack(t *testing.T) {
	db := tracksDB(t)
	full := []store.CHDTrack{
		{Ordinal: 1, Size: 1000, CRC32: "aaaaaaaa", SHA1: "1111111111111111111111111111111111111111"},
		{Ordinal: 2, Size: 500, CRC32: "bbbbbbbb"},
	}
	res, ok, err := MatchTracks(db, "psx", full)
	if err != nil || !ok || res.Title != "Two Track (USA)" || res.Confidence != ConfidenceDATVerified {
		t.Fatalf("full: %+v ok=%v err=%v", res, ok, err)
	}

	// Track 1 alone also equals the single-track "Other (USA)" disc, which
	// is the right answer for a one-track image...
	res, ok, _ = MatchTracks(db, "psx", full[:1])
	if !ok || res.Title != "Other (USA)" {
		t.Errorf("single track should match the single-track game, got %+v ok=%v", res, ok)
	}
	// ...but a track set with a wrong second track matches nothing.
	bad := []store.CHDTrack{full[0], {Ordinal: 2, Size: 500, CRC32: "cccccccc"}}
	if _, ok, _ := MatchTracks(db, "psx", bad); ok {
		t.Error("a mismatching track must not verify")
	}
	// A wrong SHA1 on a CRC+size hit is rejected.
	wrong := []store.CHDTrack{{Ordinal: 1, Size: 1000, CRC32: "aaaaaaaa", SHA1: "2222222222222222222222222222222222222222"}}
	if _, ok, _ := MatchTracks(db, "psx", wrong); ok {
		t.Error("sha1 mismatch must not verify")
	}
	if _, ok, _ := MatchTracks(db, "psx", nil); ok {
		t.Error("no tracks must not verify")
	}
}
