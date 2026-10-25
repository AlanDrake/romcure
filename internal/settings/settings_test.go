package settings

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"romcure/internal/link"
	"romcure/internal/transcode"
)

func TestDefaultsAreValid(t *testing.T) {
	if errs := Default().Validate(); len(errs) != 0 {
		t.Fatalf("defaults invalid: %v", errs)
	}
}

func TestSaveLoadRoundTripAndMissingFile(t *testing.T) {
	dir := t.TempDir()
	s, err := Load(dir)
	if err != nil || s.Parallelism != 1 || s.Elevation != ElevationAsk {
		t.Fatalf("missing file: %+v err=%v", s, err)
	}
	s.Parallelism, s.Elevation, s.RegionPreset, s.AllowHardlink = 3, ElevationNever, "europe", true
	s.ScratchDir = filepath.Join(dir, "scr")
	if err := Save(dir, s); err != nil {
		t.Fatal(err)
	}
	got, err := Load(dir)
	if err != nil || got != s {
		t.Fatalf("round trip: %+v vs %+v err=%v", got, s, err)
	}
}

func TestLoadKeepsDefaultsForMissingFields(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, fileName), []byte(`{"parallelism": 4}`), 0o644)
	s, err := Load(dir)
	if err != nil || s.Parallelism != 4 || s.Elevation != ElevationAsk || s.RegionPreset != "usa" {
		t.Fatalf("%+v err=%v", s, err)
	}
}

func TestCorruptFileIsMovedAsideNotSilentlyLost(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, fileName), []byte("{not json"), 0o644)
	s, err := Load(dir)
	if !errors.Is(err, ErrCorrupt) || s.Parallelism != 1 {
		t.Fatalf("%+v err=%v", s, err)
	}
	if _, e := os.Stat(filepath.Join(dir, fileName+".bad")); e != nil {
		t.Error("corrupt file was not preserved")
	}
}

func TestValidation(t *testing.T) {
	bad := Default()
	bad.Parallelism = 0
	bad.CHDCodecsCD = "cdlz,evil;rm -rf"
	bad.CHDCodecsDVD = "lzma,zlib,huff,flac,zstd"
	bad.Elevation = "sometimes"
	errs := bad.Validate()
	if len(errs) != 4 {
		t.Errorf("errors = %v, want 4", errs)
	}
	if err := Save(t.TempDir(), bad); err == nil {
		t.Error("Save must refuse invalid settings")
	}
}

func TestScratchMustNotBeInsideASourceRoot(t *testing.T) {
	root := t.TempDir()
	if err := link.RegisterSourceRoot(root); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(link.ClearSourceRoots)
	s := Default()
	s.ScratchDir = filepath.Join(root, "scratch")
	errs := s.Validate()
	if len(errs) != 1 || !strings.Contains(errs[0].Error(), "source root") {
		t.Errorf("errors = %v", errs)
	}
	s.ScratchDir = filepath.Join(t.TempDir(), "ok")
	if errs := s.Validate(); len(errs) != 0 {
		t.Errorf("valid scratch rejected: %v", errs)
	}
}

func TestCHDProfileOverride(t *testing.T) {
	s := Default()
	if p := s.CHDProfile(transcode.MediaCD); p.Name != "compat" {
		t.Errorf("default = %+v", p)
	}
	s.CHDCodecsDVD = "zstd"
	p := s.CHDProfile(transcode.MediaDVD)
	if p.Name != "custom" || len(p.Flags) != 2 || p.Flags[1] != "zstd" {
		t.Errorf("override = %+v", p)
	}
	// The default scratch space sits beside the caches (see DefaultCacheRoot).
	if got, want := s.Scratch("/app"), filepath.Join(s.CacheRoot("/app"), "scratch"); got != want {
		t.Errorf("scratch = %s, want %s", got, want)
	}
	s.ScratchDir = "/mine"
	if got := s.Scratch("/app"); got != "/mine" {
		t.Errorf("a chosen scratch dir wins, got %s", got)
	}
}

func TestCacheDirMustNotBeInsideASourceRootAndDefaultsToTheDataDir(t *testing.T) {
	root := t.TempDir()
	if err := link.RegisterSourceRoot(root); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(link.ClearSourceRoots)
	s := Default()
	s.CacheDir = filepath.Join(root, "cache")
	errs := s.Validate()
	if len(errs) != 1 || !strings.Contains(errs[0].Error(), "cache dir") || !strings.Contains(errs[0].Error(), "source root") {
		t.Errorf("errors = %v", errs)
	}
	ok := filepath.Join(t.TempDir(), "cache")
	s.CacheDir = ok
	if errs := s.Validate(); len(errs) != 0 {
		t.Errorf("valid cache dir rejected: %v", errs)
	}
	if got := s.CacheRoot("data"); got != ok {
		t.Errorf("CacheRoot = %q, want %q", got, ok)
	}
	s.CacheDir = ""
	userCache, _ := os.UserCacheDir()
	if got, want := s.CacheRoot("data"), DefaultCacheRoot("data", userCache); got != want {
		t.Errorf("default CacheRoot = %q, want %q", got, want)
	}
}

func TestDefaultCacheRootKeepsExistingCachesAndUsesTheLocalCacheForNewInstalls(t *testing.T) {
	local := filepath.Join(t.TempDir(), "LocalCache")

	fresh := t.TempDir()
	if got := DefaultCacheRoot(fresh, local); got != filepath.Join(local, "romcure") {
		t.Errorf("a new install should use the local cache folder, got %q", got)
	}
	if got := DefaultCacheRoot(fresh, ""); got != fresh {
		t.Errorf("without an OS cache folder fall back to the data dir, got %q", got)
	}

	existing := t.TempDir()
	if err := os.MkdirAll(filepath.Join(existing, "transcode-cache"), 0o755); err != nil {
		t.Fatal(err)
	}
	if got := DefaultCacheRoot(existing, local); got != existing {
		t.Errorf("an install that already has caches keeps them, got %q", got)
	}

}
