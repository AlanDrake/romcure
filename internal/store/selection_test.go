package store

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"testing"
)

// An output created before per-output selection that listed systems must keep
// taking exactly those systems: default exclude plus one ticked row each.
func TestMigrationTurnsListedSystemsIntoTicks(t *testing.T) {
	path := filepath.Join(t.TempDir(), "old.db")
	raw, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer raw.Close()
	raw.SetMaxOpenConns(1)
	raw.Exec(`PRAGMA foreign_keys = ON`)
	// Apply the migrations that existed before this feature, by hand.
	if _, err := raw.Exec(`CREATE TABLE schema_migrations (version INTEGER PRIMARY KEY, applied_at TEXT NOT NULL DEFAULT '')`); err != nil {
		t.Fatal(err)
	}
	for v := 1; v <= 7; v++ {
		matches, _ := migrationFiles.ReadDir("migrations")
		for _, m := range matches {
			var n int
			if _, err := fmt.Sscanf(m.Name(), "%04d_", &n); err != nil || n != v {
				continue
			}
			body, _ := migrationFiles.ReadFile("migrations/" + m.Name())
			if _, err := raw.Exec(string(body)); err != nil {
				t.Fatalf("migration %s: %v", m.Name(), err)
			}
			raw.Exec(`INSERT INTO schema_migrations (version) VALUES (?)`, v)
		}
	}
	raw.Exec(`INSERT INTO profiles (name, kind, output_root, systems) VALUES ('Some', 'raw', 'x', 'nes,snes'), ('All', 'raw', 'y', '')`)
	if err := migrate(raw); err != nil {
		t.Fatal(err)
	}

	db := &DB{raw}
	profs, _ := db.Profiles()
	byName := map[string]Profile{}
	for _, p := range profs {
		byName[p.Name] = p
	}
	if !byName["Some"].StartEmpty || byName["All"].StartEmpty {
		t.Errorf("defaults: Some startEmpty=%v, All startEmpty=%v", byName["Some"].StartEmpty, byName["All"].StartEmpty)
	}
	in, err := db.Inclusion(byName["Some"].ID)
	if err != nil {
		t.Fatal(err)
	}
	if !in.Includes("nes", 1) || !in.Includes("snes", 1) || in.Includes("gba", 1) {
		t.Errorf("listed systems not kept: %+v", in.Systems)
	}
	in, _ = db.Inclusion(byName["All"].ID)
	if !in.Includes("gba", 1) {
		t.Error("an output with no list takes every system")
	}
}

func TestInclusionPrecedence(t *testing.T) {
	db := openTestDB(t)
	p, err := db.AddProfile(Profile{Name: "SD", Kind: "device", OutputRoot: "x", StartEmpty: true})
	if err != nil {
		t.Fatal(err)
	}
	gid, err := db.UpsertGameGroup(GameGroupRecord{SystemID: "nes", GroupKey: "g", GroupingSource: "none", DisplayName: "G"})
	if err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"k1", "k2"} {
		if _, err := db.UpsertGame(GameRecord{GroupID: gid, SystemID: "nes", GameKey: k, Confidence: "heuristic",
			UltimateSourceKind: "raw_file", UltimateSourceRef: "/r/" + k}); err != nil {
			t.Fatal(err)
		}
	}
	check := func(want bool, why string) {
		t.Helper()
		in, err := db.Inclusion(p.ID)
		if err != nil {
			t.Fatal(err)
		}
		if got := in.Includes("nes", gid); got != want {
			t.Errorf("%s: included=%v, want %v", why, got, want)
		}
	}
	check(false, "output default")
	db.SetSystemIncluded(p.ID, "nes", true)
	check(true, "system tick beats default")
	db.SetGameIncluded(p.ID, gid, false)
	check(false, "game tick beats system tick")
	db.SetGameIncluded(p.ID, gid, true)
	check(true, "game ticked back to its inherited state")
	var rows int
	db.QueryRow(`SELECT COUNT(*) FROM output_games`).Scan(&rows)
	if rows != 0 {
		t.Errorf("%d exception rows kept for a game matching its system", rows)
	}
	db.SetGameIncluded(p.ID, gid, false)
	db.SetSystemIncluded(p.ID, "nes", false)
	db.QueryRow(`SELECT COUNT(*) FROM output_games`).Scan(&rows)
	if rows != 0 {
		t.Errorf("setting a system must clear the game ticks inside it, %d left", rows)
	}
	check(false, "system unticked")

	// Deleting the output drops its ticks (foreign keys cascade).
	db.SetGameIncluded(p.ID, gid, true)
	db.RemoveProfile(p.ID)
	db.QueryRow(`SELECT COUNT(*) FROM output_games`).Scan(&rows)
	if rows != 0 {
		t.Errorf("%d ticks left after removing the output", rows)
	}
}

// Migration 0009 rebuilds link_state: existing rows survive, and the new
// source kinds are accepted afterwards.
func TestLinkStateRebuildKeepsRowsAndAcceptsNewKinds(t *testing.T) {
	raw, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "old.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer raw.Close()
	raw.SetMaxOpenConns(1)
	if _, err := raw.Exec(`CREATE TABLE schema_migrations (version INTEGER PRIMARY KEY, applied_at TEXT NOT NULL DEFAULT '')`); err != nil {
		t.Fatal(err)
	}
	entries, _ := migrationFiles.ReadDir("migrations")
	for _, m := range entries {
		var n int
		if _, err := fmt.Sscanf(m.Name(), "%04d_", &n); err != nil || n > 8 {
			continue
		}
		body, _ := migrationFiles.ReadFile("migrations/" + m.Name())
		if _, err := raw.Exec(string(body)); err != nil {
			t.Fatalf("migration %s: %v", m.Name(), err)
		}
		raw.Exec(`INSERT INTO schema_migrations (version) VALUES (?)`, n)
	}
	if _, err := raw.Exec(`INSERT INTO link_state (profile_id, system_id, game_key, target_path, source_kind, source_path, source_hash, link_type, link_status)
		VALUES ('p1', 'nes', 'k', 'nes/Game.nes', 'raw_file', '/src/Game.nes', 'abcd', 'symlink_abs', 'ok')`); err != nil {
		t.Fatal(err)
	}
	if err := migrate(raw); err != nil {
		t.Fatal(err)
	}
	db := &DB{raw}
	rec, err := db.GetLinkState("p1", "nes/Game.nes")
	if err != nil || rec == nil || rec.SourcePath != "/src/Game.nes" || rec.SourceHash != "abcd" {
		t.Fatalf("existing row lost or changed: %+v %v", rec, err)
	}
	for _, kind := range []string{"generated", "sbi"} {
		if err := db.UpsertLinkState(LinkStateRecord{ProfileID: "p1", SystemID: "nes", GameKey: "g", TargetPath: "x/" + kind,
			SourceKind: kind, LinkType: "copy", LinkStatus: "ok"}); err != nil {
			t.Errorf("source kind %q rejected: %v", kind, err)
		}
	}
	if err := db.UpsertLinkState(LinkStateRecord{ProfileID: "p1", SystemID: "nes", GameKey: "g", TargetPath: "x/bad",
		SourceKind: "nonsense", LinkType: "copy", LinkStatus: "ok"}); err == nil {
		t.Error("an unknown source kind must still be rejected")
	}
}
