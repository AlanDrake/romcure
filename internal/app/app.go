// Package app is the orchestration layer the UI calls into: it owns the
// configuration (sources, profiles, settings), and runs the whole pipeline
// (scan → identify/select → generate each profile → verify). It has no
// Wails dependency, so it is testable without a window; cmd/romcure
// exposes it through a thin service.
package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"romcure/internal/catalog"
	"romcure/internal/discover"
	"romcure/internal/elevate"
	"romcure/internal/fsinfo"
	"romcure/internal/library"
	"romcure/internal/link"
	"romcure/internal/plan"
	"romcure/internal/profile"
	"romcure/internal/rules"
	"romcure/internal/scan"
	"romcure/internal/settings"
	"romcure/internal/store"
	"romcure/internal/transcode"
)

// Tools locates the bundled converters. Empty paths disable that converter:
// disc images are then linked as they are, or reported as errors where a
// system can't be used without conversion.
type Tools struct {
	Chdman, ChdmanVersion   string
	Dolphin, DolphinVersion string
	SevenZip                string // reads RAR archives
}

// App is the application core.
type App struct {
	DB       *store.DB
	Dir      string // app data dir (library.db, caches, settings)
	Settings settings.Settings
	Tools    Tools
	Log      *slog.Logger
	// Warning is a non-fatal startup problem (e.g. a corrupt settings file).
	Warning error

	// Games screen state: the index (rebuilt lazily after a sync or version
	// change) and the undo history of tick and version changes.
	gamesMu sync.Mutex
	cat     *catalog.Catalog
	incl    map[int64]*store.Inclusion // cached ticks per output
	undo    []undoEntry
	undoSeq int

	// symlinksOK is whether this process can create symbolic links; probed
	// once at startup. elevationApplies is whether the Windows elevation
	// flow (spec §2.3) is relevant: true on Windows, or anywhere when
	// ROMCURE_ASSUME_NO_SYMLINKS is set (a test hook for the UI).
	symlinksOK       bool
	elevationApplies bool

	// stepDelay pauses after every progress step: a test hook
	// (ROMCURE_STEP_DELAY_MS) that makes short jobs long enough to watch.
	stepDelay time.Duration
}

// Open opens (creating) the library in dir and loads the settings. A
// corrupt settings file does not stop the app: it starts on defaults and the
// problem is left in App.Warning for the UI to show.
func Open(dir string, tools Tools, log *slog.Logger) (*App, error) {
	if log == nil {
		log = slog.New(slog.NewTextHandler(os.Stderr, nil))
	}
	db, err := store.Open(filepath.Join(dir, "library.db"))
	if err != nil {
		return nil, err
	}
	s, warn := settings.Load(dir)
	a := &App{DB: db, Dir: dir, Settings: s, Tools: tools, Log: log, Warning: warn}
	if ms, err := strconv.Atoi(os.Getenv("ROMCURE_STEP_DELAY_MS")); err == nil && ms > 0 {
		a.stepDelay = time.Duration(ms) * time.Millisecond
	}
	a.symlinksOK, _ = elevate.SymlinkProbe()
	a.elevationApplies = runtime.GOOS == "windows"
	if os.Getenv("ROMCURE_ASSUME_NO_SYMLINKS") != "" {
		a.symlinksOK, a.elevationApplies = false, true
	}
	if err := a.registerRoots(); err != nil {
		db.Close()
		return nil, err
	}
	return a, nil
}

// Close releases the database.
func (a *App) Close() error { return a.DB.Close() }

// registerRoots (re)registers every source with the write guard.
func (a *App) registerRoots() error {
	link.ClearSourceRoots()
	srcs, err := a.DB.Sources()
	if err != nil {
		return err
	}
	for _, s := range srcs {
		if err := link.RegisterSourceRoot(s.Path); err != nil {
			return err
		}
	}
	return nil
}

// overlaps reports whether either path is the same as, or inside, the other.
func overlaps(a, b string) bool {
	a, b = strings.ToLower(filepath.Clean(a)), strings.ToLower(filepath.Clean(b))
	sep := string(filepath.Separator)
	return a == b || strings.HasPrefix(a, b+sep) || strings.HasPrefix(b, a+sep)
}

