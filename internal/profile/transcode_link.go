package profile

import (
	"sync"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"romcure/internal/discsheet"
	"romcure/internal/store"
	"romcure/internal/transcode"
)

// TranscodeOptions enables the Transcode archive policy (spec §2.8):
// disc images are converted to a verified compressed image in the
// transcode cache, and the cache entry is what gets linked. Tool (chdman)
// serves CD/DVD systems; RVZ (dolphin-tool) serves GameCube and Wii. Either
// may be nil, in which case those systems are not transcoded.
type TranscodeOptions struct {
	Cache      *transcode.Cache
	Tool       *transcode.Chdman
	RVZ        *transcode.DolphinTool
	ScratchDir string
	// Profile, when set, chooses the chdman codecs for a media kind (the
	// user's settings); nil = transcode.CodecsCompat.
	Profile func(transcode.Media) transcode.CodecProfile
}

// discPlan is the tool, codec profile and media chosen for one game.
type discPlan struct {
	tool   transcode.DiscTool
	codecs transcode.CodecProfile
	media  transcode.Media
}

// usesRVZ reports whether systemID is converted by dolphin-tool.
func usesRVZ(systemID string) bool { return systemID == "gc" || systemID == "wii" }

// planFor picks the converter for systemID, or ok=false when none is
// configured for it.
func (t *TranscodeOptions) planFor(systemID string, imageBytes int64) (discPlan, bool) {
	if t == nil || t.Cache == nil {
		return discPlan{}, false
	}
	if usesRVZ(systemID) {
		if t.RVZ == nil {
			return discPlan{}, false
		}
		return discPlan{tool: t.RVZ, codecs: transcode.RVZProfile}, true
	}
	if t.Tool == nil {
		return discPlan{}, false
	}
	media := mediaForSystem(systemID, imageBytes)
	codecs := transcode.CodecsCompat[media]
	if t.Profile != nil {
		codecs = t.Profile(media)
	}
	return discPlan{tool: t.Tool, codecs: codecs, media: media}, true
}

// available reports whether a converter is configured for systemID.
func (t *TranscodeOptions) available(systemID string) bool {
	_, ok := t.planFor(systemID, 0)
	return ok
}

// discSheetExts are the members chdman can take as input, in preference
// order (a .cue/.gdi names its tracks; a bare .iso stands alone).
var discSheetExts = []string{".cue", ".gdi", ".iso"}

// dolphinInputExts are GameCube/Wii images dolphin-tool can convert to RVZ.
// .wbfs and .ciso go through an ISO first (see transcode.NeedsISOFirst).
var dolphinInputExts = []string{".iso", ".gcm", ".wbfs", ".ciso", ".gcz", ".wia"}

func inputExtsFor(systemID string) []string {
	if usesRVZ(systemID) {
		return dolphinInputExts
	}
	return discSheetExts
}

func findDiscSheet(systemID string, members []store.ArchiveMemberRecord) (string, bool) {
	for _, ext := range inputExtsFor(systemID) {
		for _, m := range members {
			if strings.EqualFold(filepath.Ext(m.MemberName), ext) {
				return m.MemberName, true
			}
		}
	}
	return "", false
}

// isoScratchBytes is the worst-case size of the ISO a WBFS/CISO expands to
// (a dual-layer Wii disc): observed ~9 GB for a single-layer Wii game.
const isoScratchBytes = 9500 << 20

// scratchNeeded is how much scratch space preparing sheet will take.
func scratchNeeded(systemID, sheet string, extracted int64) int64 {
	if usesRVZ(systemID) && transcode.NeedsISOFirst(sheet) {
		return extracted + isoScratchBytes
	}
	return extracted
}

// prepareInput makes sheet convertible: for dolphin-tool inputs that need
// it, expands them to an ISO in work.
func prepareInput(ctx context.Context, t *TranscodeOptions, systemID, sheet, work string) (string, error) {
	if usesRVZ(systemID) && transcode.NeedsISOFirst(sheet) {
		iso := filepath.Join(work, "disc.iso")
		if err := t.RVZ.ToISO(ctx, sheet, iso); err != nil {
			return "", err
		}
		return iso, nil
	}
	return sheet, nil
}

