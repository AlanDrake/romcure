package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"

	"romcure/internal/app"
	"romcure/internal/catalog"
	"romcure/internal/discover"
	"romcure/internal/notices"
	"romcure/internal/proc"
	"romcure/internal/profile"
	"romcure/internal/rules"
	"romcure/internal/settings"
	"romcure/internal/store"
	"romcure/internal/support"
)

// Event names the frontend listens to. Every background job (sync, DAT
// update, CHD check) reports through the same two events, tagged by Kind.
const (
	eventJobProgress = "job:progress"
	eventJobDone     = "job:done"
)

func init() {
	application.RegisterEvent[JobProgress](eventJobProgress)
	application.RegisterEvent[JobDone](eventJobDone)
}

// JobProgress is one progress step of the running job.
type JobProgress struct {
	Kind     string // "sync" | "apply" | "dats" | "chd"
	Stage    string
	SystemID string
	Profile  string
	Detail   string
	Item     string  // the game or file being worked on
	Done     int     // steps of the whole job finished
	Total    int     // steps of the whole job (0 = unknown)
	Percent  float64 // of a running conversion, 0-100
}

// JobDone is emitted when a job finishes, fails or is cancelled. Only the
// result field matching Kind is set.
type JobDone struct {
	Kind      string
	Error     string
	Cancelled bool
	Report    *app.SyncReport
	DATs      []app.DATResult
	CHDs      []app.CHDResult
}

// SourceInfo is a source with its display name.
type SourceInfo struct {
	ID         int64
	Path       string
	SystemID   string
	SystemName string
	Short      string
}

// PresetInfo describes a region preset.
type PresetInfo struct {
	ID          string
	Name        string
	Description string
}

// ToolInfo reports whether a bundled converter was found.
type ToolInfo struct {
	Name    string
	Found   bool
	Path    string
	Version string
}

// State is everything the UI needs to render, in one call.
type State struct {
	Sources  []SourceInfo
	Profiles []store.Profile
	Settings settings.Settings
	Systems  []profile.SystemInfo
	Presets  []PresetInfo
	Tools    []ToolInfo
	Warning  string
	// Job is the kind of the running background job, "" when idle.
	Job        string
	LastReport *app.SyncReport
	DataDir    string
	Links      app.LinkStatus
	// CacheRoot and ScratchDir are where the big caches and the conversion
	// scratch space really are, for the settings screen to show.
	CacheRoot  string
	ScratchDir string
}

// Service is the thin Wails-facing layer over internal/app: it adapts calls
// and events, and holds no logic of its own.
type Service struct {
	core *app.App

	mu     sync.Mutex
	job    string
	cancel context.CancelFunc
	last   *app.SyncReport
}


// State returns the full UI state.
func (s *Service) State() (State, error) {
	srcs, err := s.core.DB.Sources()
	if err != nil {
		return State{}, err
	}
	profs, err := s.core.DB.Profiles()
	if err != nil {
		return State{}, err
	}
	names := map[string]string{}
	systems := profile.KnownSystems()
	for _, sys := range systems {
		names[sys.ID] = sys.Name
	}
	st := State{Profiles: profs, Settings: s.core.Settings, Systems: systems, DataDir: s.core.Dir, Links: s.core.LinkStatus(),
		CacheRoot: s.core.Settings.CacheRoot(s.core.Dir), ScratchDir: s.core.Settings.Scratch(s.core.Dir)}
	for _, src := range srcs {
		name := names[src.SystemID]
		if name == "" {
			name = src.SystemID
		}
		st.Sources = append(st.Sources, SourceInfo{ID: src.ID, Path: src.Path, SystemID: src.SystemID, SystemName: name, Short: profile.ShortName(src.SystemID)})
	}
	for _, p := range rules.Presets {
		st.Presets = append(st.Presets, PresetInfo{ID: p.ID, Name: p.Name, Description: p.Description})
	}
	st.Tools = []ToolInfo{
		{Name: "chdman", Found: s.core.Tools.Chdman != "", Path: s.core.Tools.Chdman, Version: s.core.Tools.ChdmanVersion},
		{Name: "dolphin-tool", Found: s.core.Tools.Dolphin != "", Path: s.core.Tools.Dolphin, Version: s.core.Tools.DolphinVersion},
		{Name: "7-Zip (RAR)", Found: s.core.Tools.SevenZip != "", Path: s.core.Tools.SevenZip},
	}
	if s.core.Warning != nil {
		st.Warning = s.core.Warning.Error()
	}
	s.mu.Lock()
	st.Job, st.LastReport = s.job, s.last
	s.mu.Unlock()
	return st, nil
}