// AddSource registers a ROM collection root for one system. A system may have
// several folders: their games are one library, and a game found in more than
// one is used from the folder added first.
func (a *App) AddSource(path, systemID string) (store.Source, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return store.Source{}, err
	}
	if st, err := os.Stat(abs); err != nil || !st.IsDir() {
		return store.Source{}, fmt.Errorf("%s is not a folder", abs)
	}
	if strings.TrimSpace(systemID) == "" {
		return store.Source{}, errors.New("choose which system this folder holds")
	}
	srcs, err := a.DB.Sources()
	if err != nil {
		return store.Source{}, err
	}
	for _, s := range srcs {
		if overlaps(s.Path, abs) {
			return store.Source{}, fmt.Errorf("%s overlaps the existing source %s", abs, s.Path)
		}
	}
	profs, err := a.DB.Profiles()
	if err != nil {
		return store.Source{}, err
	}
	for _, p := range profs {
		if overlaps(p.OutputRoot, abs) {
			return store.Source{}, fmt.Errorf("%s overlaps the output of profile %q; sources are never written to", abs, p.Name)
		}
	}
	src, err := a.DB.AddSource(abs, systemID)
	if err != nil {
		return store.Source{}, err
	}
	return src, a.registerRoots()
}

// RemoveSource unregisters a source and forgets the games found in it (the
// same game in another folder of the system comes back at the next sync).
func (a *App) RemoveSource(id int64) error {
	srcs, err := a.DB.Sources()
	if err != nil {
		return err
	}
	if err := a.DB.RemoveSource(id); err != nil {
		return err
	}
	for _, s := range srcs {
		if s.ID == id {
			if _, err := a.DB.PruneGamesUnderRoot(s.SystemID, s.Path, nil); err != nil {
				return err
			}
			a.InvalidateCatalog()
		}
	}
	return a.registerRoots()
}

// systemsOf lists the distinct systems that have a source, in order, with the
// folders of each.
func systemsOf(srcs []store.Source) (ids []string, roots map[string][]string) {
	roots = map[string][]string{}
	for _, s := range srcs {
		if _, seen := roots[s.SystemID]; !seen {
			ids = append(ids, s.SystemID)
		}
		roots[s.SystemID] = append(roots[s.SystemID], s.Path)
	}
	return ids, roots
}

var profileKinds = map[string]bool{"retroarch": true, "esde": true, "retrodeck": true, "batocera": true, "retrobat": true, "raw": true, "device": true}

// AddProfile validates and stores an output profile. Outputs must not lie
// inside (or contain) a source, must not be on a network share (spec §2.4),
// and FAT32/exFAT destinations are only allowed for the "device" kind, which
// copies.
func (a *App) AddProfile(p store.Profile) (store.Profile, error) {
	if strings.TrimSpace(p.Name) == "" {
		return store.Profile{}, errors.New("give the profile a name")
	}
	if !profileKinds[p.Kind] {
		return store.Profile{}, fmt.Errorf("unknown profile kind %q", p.Kind)
	}
	abs, err := a.checkOutputFolder(p.OutputRoot, p.Kind)
	if err != nil {
		return store.Profile{}, err
	}
	p.OutputRoot = abs
	if p.RetroArchConfig, err = checkRetroArchConfig(p.Kind, p.RetroArchConfig); err != nil {
		return store.Profile{}, err
	}
	return a.DB.AddProfile(p)
}

// checkRetroArchConfig returns the retroarch.cfg to keep for an output: "" for
// any kind but RetroArch, otherwise the path, which must be a readable config.
func checkRetroArchConfig(kind, path string) (string, error) {
	if kind != "retroarch" || strings.TrimSpace(path) == "" {
		return "", nil
	}
	in, err := discover.ParseRetroArchConfig(path)
	if err != nil {
		return "", fmt.Errorf("RetroArch's config can't be read: %w", err)
	}
	return in.Config, nil
}

