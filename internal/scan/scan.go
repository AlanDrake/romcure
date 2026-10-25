// Package scan walks source ROM directories, reads archive member CRCs
// without decompressing, and maintains the hash cache. See spec §2, §2.8.
package scan

import (
	"context"
	"database/sql"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"

	"romcure/internal/discid"
	"romcure/internal/rararc"
	"romcure/internal/store"
)

// Progress reports cumulative scan state after each file. The same value
// is never mutated after being passed to onProgress; each call gets its
// own copy.
type Progress struct {
	FilesScanned int
	BytesScanned int64
	CurrentPath  string
}

// Scanner walks a source root and maintains its hash cache in db (the
// files and archive_members tables).
type Scanner struct {
	db *store.DB
	// Problems lists the files the last Scan could not read (damaged
	// archives, unreadable folders). They are skipped, not fatal: one bad
	// file must not stop the scan of the other thousands.
	Problems []Problem
	// SevenZip is the path of the bundled 7-Zip, which reads RAR archives. Empty =
	// RAR files are reported as unreadable.
	SevenZip string

	// discPending holds files scanned before disc headers were recorded, whose
	// header is read on the next scan without rehashing them.
	discPending map[string]bool
}

// Problem is one file or folder a scan had to skip.
type Problem struct {
	Path   string
	Reason string
}

// New returns a Scanner backed by db.
func New(db *store.DB) *Scanner {
	return &Scanner{db: db}
}

// Scan walks root recursively and updates the hash cache for every
// regular file found: raw files get a whole-file CRC32; zip/7z archives
// get their member list with per-member CRC32 read from the archive's own
// headers, without decompressing (spec §2.8). Symlinks are not followed.
//
// onProgress (may be nil) is called after each file. ctx cancellation is
// checked between files; a cancelled or otherwise interrupted scan can
// simply be re-run — already-processed, unchanged files are skipped via
// the (path, size, mtime) cache in the files table, so there is no
// separate resume state to manage (spec §5's resumability rule).
func (s *Scanner) Scan(ctx context.Context, root string, onProgress func(Progress)) (err error) {
	var progress Progress
	seen := map[string]bool{}
	s.Problems = nil
	if s.discPending, err = s.db.DiscHeadersPending(); err != nil {
		return err
	}
	zips, err := s.db.ZipMethodPending()
	if err != nil {
		return err
	}
	for _, p := range zips {
		s.discPending[p] = true // one pass reads whatever header info is missing
	}
	skip := func(path string, err error) {
		s.Problems = append(s.Problems, Problem{Path: path, Reason: reasonOf(path, err)})
	}

	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
		if err != nil {
			if path == root {
				return fmt.Errorf("walk %s: %w", path, err) // nothing can be scanned
			}
			skip(path, err)
			if d != nil && d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			return nil
		}

		info, err := d.Info()
		if err != nil {
			skip(path, err)
			return nil
		}
		if info.Mode()&fs.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return nil
		}
		// Later volumes of a multi-part RAR are read through the first one.
		if rararc.IsContinuation(path) {
			return nil
		}

		// Name the file before reading it: hashing a big image or archive can take a while.
		if onProgress != nil {
			starting := progress
			starting.CurrentPath = path
			onProgress(starting)
		}

		// A file that cannot be read is left out of `seen`, so any older
		// record of it (from when it was fine) is forgotten below.
		if err := s.scanFileSafely(path, info); err != nil {
			skip(path, err)
			return nil
		}
		seen[path] = true

		progress.FilesScanned++
		progress.BytesScanned += info.Size()
		progress.CurrentPath = path
		if onProgress != nil {
			onProgress(progress)
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("scan %s: %w", root, err)
	}

	// Forget files that were deleted since the last scan. Only reached after a
	// complete walk, so an unreadable or unmounted root never wipes the cache.
	known, err := s.db.ListFilesUnderRoot(root)
	if err != nil {
		return fmt.Errorf("scan %s: list known files: %w", root, err)
	}
	for _, f := range known {
		if !seen[f.Path] {
			if err := s.db.DeleteFile(f.ID); err != nil {
				return fmt.Errorf("scan %s: forget %s: %w", root, f.Path, err)
			}
		}
	}
	return nil
}

// scanFileSafely is scanFile, turning a panic inside an archive reader into an
// error: a damaged file must never crash the app.
func (s *Scanner) scanFileSafely(path string, info fs.FileInfo) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("reader crashed: %v", r)
		}
	}()
	return s.scanFile(path, info)
}

// reasonOf explains in a sentence why a file was skipped, without repeating
// its path (the caller shows that).
func reasonOf(path string, err error) string {
	msg := strings.ReplaceAll(err.Error(), path+": ", "")
	msg = strings.TrimPrefix(strings.TrimPrefix(strings.TrimPrefix(msg, "open 7z "), "open zip "), "open rar ")
	switch classify(path) {
	case "zip", "sevenzip", "rar":
		return "damaged or unreadable archive (" + msg + ")"
	}
	return msg
}

func (s *Scanner) scanFile(path string, info fs.FileInfo) error {
	size := info.Size()
	mtime := info.ModTime().Unix()

	cached, err := s.db.GetFileByPath(path)
	if err != nil {
		return err
	}
	// A changed kind (a format the scanner learned to read since, e.g. RAR) is a
	// miss too: the old row holds no members.
	if cached != nil && cached.Size == size && cached.MtimeUnix == mtime && cached.Kind == classify(path) {
		if s.discPending[path] {
			return s.fillDiscHeaders(*cached)
		}
		return nil // unchanged since the last scan; cache hit
	}

	kind := classify(path)
	rec := store.FileRecord{Path: path, Size: size, MtimeUnix: mtime, Kind: kind}

	var members []store.ArchiveMemberRecord
	switch kind {
	case "raw":
		sum, alt, err := hashFile(path, size)
		if err != nil {
			return err
		}
		rec.CRC32.String, rec.CRC32.Valid = sum, true
		rec.AltCRC32, rec.AltSize = alt.CRC32, alt.Size
		if discid.MayHaveAny(path) {
			info, err := readDiscFile(path)
			if err != nil {
				return err
			}
			rec.DiscID, rec.DiscTitle, rec.DiscNumber = sql.NullString{String: info.ID, Valid: true}, info.Title, info.Disc
		}
	case "zip":
		var oldMethod string
		members, oldMethod, err = readZipMembers(path)
		if err != nil {
			return err
		}
		rec.ZipMethod = sql.NullString{String: oldMethod, Valid: true}
	case "sevenzip":
		members, err = readSevenZipMembers(path)
		if err != nil {
			return err
		}
	case "rar":
		members, err = s.readRARMembers(path)
		if err != nil {
			return err
		}
	}
	if len(members) == 1 {
		members[0].IsOnlyMember = true
	}
	if kind != "raw" {
		if err := s.addDiscHeaders(path, kind, members); err != nil {
			return err
		}
	}

	fileID, err := s.db.UpsertFile(rec)
	if err != nil {
		return err
	}

	if kind == "zip" || kind == "sevenzip" || kind == "rar" {
		if err := s.db.ReplaceArchiveMembers(fileID, members); err != nil {
			return err
		}
	}
	return nil
}

// classify determines a file's archive kind from its extension. Deciding
// by extension (not content sniffing) matches how No-Intro/Redump/TOSEC
// sets are actually distributed, and keeps scanning cheap.
func classify(path string) string {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".zip":
		return "zip"
	case ".7z":
		return "sevenzip"
	case ".rar":
		return "rar"
	default:
		return "raw"
	}
}
