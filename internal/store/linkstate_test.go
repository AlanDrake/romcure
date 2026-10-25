package store

import "testing"

func TestUpsertAndGetLinkState(t *testing.T) {
	db := openTestDB(t)

	rec := LinkStateRecord{
		ProfileID: "retroarch", SystemID: "nes", GameKey: "abc123",
		TargetPath: "nes/Game (USA).nes", SourceKind: "raw_file",
		SourcePath: "/src/Game (USA).nes", SourceHash: "deadbeef",
		LinkType: "symlink_rel", LinkStatus: "ok",
	}
	if err := db.UpsertLinkState(rec); err != nil {
		t.Fatalf("UpsertLinkState: %v", err)
	}

	got, err := db.GetLinkState("retroarch", "nes/Game (USA).nes")
	if err != nil {
		t.Fatalf("GetLinkState: %v", err)
	}
	if got == nil {
		t.Fatal("GetLinkState returned nil")
	}
	if got.SourcePath != rec.SourcePath || got.LinkType != rec.LinkType {
		t.Errorf("got = %+v, want matching SourcePath/LinkType from %+v", got, rec)
	}

	// Upsert again with a different link type — must update in place
	// (unique on profile_id, target_path), not duplicate.
	rec.LinkType = "copy"
	if err := db.UpsertLinkState(rec); err != nil {
		t.Fatalf("second UpsertLinkState: %v", err)
	}
	got, err = db.GetLinkState("retroarch", "nes/Game (USA).nes")
	if err != nil {
		t.Fatalf("GetLinkState after update: %v", err)
	}
	if got.LinkType != "copy" {
		t.Errorf("LinkType = %q after update, want %q", got.LinkType, "copy")
	}

	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM link_state`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Errorf("link_state row count = %d, want 1 (upsert, not duplicate)", count)
	}
}

func TestGetLinkStateMissing(t *testing.T) {
	db := openTestDB(t)
	got, err := db.GetLinkState("retroarch", "nes/nonexistent.nes")
	if err != nil {
		t.Fatalf("GetLinkState: %v", err)
	}
	if got != nil {
		t.Errorf("GetLinkState = %+v, want nil", got)
	}
}