// PickFolder opens a native folder chooser; "" means the user cancelled.
func (s *Service) PickFolder(title string) (string, error) {
	return application.Get().Dialog.OpenFile().
		CanChooseDirectories(true).CanChooseFiles(false).CanCreateDirectories(true).
		SetTitle(title).PromptForSingleSelection()
}

// PickDATFile opens a native file chooser for a DAT file; "" means cancelled.
func (s *Service) PickDATFile() (string, error) {
	return application.Get().Dialog.OpenFile().
		CanChooseFiles(true).CanChooseDirectories(false).
		AddFilter("DAT files (.dat, .xml, .zip)", "*.dat;*.xml;*.zip").
		SetTitle("Choose a DAT file").PromptForSingleSelection()
}

// ImportDAT imports a DAT file by hand (systemID "" = the system the DAT
// names). The next sync uses it.
func (s *Service) ImportDAT(path, systemID string) (app.DATImport, error) {
	return s.core.ImportDAT(path, systemID)
}

func (s *Service) AddSource(path, systemID string) error {
	_, err := s.core.AddSource(path, systemID)
	return err
}

func (s *Service) RemoveSource(id int64) error { return s.core.RemoveSource(id) }

// AddProfile adds an output; startEmpty makes it take no games until they are
// ticked (the right start for an SD card).
func (s *Service) AddProfile(name, kind, outputRoot string, systems []string, startEmpty bool, retroarchConfig string) error {
	_, err := s.core.AddProfile(store.Profile{Name: name, Kind: kind, OutputRoot: outputRoot, Systems: systems, StartEmpty: startEmpty, RetroArchConfig: retroarchConfig})
	return err
}

// RemoveProfile deletes an output; deleteLinks also removes the links and
// copies it made (never the user's games).
func (s *Service) RemoveProfile(id int64, deleteLinks bool) error {
	return s.core.RemoveProfile(id, deleteLinks)
}

// SaveSettings validates and persists new settings.
func (s *Service) SaveSettings(next settings.Settings) error {
	if err := settings.Save(s.core.Dir, next); err != nil {
		return err
	}
	s.core.Settings = next
	return nil
}

// startJob runs fn in the background unless another job is running.
// fn fills in the result fields of done; errors are reported in done too.
func (s *Service) startJob(kind string, fn func(ctx context.Context, emit func(app.Progress), done *JobDone) error) error {
	s.mu.Lock()
	if s.job != "" {
		s.mu.Unlock()
		return fmt.Errorf("wait for the current %s to finish", s.job)
	}
	ctx, cancel := context.WithCancel(context.Background())
	s.job, s.cancel = kind, cancel
	s.mu.Unlock()

	go func() {
		var (
			emitMu   sync.Mutex
			lastConv time.Time
		)
		emit := func(p app.Progress) {
			// Conversions report every percent and scans every file; the screen only needs a few per second.
			if p.Stage == "convert" || p.Item != "" {
				emitMu.Lock()
				skip := time.Since(lastConv) < 250*time.Millisecond && p.Percent < 100
				if !skip {
					lastConv = time.Now()
				}
				emitMu.Unlock()
				if skip {
					return
				}
			}
			application.Get().Event.Emit(eventJobProgress, JobProgress{
				Kind: kind, Stage: p.Stage, SystemID: p.SystemID, Profile: p.Profile, Detail: p.Detail,
				Item: p.Item, Done: p.Done, Total: p.Total, Percent: p.Percent,
			})
		}
		done := JobDone{Kind: kind}
		if err := fn(ctx, emit, &done); err != nil {
			done.Error = err.Error()
			done.Cancelled = errors.Is(err, context.Canceled)
		}
		s.mu.Lock()
		if done.Report != nil && done.Error == "" {
			s.last = done.Report
		}
		s.job, s.cancel = "", nil
		s.mu.Unlock()
		cancel()
		application.Get().Event.Emit(eventJobDone, done)
	}()
	return nil
}

// StartSync runs the full pipeline (scan, identify, generate, verify).
func (s *Service) StartSync() error {
	return s.startJob("sync", func(ctx context.Context, emit func(app.Progress), done *JobDone) error {
		report, err := s.core.Sync(ctx, emit)
		done.Report = &report
		return err
	})
}

// StartDATUpdate downloads newer Redump/TOSEC DATs for the configured systems.
func (s *Service) StartDATUpdate() error {
	return s.startJob("dats", func(ctx context.Context, emit func(app.Progress), done *JobDone) error {
		res, err := s.core.UpdateDATs(ctx, app.Endpoints{}, emit)
		done.DATs = res
		return err
	})
}

// StartCHDCheck hashes CHD files so they can be matched against Redump.
func (s *Service) StartCHDCheck() error {
	return s.startJob("chd", func(ctx context.Context, emit func(app.Progress), done *JobDone) error {
		res, err := s.core.VerifyCHDs(ctx, emit)
		done.CHDs = res
		return err
	})
}