// SetRetroArchConfig chooses the RetroArch an existing RetroArch output is
// for; "" clears the choice.
func (a *App) SetRetroArchConfig(id int64, path string) error {
	profs, err := a.DB.Profiles()
	if err != nil {
		return err
	}
	for _, p := range profs {
		if p.ID == id {
			path, err = checkRetroArchConfig(p.Kind, path)
			if err != nil {
				return err
			}
			return a.DB.SetRetroArchConfig(id, path)
		}
	}
	return fmt.Errorf("output %d not found", id)
}

// RemoveProfile deletes an output. With deleteLinks it first removes the
// links and copies the output created inside its folder (never the user's
// games or other files). Its link_state rows are always forgotten, since a
// later output may be given the same id.
func (a *App) RemoveProfile(id int64, deleteLinks bool) error {
	profs, err := a.DB.Profiles()
	if err != nil {
		return err
	}
	for _, p := range profs {
		if p.ID != id {
			continue
		}
		pid := store.OutputKey(id)
		if deleteLinks {
			if _, err := profile.RemoveOutputs(a.DB, pid, p.OutputRoot); err != nil {
				return fmt.Errorf("remove the output's files: %w", err)
			}
			// A RetroArch that is gone has no playlists left to clean; one that is open
			// would rewrite them, so the user is asked to close it first.
			if _, err := discover.ParseRetroArchConfig(p.RetroArchConfig); err == nil && p.Kind == "retroarch" {
				ra, err := playlistDir(p)
				if err != nil {
					return err
				}
				if _, err := profile.RemoveFromPlaylists(ra.Playlists, p.OutputRoot); err != nil {
					return fmt.Errorf("remove the output's playlist entries: %w", err)
				}
			}
		}
		if err := a.DB.RemoveProfileData(pid); err != nil {
			return err
		}
	}
	return a.DB.RemoveProfile(id)
}

// SystemResult summarises one system's identification and selection.
type SystemResult struct {
	SystemID   string
	Selected   int
	Excluded   int
	Unmatched  int
	Overridden int
}

// ProfileResult summarises one profile × system generation.
type ProfileResult struct {
	Profile  string
	SystemID string
	Linked   int
	Errors   []string
	Notes    []string
	// Verified counts the outputs checked after generation.
	OK, Broken, Stale int
	Blockers          []string // export plan blockers, if the run was refused
}

// SyncReport is the outcome of one Sync run.
type SyncReport struct {
	Systems  []SystemResult
	Profiles []ProfileResult
	// Unreadable lists files the scan had to skip (damaged archives...).
	Unreadable []UnreadableFile
}

// AfterApply is the report to show after an Apply: the last sync's per-system
// results and unreadable files (an Apply does not scan), with the results of
// the outputs that were just updated replacing that sync's results for them.
// last may be nil (no sync yet in this session).
func AfterApply(last *SyncReport, applied SyncReport) SyncReport {
	var out SyncReport
	if last != nil {
		out.Systems, out.Unreadable = last.Systems, last.Unreadable
		updated := map[string]bool{}
		for _, p := range applied.Profiles {
			updated[p.Profile] = true
		}
		for _, p := range last.Profiles {
			if !updated[p.Profile] {
				out.Profiles = append(out.Profiles, p)
			}
		}
	}
	out.Profiles = append(out.Profiles, applied.Profiles...)
	return out
}

// UnreadableFile is a file the scan could not read; the sync went on without it.
type UnreadableFile struct {
	SystemID string
	Path     string
	Reason   string
}

// transcodeOptions builds the converter configuration from the tools and
// settings, or nil when no converter is available.
func (a *App) transcodeOptions() *profile.TranscodeOptions {
	if a.Tools.Chdman == "" && a.Tools.Dolphin == "" {
		return nil
	}
	t := &profile.TranscodeOptions{
		Cache:      &transcode.Cache{Dir: a.bigCache("transcode-cache"), DB: a.DB},
		ScratchDir: a.Settings.Scratch(a.Dir),
		Profile:    a.Settings.CHDProfile,
	}
	if a.Tools.Chdman != "" {
		t.Tool = &transcode.Chdman{Path: a.Tools.Chdman, Version: a.Tools.ChdmanVersion}
	}
	if a.Tools.Dolphin != "" {
		t.RVZ = &transcode.DolphinTool{Path: a.Tools.Dolphin, Version: a.Tools.DolphinVersion, UserDir: filepath.Join(a.Dir, "dolphin-user")}
	}
	return t
}

