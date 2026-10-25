// Package settings holds the user-adjustable options (spec M6: scratch dir,
// parallelism, codec flags, elevation memory), persisted as JSON in the app
// data dir and validated before use.
package settings

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"romcure/internal/fsinfo"
	"romcure/internal/link"
	"romcure/internal/transcode"
)

const fileName = "settings.json"

// Elevation records what to do when a symlink needs Windows elevation
// (spec §2.3): ask each time, or remember that the user said no and fall
// back to copies/other tiers without asking again.
type Elevation string

const (
	ElevationAsk   Elevation = "ask"
	ElevationNever Elevation = "never"
)

// Settings is the persisted configuration.
type Settings struct {
	Version int `json:"version"`
	// ScratchDir receives extracted discs and conversions. Empty = a
	// "scratch" folder in the app data dir.
	ScratchDir string `json:"scratch_dir"`
	// CacheDir holds the converted discs (transcode-cache/) and extracted
	// games (extract-cache/), which can grow to many GB. Empty = the app
	// data dir. Entries already made stay where they are.
	CacheDir string `json:"cache_dir"`
	// Parallelism is the number of concurrent conversions (1-8).
	Parallelism int `json:"parallelism"`
	// CHDCodecsCD / CHDCodecsDVD override the pinned chdman codec lists
	// (comma separated, e.g. "cdlz,cdzl,cdfl"). Empty = the compatible
	// defaults from transcode.CodecsCompat. Part of the transcode cache key.
	CHDCodecsCD   string    `json:"chd_codecs_cd"`
	CHDCodecsDVD  string    `json:"chd_codecs_dvd"`
	Elevation     Elevation `json:"elevation"`
	AllowHardlink bool      `json:"allow_hardlink"`
	// RegionPreset is a rules.Presets id ("usa", "europe", ...).
	RegionPreset string `json:"region_preset"`
	// Onboarded is set once the first-run setup was finished or skipped.
	Onboarded bool `json:"onboarded"`
}

// Default returns the settings of a fresh install.
func Default() Settings {
	return Settings{Version: 1, Parallelism: 1, Elevation: ElevationAsk, RegionPreset: "usa"}
}

// allowedCodecs are the chdman codecs we accept in a user override; the list
// is closed so a settings file can never inject arbitrary command-line text.
var allowedCodecs = map[string]bool{
	"none": true, "zlib": true, "lzma": true, "huff": true, "flac": true, "zstd": true,
	"cdzl": true, "cdlz": true, "cdfl": true, "cdzs": true,
}

func validCodecList(list string) error {
	if list == "" {
		return nil
	}
	parts := strings.Split(list, ",")
	if len(parts) > 4 {
		return errors.New("chdman accepts at most four codecs")
	}
	for _, p := range parts {
		if !allowedCodecs[strings.TrimSpace(p)] {
			return fmt.Errorf("unknown codec %q", p)
		}
	}
	return nil
}

// Validate returns every problem with s (nil = valid). Scratch-dir checks
// touch the filesystem: it must not be inside a registered source root and
// must not be a network location.
func (s Settings) Validate() []error {
	var errs []error
	if s.Parallelism < 1 || s.Parallelism > 8 {
		errs = append(errs, fmt.Errorf("parallelism must be between 1 and 8, got %d", s.Parallelism))
	}
	if err := validCodecList(s.CHDCodecsCD); err != nil {
		errs = append(errs, fmt.Errorf("CD codecs: %w", err))
	}
	if err := validCodecList(s.CHDCodecsDVD); err != nil {
		errs = append(errs, fmt.Errorf("DVD codecs: %w", err))
	}
	if s.Elevation != ElevationAsk && s.Elevation != ElevationNever {
		errs = append(errs, fmt.Errorf("elevation must be %q or %q", ElevationAsk, ElevationNever))
	}
	if s.CacheDir != "" {
		if err := link.GuardWrite(s.CacheDir); err != nil {
			errs = append(errs, fmt.Errorf("cache dir: %w", err))
		} else if err := checkWritableLocal(s.CacheDir); err != nil {
			errs = append(errs, fmt.Errorf("cache dir: %w", err))
		}
	}
	if s.ScratchDir != "" {
		if err := link.GuardWrite(s.ScratchDir); err != nil {
			errs = append(errs, fmt.Errorf("scratch dir: %w", err))
		} else if err := checkWritableLocal(s.ScratchDir); err != nil {
			errs = append(errs, fmt.Errorf("scratch dir: %w", err))
		}
	}
	return errs
}