// CancelJob stops the running job, if any.
func (s *Service) CancelJob() {
	s.mu.Lock()
	if s.cancel != nil {
		s.cancel()
	}
	s.mu.Unlock()
}

// runningJob is the kind of the running background job, "" when idle.
func (s *Service) runningJob() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.job
}

// stopAndWait cancels the running job and waits (up to timeout) for it to wind down,
// so the library is not closed underneath it.
func (s *Service) stopAndWait(timeout time.Duration) {
	s.CancelJob()
	for deadline := time.Now().Add(timeout); s.runningJob() != "" && time.Now().Before(deadline); {
		time.Sleep(50 * time.Millisecond)
	}
}

// ExportLogBundle lets the user choose a file and writes the diagnostics bundle there.
func (s *Service) ExportLogBundle() (string, error) {
	dest, err := application.Get().Dialog.SaveFile().SetFilename("romcure-diagnostics.zip").PromptForSingleSelection()
	if err != nil || dest == "" {
		return "", err
	}
	info := map[string]string{"os": runtime.GOOS + "/" + runtime.GOARCH}
	if s.core.Tools.Chdman != "" {
		info["chdman"] = s.core.Tools.ChdmanVersion
	}
	if s.core.Tools.Dolphin != "" {
		info["dolphin-tool"] = s.core.Tools.DolphinVersion
	}
	if s.core.Tools.SevenZip != "" {
		info["7-zip"] = "found"
	}
	if err := support.ExportBundle(s.core.Dir, dest, info); err != nil {
		return "", fmt.Errorf("write diagnostics bundle: %w", err)
	}
	return dest, nil
}

// findTools looks for the converters next to the executable (installed
// layout: <exe dir>/tools/), in the repository's tools/bin/<os>-<arch>
// (development), and finally on the PATH, where Linux and macOS package
// managers put them (mame-tools, rom-tools, dolphin-emu, 7zip).
func findTools() app.Tools {
	exe, _ := os.Executable()
	exeDir := filepath.Dir(exe)
	platform := runtime.GOOS + "-" + runtime.GOARCH
	dirs := []string{
		filepath.Join(exeDir, "tools"),
		filepath.Join(exeDir, "..", "tools", "bin", platform),
		filepath.Join("tools", "bin", platform),
	}
	find := func(names ...string) string { return findTool(dirs, names, exec.LookPath) }
	t := app.Tools{
		Chdman:   find("chdman", "chdman.exe"),
		Dolphin:  find("dolphin-tool", "DolphinTool.exe", "DolphinTool"),
		SevenZip: find("7zz", "7z.exe", "7z"),
	}
	t.ChdmanVersion, t.DolphinVersion = toolVersion(t.Chdman), toolVersion(t.Dolphin)
	return t
}

var chdmanVersion = regexp.MustCompile(`\((mame\d+)\)`)

// toolVersion identifies a bundled converter for the transcode cache key: the
// MAME release for chdman (printed by its usage text), otherwise a
// size/mtime fingerprint, so replacing the binary invalidates cached output.
func toolVersion(path string) string {
	if path == "" {
		return ""
	}
	if strings.Contains(strings.ToLower(filepath.Base(path)), "chdman") {
		out, _ := proc.Quiet(exec.Command(path)).CombinedOutput() // usage text; exits non-zero by design
		if m := chdmanVersion.FindSubmatch(out); m != nil {
			return string(m[1])
		}
	}
	if st, err := os.Stat(path); err == nil {
		return fmt.Sprintf("bin-%d-%d", st.Size(), st.ModTime().Unix())
	}
	return "unknown"
}

// Games lists one page of games for the Games screen.
func (s *Service) Games(f catalog.Filter, offset, limit int) (catalog.Page, error) {
	return s.core.Games(f, offset, limit)
}

// SystemCounts returns the systems rail for an output.
func (s *Service) SystemCounts(outputID int64) (app.SystemCounts, error) {
	return s.core.SystemCounts(outputID)
}

// Game returns the details of one list row.
func (s *Service) Game(key string) (app.GameDetail, error) { return s.core.Game(key) }

// SetIncluded ticks or unticks games on an output (saved at once).
func (s *Service) SetIncluded(outputID int64, t app.Target, included bool) (app.Change, error) {
	return s.core.SetIncluded(outputID, t, included)
}

// FollowSystem drops the user's own ticks on games.
func (s *Service) FollowSystem(outputID int64, keys []string) (app.Change, error) {
	return s.core.FollowSystem(outputID, keys)
}

// SetVersion chooses a game's version for every output.
func (s *Service) SetVersion(rowKey, gameKey string) (app.Change, error) {
	return s.core.SetVersion(rowKey, gameKey)
}

