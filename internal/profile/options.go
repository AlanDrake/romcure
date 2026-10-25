package profile

import (
	"context"

	"romcure/internal/store"
)

// Options and results shared by every output generator.

// Options configures how an output is generated, for every kind of output
// (RetroArch, the ES-DE family, plain folders, device exports).
type Options struct {
	// OutputRoot is the profile's roms root directory (what RetroArch's
	// "File Browser" / content directory points at).
	OutputRoot string
	// ExtractCacheDir holds ROMs extracted out of multi-file zips and all
	// 7z archives (spec §2.8's Extract policy — RetroArch generally can't
	// passthrough either). Required whenever a selected game needs it;
	// an empty value only works if nothing in this sync does.
	ExtractCacheDir string
	// AllowHardlink opts into the hardlink tier (spec §2.2); see
	// link.Preferences.
	AllowHardlink bool
	// Transcode enables CHD conversion of archived discs (spec §2.10).
	// Nil means disc archives are reported as errors, not silently skipped.
	Transcode *TranscodeOptions
	// SBIDir is where downloaded Redump .sbi subchannel files live (named
	// after the DAT game). Empty disables SBI linking.
	SBIDir string
	// ProfileID is recorded in link_state; "" means "retroarch".
	ProfileID string
	// ForceCopy makes every output a real copy (device export, spec §2.4)
	// instead of a link, and makes re-runs refresh copies whose size no
	// longer matches the source.
	ForceCopy bool
	// MaxFileBytes (with ForceCopy) rejects files larger than the target
	// filesystem allows (FAT32: 4 GiB - 1). 0 = no limit.
	MaxFileBytes int64
	// Include, when set, restricts the output to the games it accepts
	// (the user's per-output ticks). Games it rejects are not generated and
	// any outputs recorded for them earlier are removed. Nil = every
	// selected game.
	Include func(store.SelectedGame) bool
	// Context, when set, lets the caller cancel a run: extraction and
	// conversion stop (a running chdman/dolphin-tool is killed) and
	// generation returns the context's error. Nil = not cancellable.
	Context context.Context
	// PlaylistDir, for a RetroArch output, is RetroArch's own playlist folder: the
	// output's entries are merged into the playlists there (see mergePlaylist).
	// Empty = only the playlist inside the output.
	PlaylistDir string
	// ChooseCore, with PlaylistDir, picks the default core for a playlist that has
	// none, given the content types its games use ("chd", "m3u"); a core already
	// set there is kept. Nil = no default core.
	ChooseCore func(exts []string) (path, name, reason string, ok bool)
	// Parallelism is how many disc conversions may run at once (below 2 =
	// one at a time). Each needs its own scratch space.
	Parallelism int
	// OnProgress, when set, is told how far each conversion is (item = the
	// game, percent 0-100). It may be called from several goroutines.
	OnProgress func(item string, percent float64)
	// SevenZip is the path of the bundled 7-Zip, needed to extract from RAR archives.
	SevenZip string
	// OnItem, when set, is told which game (or multi-disc set) is being
	// processed. Called from the generating goroutine only.
	OnItem func(title string)

	// failed holds conversions that already failed in the prewarm pass.
	failed map[string]error
}

// ctx returns the cancellation context, never nil.
func (o Options) ctx() context.Context {
	if o.Context == nil {
		return context.Background()
	}
	return o.Context
}

// LinkedGame is one game actually linked into the profile.
type LinkedGame struct {
	GameKey string
	Title   string
	Path    string // absolute path of the link target
}

// Report summarizes one generation run of an output for one system.
type Report struct {
	Linked []LinkedGame
	// Errors holds one message per game that couldn't be linked, so a
	// single bad game doesn't abort the whole profile (spec §5:
	// "every skip, fallback, grouping method, and copy is visible").
	Errors []string
	// Notes holds non-fatal remarks the user should see (fallbacks taken).
	Notes []string
}

// profileIDOf is the link_state profile_id the output records under.
func profileIDOf(opts Options) string {
	if opts.ProfileID != "" {
		return opts.ProfileID
	}
	return retroArchProfileID
}
