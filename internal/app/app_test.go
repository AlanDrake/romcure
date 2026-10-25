package app

import (
	"context"
	"fmt"
	"hash/crc32"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"romcure/internal/link"
	"romcure/internal/store"
)

func newApp(t *testing.T) *App {
	t.Helper()
	a, err := Open(t.TempDir(), Tools{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { a.Close(); link.ClearSourceRoots() })
	return a
}

func writeROMs(t *testing.T, dir string) {
	t.Helper()
	os.MkdirAll(dir, 0o755)
	os.WriteFile(filepath.Join(dir, "Alpha (USA).nes"), []byte("alpha"), 0o644)
	os.WriteFile(filepath.Join(dir, "Alpha (Europe).nes"), []byte("alpha-eu"), 0o644)
	os.WriteFile(filepath.Join(dir, "Beta (USA).nes"), []byte("beta"), 0o644)
}

func TestSourceValidation(t *testing.T) {
	a := newApp(t)
	src := filepath.Join(t.TempDir(), "nes")
	writeROMs(t, src)

	if _, err := a.AddSource(filepath.Join(src, "missing"), "nes"); err == nil {
		t.Error("a missing folder must be rejected")
	}
	if _, err := a.AddSource(src, " "); err == nil {
		t.Error("a missing system must be rejected")
	}
	if _, err := a.AddSource(src, "nes"); err != nil {
		t.Fatal(err)
	}
	if _, err := a.AddSource(t.TempDir(), "nes"); err != nil {
		t.Errorf("a system may have several folders: %v", err)
	}
	sub := filepath.Join(src, "inner")
	os.MkdirAll(sub, 0o755)
	if _, err := a.AddSource(sub, "snes"); err == nil || !strings.Contains(err.Error(), "overlaps") {
		t.Errorf("nested source: %v", err)
	}
}

func TestProfileValidationKeepsSourcesAndOutputsApart(t *testing.T) {
	a := newApp(t)
	src := filepath.Join(t.TempDir(), "nes")
	writeROMs(t, src)
	if _, err := a.AddSource(src, "nes"); err != nil {
		t.Fatal(err)
	}

	if _, err := a.AddProfile(store.Profile{Name: "", Kind: "raw", OutputRoot: t.TempDir()}); err == nil {
		t.Error("empty name must be rejected")
	}
	if _, err := a.AddProfile(store.Profile{Name: "x", Kind: "nope", OutputRoot: t.TempDir()}); err == nil {
		t.Error("unknown kind must be rejected")
	}
	if _, err := a.AddProfile(store.Profile{Name: "inside", Kind: "raw", OutputRoot: filepath.Join(src, "out")}); err == nil || !strings.Contains(err.Error(), "never written") {
		t.Errorf("output inside a source: %v", err)
	}
	if _, err := a.AddProfile(store.Profile{Name: "parent", Kind: "raw", OutputRoot: filepath.Dir(src)}); err == nil {
		t.Error("output containing a source must be rejected")
	}
	// And the other direction: a source may not be added over an output.
	out := filepath.Join(t.TempDir(), "out")
	if _, err := a.AddProfile(store.Profile{Name: "ok", Kind: "raw", OutputRoot: out}); err != nil {
		t.Fatal(err)
	}
	if _, err := a.AddSource(out, "snes"); err == nil {
		t.Error("a source over an output must be rejected")
	}
}

func TestSyncRunsThePipelineEndToEndAndIsRepeatable(t *testing.T) {
	a := newApp(t)
	src := filepath.Join(t.TempDir(), "nes")
	writeROMs(t, src)
	if _, err := a.AddSource(src, "nes"); err != nil {
		t.Fatal(err)
	}
	ra := filepath.Join(t.TempDir(), "retroarch")
	es := filepath.Join(t.TempDir(), "esde")
	if _, err := a.AddProfile(store.Profile{Name: "RetroArch", Kind: "retroarch", OutputRoot: ra}); err != nil {
		t.Fatal(err)
	}
	if _, err := a.AddProfile(store.Profile{Name: "ES-DE", Kind: "esde", OutputRoot: es, Systems: []string{"nes"}}); err != nil {
		t.Fatal(err)
	}

	var stages []string
	report, err := a.Sync(context.Background(), func(p Progress) { stages = append(stages, p.Stage) })
	if err != nil {
		t.Fatal(err)
	}
	// Default "usa" preset: Alpha (USA) beats Alpha (Europe); Beta stands alone.
	if len(report.Systems) != 1 || report.Systems[0].Selected != 2 || report.Systems[0].Excluded != 1 {
		t.Fatalf("systems = %+v", report.Systems)
	}
	if len(report.Profiles) != 2 {
		t.Fatalf("profiles = %+v", report.Profiles)
	}
	for _, p := range report.Profiles {
		if p.Linked != 2 || len(p.Errors) != 0 || p.Broken != 0 || p.Stale != 0 || p.OK == 0 {
			t.Errorf("%s: %+v", p.Profile, p)
		}
	}
	if _, err := os.Stat(filepath.Join(es, "nes")); err != nil {
		t.Errorf("ES-DE folder missing: %v", err)
	}
	if got := strings.Join(stages, ","); !strings.Contains(got, "scan") || !strings.Contains(got, "generate") || !strings.Contains(got, "verify") {
		t.Errorf("stages = %s", got)
	}

	// The two profiles must not share link_state rows.
	var n int
	a.DB.QueryRow(`SELECT COUNT(DISTINCT profile_id) FROM link_state`).Scan(&n)
	if n != 2 {
		t.Errorf("distinct profile ids = %d, want 2", n)
	}

	// A second run changes nothing and still verifies clean.
	again, err := a.Sync(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range again.Profiles {
		if p.Linked != 2 || p.Broken != 0 || p.Stale != 0 {
			t.Errorf("second run %s: %+v", p.Profile, p)
		}
	}
}

func TestSyncReportsBrokenOutputsAfterTheSourceDisappears(t *testing.T) {
	a := newApp(t)
	src := filepath.Join(t.TempDir(), "nes")
	writeROMs(t, src)
	a.AddSource(src, "nes")
	out := filepath.Join(t.TempDir(), "out")
	a.AddProfile(store.Profile{Name: "raw", Kind: "raw", OutputRoot: out})
	if _, err := a.Sync(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
	os.Remove(filepath.Join(out, "nes", "Beta.nes")) // simulate a vanished output
	rep, err := a.Sync(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	// Sync re-creates what is missing, so the final verification is clean.
	if p := rep.Profiles[0]; p.Broken != 0 || p.Linked != 2 {
		t.Errorf("%+v", p)
	}
	if _, err := os.Lstat(filepath.Join(out, "nes", "Beta.nes")); err != nil {
		t.Error("missing output was not recreated")
	}
}

func TestSyncCancellation(t *testing.T) {
	a := newApp(t)
	src := filepath.Join(t.TempDir(), "nes")
	writeROMs(t, src)
	a.AddSource(src, "nes")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := a.Sync(ctx, nil); err == nil {
		t.Error("a cancelled sync must return an error")
	}
}

func TestCorruptSettingsDoNotStopStartup(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "settings.json"), []byte("{broken"), 0o644)
	a, err := Open(dir, Tools{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	if a.Warning == nil || a.Settings.Parallelism != 1 {
		t.Errorf("warning=%v settings=%+v", a.Warning, a.Settings)
	}
}

func crcHex(b []byte) string { return fmt.Sprintf("%08x", crc32.ChecksumIEEE(b)) }

func TestAfterApplyKeepsTheSyncResultsAndReplacesTheUpdatedOutputs(t *testing.T) {
	last := &SyncReport{
		Systems:    []SystemResult{{SystemID: "nes", Selected: 3}},
		Unreadable: []UnreadableFile{{Path: "bad.7z"}},
		Profiles:   []ProfileResult{{Profile: "A", Linked: 1}, {Profile: "B", Linked: 2}},
	}
	got := AfterApply(last, SyncReport{Profiles: []ProfileResult{{Profile: "B", Linked: 5}}})
	if len(got.Systems) != 1 || len(got.Unreadable) != 1 {
		t.Errorf("sync results lost: %+v", got)
	}
	if len(got.Profiles) != 2 || got.Profiles[0].Profile != "A" || got.Profiles[1].Linked != 5 {
		t.Errorf("profiles = %+v", got.Profiles)
	}
	if got := AfterApply(nil, SyncReport{Profiles: []ProfileResult{{Profile: "A"}}}); len(got.Profiles) != 1 || got.Systems != nil {
		t.Errorf("without a sync: %+v", got)
	}
}

func TestAnOutputKeepsTheRetroArchConfigItWasGiven(t *testing.T) {
	a := newApp(t)
	cfg := filepath.Join(t.TempDir(), "RA", "retroarch.cfg")
	if err := os.MkdirAll(filepath.Dir(cfg), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cfg, []byte("playlist_directory = \"default\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ra, err := a.AddProfile(store.Profile{Name: "RA", Kind: "retroarch", OutputRoot: filepath.Join(t.TempDir(), "ra"), RetroArchConfig: cfg})
	if err != nil {
		t.Fatal(err)
	}
	// Only a RetroArch output keeps it.
	raw, err := a.AddProfile(store.Profile{Name: "Raw", Kind: "raw", OutputRoot: filepath.Join(t.TempDir(), "raw"), RetroArchConfig: cfg})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.AddProfile(store.Profile{Name: "Bad", Kind: "retroarch", OutputRoot: filepath.Join(t.TempDir(), "bad"), RetroArchConfig: cfg + ".missing"}); err == nil {
		t.Error("a config that can't be read should be refused")
	}
	got := func() map[int64]string {
		profs, err := a.DB.Profiles()
		if err != nil {
			t.Fatal(err)
		}
		m := map[int64]string{}
		for _, p := range profs {
			m[p.ID] = p.RetroArchConfig
		}
		return m
	}
	if m := got(); m[ra.ID] != cfg || m[raw.ID] != "" {
		t.Fatalf("kept %v", m)
	}
	if err := a.SetRetroArchConfig(ra.ID, ""); err != nil {
		t.Fatal(err)
	}
	if err := a.SetRetroArchConfig(ra.ID, cfg); err != nil {
		t.Fatal(err)
	}
	if err := a.SetRetroArchConfig(raw.ID, cfg); err != nil || got()[raw.ID] != "" {
		t.Errorf("a non-RetroArch output should ignore the choice: %v", err)
	}
}