// transcodeArchive converts the disc in g's archive and links the result.
// The conversion runs synchronously (prewarmConversions runs several at once
// beforehand when the user allows it, so this then finds the cache filled).
func transcodeArchive(db *store.DB, destDir, systemID string, g store.SelectedGame, members []store.ArchiveMemberRecord, opts Options) (string, error) {
	if err := opts.failed[g.GameKey]; err != nil {
		return "", err
	}
	plan, job, err := archiveJob(systemID, g, members, opts)
	if err != nil {
		return "", err
	}
	return convertAndLink(db, destDir, systemID, g, plan, job, opts)
}

// archiveJob prepares the conversion of the disc in g's archive: the tool,
// codecs and a job that extracts it into scratch.
func archiveJob(systemID string, g store.SelectedGame, members []store.ArchiveMemberRecord, opts Options) (discPlan, transcode.Job, error) {
	t := opts.Transcode
	var total int64
	for _, m := range members {
		total += m.Size
	}
	plan, ok := t.planFor(systemID, total)
	if !ok {
		return plan, transcode.Job{}, fmt.Errorf("%s is a disc image and needs transcoding, but no transcode tool is configured", g.DisplayName)
	}
	sheet, ok := findDiscSheet(systemID, members)
	if !ok {
		return plan, transcode.Job{}, fmt.Errorf("no convertible disc image (%s) found in %s", strings.Join(inputExtsFor(systemID), "/"), g.UltimateSourceRef)
	}
	if err := transcode.CheckScratchSpace(t.ScratchDir, scratchNeeded(systemID, sheet, total), 1); err != nil {
		return plan, transcode.Job{}, err
	}

	archive := g.UltimateSourceRef
	job := transcode.Job{
		ID:          g.GameKey,
		ContentHash: g.GameKey, // content-hash-based identity (spec §2.11)
		Media:       plan.media,
		Codecs:      plan.codecs,
		Prepare: func(work string) (string, error) {
			for _, m := range members {
				if err := extractMember(opts.ctx(), opts.SevenZip, archive, m.MemberName, filepath.Join(work, filepath.Base(m.MemberName))); err != nil {
					return "", err
				}
			}
			return prepareInput(opts.ctx(), t, systemID, filepath.Join(work, filepath.Base(sheet)), work)
		},
	}
	return plan, job, nil
}

func convertAndLink(db *store.DB, destDir, systemID string, g store.SelectedGame, plan discPlan, job transcode.Job, opts Options) (string, error) {
	t := opts.Transcode
	entry, err := transcode.Convert(opts.ctx(), t.Cache, plan.tool, t.ScratchDir, job, opts.progressFor(g.DisplayName))
	if err != nil {
		return "", fmt.Errorf("transcode %s: %w", g.DisplayName, err)
	}
	dst := filepath.Join(destDir, cleanFileName(g.DisplayName, entry.Path))
	if err := linkFile(db, systemID, g.GameKey, entry.Path, dst, "transcode_cache", opts); err != nil {
		return "", err
	}
	return dst, nil
}

// mediaForSystem picks chdman's mode (spec §2.10): PSP discs are UMDs and
// always need createdvd regardless of size; elsewhere the image size
// decides (PS2 has CD-based titles).
func mediaForSystem(systemID string, imageBytes int64) transcode.Media {
	if systemID == "psp" {
		return transcode.MediaDVD
	}
	return transcode.DetectMedia(imageBytes)
}

// isRawDiscImage reports whether path is a disc image the system's
// converter takes as input.
func isRawDiscImage(systemID, path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	for _, e := range inputExtsFor(systemID) {
		if ext == e {
			return true
		}
	}
	return false
}

// transcodeRaw converts an unarchived disc image and links the cache
// entry. The converter reads the source in place, so nothing is written
// near the source (it stays read-only).
func transcodeRaw(db *store.DB, destDir, systemID string, g store.SelectedGame, opts Options) (string, error) {
	if err := opts.failed[g.GameKey]; err != nil {
		return "", err
	}
	plan, job, err := rawJob(systemID, g, opts)
	if err != nil {
		return "", err
	}
	return convertAndLink(db, destDir, systemID, g, plan, job, opts)
}