// AutoVersion puts a game back to the automatic version.
func (s *Service) AutoVersion(rowKey string) (app.Change, error) { return s.core.AutoVersion(rowKey) }

// Undo reverts the latest change ("" token) or the one named; returns what was undone.
func (s *Service) Undo(token string) (string, error) { return s.core.Undo(token) }

// Pending summarises what Apply would do to an output.
func (s *Service) Pending(outputID int64) (app.Pending, error) { return s.core.Pending(outputID) }

// StartApply updates the given outputs (all when empty) from the saved
// choices, without scanning the source folders.
func (s *Service) StartApply(outputIDs []int64) error {
	return s.startJob("apply", func(ctx context.Context, emit func(app.Progress), done *JobDone) error {
		report, err := s.core.Apply(ctx, outputIDs, emit)
		s.mu.Lock()
		merged := app.AfterApply(s.last, report)
		s.mu.Unlock()
		done.Report = &merged
		return err
	})
}

// Outputs returns each output's included games and waiting changes.
func (s *Service) Outputs() ([]app.OutputSummary, error) { return s.core.OutputSummaries() }

// RelaunchElevated restarts the app as administrator (Windows). It returns
// "restarting" when the new process is starting and this one is closing, or
// "declined" when the user said no (the choice is remembered).
func (s *Service) RelaunchElevated() (string, error) {
	restarting, err := s.core.RelaunchElevated()
	if err != nil {
		return "", err
	}
	if !restarting {
		return "declined", nil
	}
	application.Get().Quit()
	return "restarting", nil
}

// KeepCopies remembers "never ask": outputs use the next best method.
func (s *Service) KeepCopies() error { return s.core.SaveElevation(settings.ElevationNever) }

// Discover looks at a folder (and those inside it) for games and proposes a
// system for each, for the first-run setup.
func (s *Service) Discover(parent string) ([]discover.Candidate, error) {
	return discover.Folders(parent)
}

// OutputSuggestions lists places an output could go: installed frontends and
// removable drives.
func (s *Service) OutputSuggestions() []discover.Suggestion { return discover.Outputs() }

// EditOutput renames an output and/or moves it to another folder.
func (s *Service) EditOutput(id int64, name, folder string, deleteOld bool) error {
	return s.core.UpdateProfile(id, name, folder, deleteOld)
}

// OutputProblems lists what is wrong with an output's files.
func (s *Service) OutputProblems(id int64) ([]app.OutputProblem, error) {
	return s.core.OutputProblems(id)
}

// RepairOutput removes the broken and stale files of an output and updates it,
// so they are recreated. Returns how many were removed.
func (s *Service) RepairOutput(id int64) (int, error) {
	n, err := s.core.RepairOutput(id)
	if err != nil {
		return 0, err
	}
	return n, s.StartApply([]int64{id})
}

// LicenseInfo is one component on the Third-Party Licenses screen.
type LicenseInfo struct {
	Name    string
	Version string
	License string
	URL     string
	Kind    string // app | go | js | font | tool
	Note    string
}

// Licenses lists every component the app includes or runs.
func (s *Service) Licenses() []LicenseInfo {
	var out []LicenseInfo
	for _, e := range notices.All() {
		out = append(out, LicenseInfo{Name: e.Name, Version: e.Version, License: e.License, URL: e.URL, Kind: e.Kind, Note: e.Note})
	}
	return out
}

// LicenseText returns the full license text of a component.
func (s *Service) LicenseText(name string) string {
	e, _ := notices.Find(name)
	return e.Text
}

// RetroArchInstalls lists the RetroArch configurations found on this computer,
// the one that is open (or used last) first.
func (s *Service) RetroArchInstalls() []discover.RetroArchInstall { return discover.RetroArchInstalls() }

// PickRetroArch opens a native chooser for RetroArch's retroarch.cfg or program
// and returns that install; nil means cancelled.
func (s *Service) PickRetroArch() (*discover.RetroArchInstall, error) {
	path, err := application.Get().Dialog.OpenFile().
		CanChooseFiles(true).CanChooseDirectories(false).
		AddFilter("RetroArch (retroarch.cfg, retroarch.exe)", "*.cfg;*.exe").
		SetTitle("Choose retroarch.cfg or retroarch.exe").PromptForSingleSelection()
	if err != nil || path == "" {
		return nil, err
	}
	in, err := discover.RetroArchFromChoice(path)
	if err != nil {
		return nil, err
	}
	return &in, nil
}

// SetRetroArchConfig chooses the RetroArch an existing RetroArch output is for.
func (s *Service) SetRetroArchConfig(id int64, path string) error {
	return s.core.SetRetroArchConfig(id, path)
}
