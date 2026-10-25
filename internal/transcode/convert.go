package transcode

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"romcure/internal/fsinfo"
	"romcure/internal/link"
)

// ErrInsufficientSpace is returned before any work starts when the scratch
// volume can't hold the uncompressed image (spec §2.10).
var ErrInsufficientSpace = errors.New("not enough free space in scratch directory")

// CheckScratchSpace verifies dir's volume has room for parallel jobs of
// the largest uncompressed image (spec §2.10: largest × parallelism).
func CheckScratchSpace(dir string, largestImageBytes int64, parallel int) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create scratch dir %s: %w", dir, err)
	}
	info, err := fsinfo.Detect(dir)
	if err != nil {
		return fmt.Errorf("check free space for %s: %w", dir, err)
	}
	need := uint64(largestImageBytes) * uint64(max(parallel, 1))
	if info.FreeBytes < need {
		return fmt.Errorf("%w: need %d bytes, %s has %d", ErrInsufficientSpace, need, dir, info.FreeBytes)
	}
	return nil
}

// Job converts one disc image (to CHD, or RVZ for GameCube/Wii).
type Job struct {
	ID           string
	ContentHash  string // hash of the uncompressed disc content (cache key input)
	ExpectedSHA1 string // optional raw SHA1 from the DAT; checked against the CHD
	Media        Media
	Codecs       CodecProfile
	// Prepare makes the input available on disk (e.g. extracts an archive
	// into scratch) and returns the .cue/.gdi/.iso path. It must not write
	// inside a source root.
	Prepare func(scratchDir string) (inputPath string, err error)
}

// Convert is the full pipeline: cache lookup → prepare → create → verify →
// SHA1 check → publish into the cache. Nothing is linkable until verified.
func Convert(ctx context.Context, cache *Cache, tool DiscTool, scratch string, j Job, onProgress func(float64)) (*CacheEntry, error) {
	key := CacheKey(j.ContentHash, tool.OutputFormat(), tool.ToolVersion(), j.Codecs.Flags)
	if hit, err := cache.Lookup(key); err != nil || hit != nil {
		return hit, err
	}

	if err := os.MkdirAll(scratch, 0o755); err != nil {
		return nil, fmt.Errorf("create scratch dir: %w", err)
	}
	work, err := os.MkdirTemp(scratch, "job-")
	if err != nil {
		return nil, fmt.Errorf("create scratch dir: %w", err)
	}
	defer os.RemoveAll(work)

	input, err := j.Prepare(work)
	if err != nil {
		return nil, fmt.Errorf("prepare input: %w", err)
	}

	final := cache.PathFor(key, tool.OutputFormat())
	if err := os.MkdirAll(filepath.Dir(final), 0o755); err != nil {
		return nil, err
	}
	// Convert beside the final path so the publish is an atomic rename on
	// the same volume; a crash leaves only a .partial, never a bad entry.
	partial := final + ".partial"
	defer os.Remove(partial)
	if err := link.GuardWrite(partial); err != nil {
		return nil, err
	}

	if onProgress == nil {
		onProgress = func(float64) {}
	}
	if err := tool.Convert(ctx, j, input, partial, onProgress); err != nil {
		return nil, err
	}
	if err := tool.Verify(ctx, partial); err != nil {
		return nil, fmt.Errorf("verify failed: %w", err)
	}
	if j.ExpectedSHA1 != "" {
		got, err := tool.RawSHA1(ctx, partial)
		if err != nil {
			return nil, err
		}
		if !strings.EqualFold(got, j.ExpectedSHA1) {
			return nil, fmt.Errorf("CHD raw SHA1 %s does not match DAT %s", got, j.ExpectedSHA1)
		}
	}
	if err := os.Rename(partial, final); err != nil {
		return nil, fmt.Errorf("publish %s: %w", final, err)
	}

	entry := CacheEntry{
		Key: key, ContentHash: j.ContentHash, Format: tool.OutputFormat(), ToolName: tool.Name(),
		ToolVersion: tool.ToolVersion(), CodecFlags: strings.Join(j.Codecs.Flags, " "),
		Path: final, Verified: true,
	}
	if err := cache.Put(entry); err != nil {
		return nil, err
	}
	return &entry, nil
}