func (a *App) prefs() rules.Preferences {
	if p, ok := rules.PresetByID(a.Settings.RegionPreset); ok {
		return p.Prefs
	}
	return rules.Preferences{}
}

// Sync runs the full pipeline for every source and every profile. Failures
// of one game or one profile are reported in the result rather than
// aborting the run; only cancellation and database errors return an error.
func (a *App) Sync(ctx context.Context, progress func(Progress)) (SyncReport, error) {
	var report SyncReport
	srcs, err := a.DB.Sources()
	if err != nil {
		return report, err
	}
	profs, err := a.DB.Profiles()
	if err != nil {
		return report, err
	}
	// Steps: scan each folder, identify each system, then generate each output
	// for each system and verify it.
	systems, roots := systemsOf(srcs)
	r := a.reporter(ctx, progress, len(srcs)+len(systems)+len(profs)*(len(systems)+1))
	for _, s := range srcs {
		r.step(Progress{Stage: "scan", SystemID: s.SystemID, Detail: s.Path})
		sc := scan.New(a.DB)
		sc.SevenZip = a.Tools.SevenZip
		if err := sc.Scan(ctx, s.Path, func(sp scan.Progress) {
			r.item(Progress{Stage: "scan", SystemID: s.SystemID, Detail: s.Path, Item: filepath.Base(sp.CurrentPath)})
		}); err != nil {
			return report, fmt.Errorf("scan %s: %w", s.Path, err)
		}
		for _, p := range sc.Problems {
			a.Log.Warn("skipped unreadable file", "system", s.SystemID, "path", p.Path, "reason", p.Reason)
			report.Unreadable = append(report.Unreadable, UnreadableFile{SystemID: s.SystemID, Path: p.Path, Reason: p.Reason})
		}
		r.next()
	}
	for _, sys := range systems {
		r.step(Progress{Stage: "identify", SystemID: sys})
		rep, err := library.SyncRoots(a.DB, roots[sys], sys, a.prefs())
		if err != nil {
			return report, err
		}
		r.next()
		report.Systems = append(report.Systems, SystemResult{
			SystemID: sys, Selected: len(rep.Selected), Excluded: len(rep.Excluded),
			Unmatched: len(rep.Unmatched), Overridden: len(rep.Overridden),
		})
	}

	a.InvalidateCatalog()
	return report, a.generateOutputs(ctx, systems, profs, r, &report)
}

