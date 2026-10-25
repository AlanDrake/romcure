// Package chdverify computes the per-track hashes of a CHD so it can be
// matched against Redump DATs. A CHD header only carries whole-disc hashes,
// so the disc is extracted to scratch, each track hashed, and the scratch
// copy deleted. Slow (a full read and write of the disc), hence opt-in.
package chdverify

import (
	"context"
	"crypto/md5"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"hash/crc32"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"romcure/internal/link"
	"romcure/internal/store"
	"romcure/internal/transcode"
)

// Verifier extracts and hashes CHDs.
type Verifier struct {
	DB         *store.DB
	Tool       *transcode.Chdman
	ScratchDir string // should be on a fast volume with room for one full disc
}

var trackMeta = regexp.MustCompile(`(?m)^\s*Metadata:.*Tag='(CHT2|CHTR|CHGD|CHGT)'`)

// IsCD reports whether `chdman info` output describes a CD (track
// metadata) rather than a DVD/hard-disk image.
func IsCD(info string) bool { return trackMeta.MatchString(info) }

// Verify hashes the tracks of the CHD recorded as rec and stores them. It
// is a no-op when the stored hashes are still current. The CHD is only
// read; all writes go to ScratchDir.
func (v *Verifier) Verify(ctx context.Context, rec store.FileRecord) error {
	if existing, err := v.DB.CHDTracks(rec); err != nil {
		return err
	} else if existing != nil {
		return nil
	}
	if err := link.GuardWrite(v.ScratchDir); err != nil {
		return err
	}
	if err := os.MkdirAll(v.ScratchDir, 0o755); err != nil {
		return err
	}
	work, err := os.MkdirTemp(v.ScratchDir, "chdverify-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(work)

	info, err := v.Tool.Info(ctx, rec.Path)
	if err != nil {
		return err
	}

	var paths []string
	if IsCD(info) {
		cue := filepath.Join(work, "disc.cue")
		if err := v.Tool.ExtractCD(ctx, rec.Path, cue, filepath.Join(work, "track%t.bin")); err != nil {
			return err
		}
		paths, err = filepath.Glob(filepath.Join(work, "*.bin"))
		if err != nil {
			return err
		}
		sort.Strings(paths)
	} else {
		iso := filepath.Join(work, "disc.iso")
		if err := v.Tool.ExtractDVD(ctx, rec.Path, iso); err != nil {
			return err
		}
		paths = []string{iso}
	}
	if len(paths) == 0 {
		return fmt.Errorf("extracting %s produced no tracks", rec.Path)
	}

	tracks := make([]store.CHDTrack, 0, len(paths))
	for i, p := range paths {
		t, err := hashFile(p)
		if err != nil {
			return err
		}
		t.Ordinal = i + 1
		t.Name = strings.TrimSuffix(filepath.Base(rec.Path), filepath.Ext(rec.Path)) + trackSuffix(i, len(paths)) + filepath.Ext(p)
		tracks = append(tracks, t)
	}
	return v.DB.ReplaceCHDTracks(rec.ID, rec.Size, rec.MtimeUnix, tracks)
}

func trackSuffix(i, n int) string {
	if n == 1 {
		return ""
	}
	return fmt.Sprintf(" (Track %d)", i+1)
}

func hashFile(path string) (store.CHDTrack, error) {
	f, err := os.Open(path)
	if err != nil {
		return store.CHDTrack{}, err
	}
	defer f.Close()
	c, s, m := crc32.NewIEEE(), sha1.New(), md5.New()
	n, err := io.Copy(io.MultiWriter(c, s, m), f)
	if err != nil {
		return store.CHDTrack{}, err
	}
	return store.CHDTrack{
		Size:  n,
		CRC32: fmt.Sprintf("%08x", c.Sum32()),
		SHA1:  hex.EncodeToString(s.Sum(nil)),
		MD5:   hex.EncodeToString(m.Sum(nil)),
	}, nil
}

// VerifyUnder verifies every .chd the scanner recorded under root, reporting
// per-file failures through onError instead of stopping the batch.
func (v *Verifier) VerifyUnder(ctx context.Context, root string, onProgress func(done, total int, path string), onError func(path string, err error)) error {
	files, err := v.DB.ListFilesUnderRoot(root)
	if err != nil {
		return err
	}
	var chds []store.FileRecord
	for _, f := range files {
		if strings.EqualFold(filepath.Ext(f.Path), ".chd") {
			chds = append(chds, f)
		}
	}
	for i, f := range chds {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if onProgress != nil {
			onProgress(i, len(chds), f.Path)
		}
		if err := v.Verify(ctx, f); err != nil && onError != nil {
			onError(f.Path, err)
		}
	}
	return nil
}