// rawJob prepares the conversion of an unarchived disc image.
func rawJob(systemID string, g store.SelectedGame, opts Options) (discPlan, transcode.Job, error) {
	t := opts.Transcode
	sheet := g.UltimateSourceRef
	total, err := discImageSize(sheet)
	if err != nil {
		return discPlan{}, transcode.Job{}, err
	}
	plan, ok := t.planFor(systemID, total)
	if !ok {
		return plan, transcode.Job{}, fmt.Errorf("%s needs transcoding, but no transcode tool is configured", g.DisplayName)
	}
	if usesRVZ(systemID) && transcode.NeedsISOFirst(sheet) {
		if err := transcode.CheckScratchSpace(t.ScratchDir, scratchNeeded(systemID, sheet, 0), 1); err != nil {
			return plan, transcode.Job{}, err
		}
	}
	job := transcode.Job{
		ID:          g.GameKey,
		ContentHash: g.GameKey,
		Media:       plan.media,
		Codecs:      plan.codecs,
		Prepare: func(work string) (string, error) {
			return prepareInput(opts.ctx(), t, systemID, sheet, work)
		},
	}
	return plan, job, nil
}

// conversionJob returns the conversion g will need when generated, if any.
func conversionJob(db *store.DB, systemID string, g store.SelectedGame, opts Options) (discPlan, transcode.Job, bool) {
	t := opts.Transcode
	if t == nil || !IsDiscSystem(systemID) || !t.available(systemID) {
		return discPlan{}, transcode.Job{}, false
	}
	switch g.UltimateSourceKind {
	case "raw_file":
		if !isRawDiscImage(systemID, g.UltimateSourceRef) {
			return discPlan{}, transcode.Job{}, false
		}
		plan, job, err := rawJob(systemID, g, opts)
		return plan, job, err == nil
	case "archive":
		rec, err := db.GetFileByPath(g.UltimateSourceRef)
		if err != nil || rec == nil {
			return discPlan{}, transcode.Job{}, false
		}
		members, err := db.ListArchiveMembers(rec.ID)
		if err != nil || DecideArchivePolicy(rec.Kind, len(members) == 1, true) != PolicyTranscode {
			return discPlan{}, transcode.Job{}, false
		}
		plan, job, err := archiveJob(systemID, g, members, opts)
		return plan, job, err == nil
	}
	return discPlan{}, transcode.Job{}, false
}

// prewarmConversions converts the discs that need it with up to
// opts.Parallelism conversions at once, so the sequential pass that follows
// finds them in the cache. It returns the games whose conversion failed (the
// sequential pass reports that error instead of trying again). With
// Parallelism below 2 it does nothing.
func prewarmConversions(db *store.DB, systemID string, games []store.SelectedGame, opts Options) map[string]error {
	if opts.Parallelism < 2 || opts.Transcode == nil || opts.Transcode.Cache == nil {
		return nil
	}
	type work struct {
		g    store.SelectedGame
		plan discPlan
		job  transcode.Job
	}
	var todo []work
	for _, g := range games {
		if plan, job, ok := conversionJob(db, systemID, g, opts); ok {
			todo = append(todo, work{g, plan, job})
		}
	}
	if len(todo) < 2 {
		return nil
	}
	var (
		mu     sync.Mutex
		failed = map[string]error{}
		wg     sync.WaitGroup
		slots  = make(chan struct{}, opts.Parallelism)
	)
	for _, w := range todo {
		if opts.ctx().Err() != nil {
			break
		}
		slots <- struct{}{}
		wg.Add(1)
		go func(w work) {
			defer func() { <-slots; wg.Done() }()
			t := opts.Transcode
			if _, err := transcode.Convert(opts.ctx(), t.Cache, w.plan.tool, t.ScratchDir, w.job, opts.progressFor(w.g.DisplayName)); err != nil && opts.ctx().Err() == nil {
				mu.Lock()
				failed[w.g.GameKey] = fmt.Errorf("transcode %s: %w", w.g.DisplayName, err)
				mu.Unlock()
			}
		}(w)
	}
	wg.Wait()
	return failed
}

// discImageSize sums the sheet and every file it references.
func discImageSize(sheet string) (int64, error) {
	st, err := os.Stat(sheet)
	if err != nil {
		return 0, err
	}
	total := st.Size()
	members, err := discsheet.ReferencedFiles(sheet)
	if err != nil {
		return 0, err
	}
	for _, m := range members {
		ms, err := os.Stat(filepath.Join(filepath.Dir(sheet), m))
		if err != nil {
			return 0, fmt.Errorf("disc track %s: %w", m, err)
		}
		total += ms.Size()
	}
	return total, nil
}

// progressFor returns the per-conversion progress callback for item, or nil.
func (o Options) progressFor(item string) func(float64) {
	if o.OnProgress == nil {
		return nil
	}
	return func(p float64) { o.OnProgress(item, p) }
}