// generateOutputs regenerates and verifies the given outputs for every system
// with a source. It does not scan or re-run the rules: it applies what the
// database already says, so it is quick when little changed.
func (a *App) generateOutputs(ctx context.Context, systems []string, profs []store.Profile, r *reporter, report *SyncReport) error {
	for _, p := range profs {
		ra, perr := playlistDir(p)
		if perr != nil {
			// Nothing is generated for this output, so no half-updated playlists.
			report.Profiles = append(report.Profiles, ProfileResult{Profile: p.Name, Errors: []string{perr.Error()}})
			continue
		}
		// Read the cores and what the user already plays with once per output.
		var cores *discover.CoreChooser
		if ra != nil && !a.Settings.NoDefaultCores {
			cores = ra.NewCoreChooser()
		}
		// Every system with a source is generated; which games each output
		// takes is decided by the user's ticks (Inclusion), not by a list here.
		for _, sys := range systems {
			if err := ctx.Err(); err != nil {
				return err
			}
			r.step(Progress{Stage: "generate", SystemID: sys, Profile: p.Name})
			report.Profiles = append(report.Profiles, a.generate(ctx, p, sys, ra, cores, func(item string, pct float64) {
				r.item(Progress{Stage: "convert", SystemID: sys, Profile: p.Name, Item: item, Percent: pct})
			}, func(title string) {
				r.item(Progress{Stage: "generate", SystemID: sys, Profile: p.Name, Item: title})
			}))
			r.next()
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		r.step(Progress{Stage: "verify", Profile: p.Name})
		a.verify(p, report)
		r.next()
	}
	return nil
}

func (a *App) generate(ctx context.Context, p store.Profile, systemID string, ra *discover.RetroArchInstall, cores *discover.CoreChooser, onProgress func(item string, percent float64), onItem func(title string)) ProfileResult {
	res := ProfileResult{Profile: p.Name, SystemID: systemID}
	opts, err := a.outputOptions(p, systemID)
	if err != nil {
		res.Errors = append(res.Errors, err.Error())
		return res
	}

	if ra != nil {
		opts.PlaylistDir = ra.Playlists
		if cores != nil {
			dbName, _ := profile.LibretroDBName(systemID)
			opts.ChooseCore = func(exts []string) (string, string, string, bool) {
				c, ok := cores.Choose(systemID, dbName, exts)
				return c.Path, c.Name, c.Reason, ok
			}
		}
	}
	opts.OnProgress = onProgress
	opts.OnItem = onItem
	opts.Context = ctx // a cancelled run stops conversions and extractions, not just between games
	var rep profile.Report
	switch p.Kind {
	case "retroarch":
		rep, err = profile.GenerateRetroArch(a.DB, systemID, opts)
	case "raw":
		rep, err = profile.GenerateRaw(a.DB, systemID, opts)
	case "device":
		plan, reports, derr := profile.ExportToDevice(a.DB, profile.DeviceExportOptions{
			Options: opts, Frontend: profile.FrontendESDE, Systems: []string{systemID},
		})
		if derr != nil {
			err = derr
		} else if !plan.OK() {
			res.Blockers = plan.Blockers
		} else {
			rep = reports[systemID]
		}
	default: // esde, retrodeck, batocera, retrobat
		rep, err = profile.GenerateESFamily(a.DB, systemID, profile.Frontend(p.Kind), opts)
	}
	if err != nil {
		res.Errors = append(res.Errors, err.Error())
	}
	res.Linked, res.Errors, res.Notes = len(rep.Linked), append(res.Errors, rep.Errors...), rep.Notes
	return res
}

// verify checks a profile's recorded outputs and folds the counts into its
// results.
func (a *App) verify(p store.Profile, report *SyncReport) {
	findings, err := plan.VerifyProfile(a.DB, store.OutputKey(p.ID), p.OutputRoot)
	if err != nil {
		a.Log.Error("verify profile", "profile", p.Name, "error", err)
		return
	}
	bySystem := map[string][]plan.Finding{}
	for _, f := range findings {
		bySystem[f.SystemID] = append(bySystem[f.SystemID], f)
	}
	for i := range report.Profiles {
		pr := &report.Profiles[i]
		if pr.Profile == p.Name {
			s := plan.Summarize(bySystem[pr.SystemID])
			pr.OK, pr.Broken, pr.Stale = s.OK, s.Broken, s.Stale
		}
	}
}

// cachePath is a folder inside the app data dir.
func (a *App) cachePath(parts ...string) string {
	return filepath.Join(append([]string{a.Dir}, parts...)...)
}

// bigCache is a cache folder that can grow large (converted discs, extracted
// games); the user may move these with the CacheDir setting.
func (a *App) bigCache(name string) string {
	return filepath.Join(a.Settings.CacheRoot(a.Dir), name)
}

// checkOutputFolder validates a folder for an output of the given kind and
// creates it: it must not overlap a source, be on a network share, or (for
// link-based kinds) be a FAT32/exFAT volume. It returns the absolute path.
func (a *App) checkOutputFolder(path, kind string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	srcs, err := a.DB.Sources()
	if err != nil {
		return "", err
	}
	for _, s := range srcs {
		if overlaps(s.Path, abs) {
			return "", fmt.Errorf("output %s overlaps the source %s; sources are never written to", abs, s.Path)
		}
	}
	if err := os.MkdirAll(abs, 0o755); err != nil {
		return "", fmt.Errorf("create %s: %w", abs, err)
	}
	info, err := fsinfo.Detect(abs)
	if err != nil {
		return "", fmt.Errorf("inspect %s: %w", abs, err)
	}
	if info.IsRemote {
		return "", errors.New("network shares are not supported as outputs: links and copies are unreliable there")
	}
	if (info.Type == fsinfo.FAT32 || info.Type == fsinfo.ExFAT) && kind != "device" {
		return "", fmt.Errorf("%s is %s, which cannot hold links; use the \"device\" (copy) profile for SD cards and USB sticks", abs, info.Type)
	}
	return abs, nil
}

// UpdateProfile renames an output and/or moves it to another folder. Moving
// forgets what the output recorded for the old folder (the new one is filled
// by the next Apply); deleteOld also removes the links and copies it made
// there (never the user's own files).
func (a *App) UpdateProfile(id int64, name, folder string, deleteOld bool) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return errors.New("give the output a name")
	}
	old, err := a.profileByID(id)
	if err != nil {
		return err
	}
	abs, err := a.checkOutputFolder(folder, old.Kind)
	if err != nil {
		return err
	}
	pid := store.OutputKey(id)
	if !samePathText(abs, old.OutputRoot) {
		if deleteOld {
			if _, err := profile.RemoveOutputs(a.DB, pid, old.OutputRoot); err != nil {
				return fmt.Errorf("remove the old links: %w", err)
			}
		}
		if err := a.DB.RemoveProfileData(pid); err != nil {
			return err
		}
	}
	if err := a.DB.UpdateProfile(id, name, abs); err != nil {
		return err
	}
	a.dropInclusions()
	return nil
}