func checkWritableLocal(dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	if info, err := fsinfo.Detect(dir); err == nil && info.IsRemote {
		return errors.New("network locations are not supported")
	}
	f, err := os.CreateTemp(dir, ".romcure-write-test-*")
	if err != nil {
		return fmt.Errorf("not writable: %w", err)
	}
	f.Close()
	return os.Remove(f.Name())
}

// CacheRoot is where the big caches live: the user's CacheDir, or the default
// (see DefaultCacheRoot).
func (s Settings) CacheRoot(dataDir string) string {
	if s.CacheDir != "" {
		return s.CacheDir
	}
	userCache, _ := os.UserCacheDir()
	return DefaultCacheRoot(dataDir, userCache)
}

// DefaultCacheRoot is where the big caches (converted discs, extracted games,
// scratch space) go when the user chose nothing. They can reach many GB, so a
// new install keeps them in the OS's local cache folder, not in the roaming
// profile that follows the user between PCs (%AppData% on Windows). An
// install that already has caches in the data folder keeps them there:
// moving them would orphan the paths the database recorded.
func DefaultCacheRoot(dataDir, userCacheDir string) string {
	for _, name := range []string{"transcode-cache", "extract-cache"} {
		if st, err := os.Stat(filepath.Join(dataDir, name)); err == nil && st.IsDir() {
			return dataDir
		}
	}
	if userCacheDir == "" {
		return dataDir
	}
	return filepath.Join(userCacheDir, "romcure")
}

// CHDProfile returns the codec profile for a media kind: the user override
// when set, otherwise the compatible default.
func (s Settings) CHDProfile(media transcode.Media) transcode.CodecProfile {
	override := s.CHDCodecsCD
	if media == transcode.MediaDVD {
		override = s.CHDCodecsDVD
	}
	if override == "" {
		return transcode.CodecsCompat[media]
	}
	return transcode.CodecProfile{Name: "custom", Flags: []string{"-c", override}}
}

// Scratch resolves the scratch directory, defaulting under appData.
func (s Settings) Scratch(appData string) string {
	if s.ScratchDir != "" {
		return s.ScratchDir
	}
	return filepath.Join(s.CacheRoot(appData), "scratch")
}

// ErrCorrupt is returned (with Default settings) when the file can't be parsed.
var ErrCorrupt = errors.New("settings file is corrupt")

// Load reads dir/settings.json. A missing file yields the defaults. A corrupt
// one is moved aside to settings.json.bad and the defaults are returned with
// an error wrapping ErrCorrupt, so the app can tell the user instead of
// silently discarding their configuration. Unspecified fields keep defaults.
func Load(dir string) (Settings, error) {
	path := filepath.Join(dir, fileName)
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return Default(), nil
	}
	if err != nil {
		return Default(), err
	}
	s := Default()
	if err := json.Unmarshal(b, &s); err != nil {
		os.Rename(path, path+".bad")
		return Default(), fmt.Errorf("%w (moved to %s.bad): %v", ErrCorrupt, fileName, err)
	}
	return s, nil
}

// Save validates and atomically writes the settings.
func Save(dir string, s Settings) error {
	if errs := s.Validate(); len(errs) > 0 {
		return errors.Join(errs...)
	}
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	path := filepath.Join(dir, fileName)
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