func samePathText(a, b string) bool {
	return strings.EqualFold(filepath.Clean(a), filepath.Clean(b))
}

// OutputProblem is one recorded output that is broken or out of date.
type OutputProblem struct {
	Name   string // the file's name in the output
	Path   string // relative to the output folder
	Status string // "broken" | "stale"
	Reason string
}

// OutputProblems checks everything an output recorded against the disk and
// lists what is wrong.
func (a *App) OutputProblems(id int64) ([]OutputProblem, error) {
	p, err := a.profileByID(id)
	if err != nil {
		return nil, err
	}
	findings, err := plan.VerifyProfile(a.DB, store.OutputKey(id), p.OutputRoot)
	if err != nil {
		return nil, err
	}
	var out []OutputProblem
	for _, f := range findings {
		if f.Status != "ok" {
			out = append(out, OutputProblem{Name: filepath.Base(f.TargetPath), Path: f.TargetPath, Status: f.Status, Reason: f.Reason})
		}
	}
	return out, nil
}

// RepairOutput removes the broken and stale outputs of an output (links and
// copies only, inside its folder) so the next Apply recreates them. It
// returns how many were removed.
func (a *App) RepairOutput(id int64) (int, error) {
	p, err := a.profileByID(id)
	if err != nil {
		return 0, err
	}
	findings, err := plan.VerifyProfile(a.DB, store.OutputKey(id), p.OutputRoot)
	if err != nil {
		return 0, err
	}
	return plan.Repair(p.OutputRoot, findings)
}

// playlistDir is the RetroArch chosen for a RetroArch output, nil for every other output. It refuses while RetroArch
// is open, since RetroArch rewrites its playlists when it closes and would undo
// ours.
func playlistDir(p store.Profile) (*discover.RetroArchInstall, error) {
	if p.Kind != "retroarch" || p.RetroArchConfig == "" {
		return nil, nil
	}
	in, err := discover.ParseRetroArchConfig(p.RetroArchConfig)
	if err != nil {
		return nil, fmt.Errorf("%s: RetroArch's config can't be read (%v). Choose RetroArch again on the Outputs tab.", p.Name, err)
	}
	if discover.RetroArchOpen() {
		return nil, fmt.Errorf("%s: RetroArch is open. Close it and press Apply again, or it would overwrite the playlists.", p.Name)
	}
	return &in, nil
}
