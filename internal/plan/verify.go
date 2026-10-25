package plan

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"romcure/internal/store"
)

// Finding is the verification verdict for one link_state row.
type Finding struct {
	SystemID   string
	TargetPath string // relative to the profile output root
	SourcePath string
	Status     string // "ok", "broken" or "stale"
	Reason     string // why, in words; "" for ok
}

// Summary counts findings by status.
type Summary struct{ OK, Broken, Stale int }

func Summarize(fs []Finding) Summary {
	var s Summary
	for _, f := range fs {
		switch f.Status {
		case "ok":
			s.OK++
		case "broken":
			s.Broken++
		default:
			s.Stale++
		}
	}
	return s
}

// VerifyProfile checks every recorded output of profileID under outputRoot
// against the filesystem and updates link_state.link_status. It only reads
// the filesystem: use Repair to remove what it flagged.
//
//	broken: the output is gone, a link dangles, or its source is missing.
//	stale:  the output exists but no longer matches what was recorded: a link
//	        pointing somewhere else, a copy whose size differs from the source,
//	        or a source that changed since linking (its hash moved).
func VerifyProfile(db *store.DB, profileID, outputRoot string) ([]Finding, error) {
	rows, err := db.ListLinkStates(profileID)
	if err != nil {
		return nil, err
	}
	findings := make([]Finding, 0, len(rows))
	for _, r := range rows {
		f := verifyOne(db, r, outputRoot)
		if err := db.SetLinkStatus(profileID, r.TargetPath, f.Status); err != nil {
			return nil, err
		}
		findings = append(findings, f)
	}
	return findings, nil
}

func verifyOne(db *store.DB, r store.LinkStateRecord, outputRoot string) Finding {
	f := Finding{SystemID: r.SystemID, TargetPath: r.TargetPath, SourcePath: r.SourcePath, Status: "ok"}
	fail := func(status, format string, a ...any) Finding {
		f.Status, f.Reason = status, fmt.Sprintf(format, a...)
		return f
	}

	target := filepath.Join(outputRoot, r.TargetPath)
	li, err := os.Lstat(target)
	if err != nil {
		return fail("broken", "output is missing")
	}
	if r.SourceKind == "generated" {
		// A file the app wrote itself (a playlist): there is no source to compare.
		if !li.Mode().IsRegular() {
			return fail("stale", "expected a file the app generated, found something else")
		}
		return f
	}

	if strings.HasPrefix(r.LinkType, "symlink") {
		if li.Mode()&os.ModeSymlink == 0 {
			return fail("stale", "recorded as a symlink but is now a regular file")
		}
		dest, err := os.Readlink(target)
		if err != nil {
			return fail("broken", "cannot read link: %v", err)
		}
		if !filepath.IsAbs(dest) {
			dest = filepath.Join(filepath.Dir(target), dest)
		}
		if !samePath(dest, r.SourcePath) {
			return fail("stale", "points at %s, expected %s", dest, r.SourcePath)
		}
	}

	src, err := os.Stat(r.SourcePath)
	if err != nil {
		return fail("broken", "source is missing")
	}

	switch r.LinkType {
	case "copy", "reflink", "hardlink":
		ti, err := os.Stat(target)
		if err != nil || !ti.Mode().IsRegular() {
			return fail("broken", "output is not a readable file")
		}
		if ti.Size() != src.Size() {
			return fail("stale", "output is %d bytes but the source is %d", ti.Size(), src.Size())
		}
	default:
		if _, err := os.Stat(target); err != nil {
			return fail("broken", "link does not resolve")
		}
	}

	// The scanner refreshes files.crc32 when a source changes; a different
	// value than the one recorded at link time means the link points at
	// content that is no longer what was linked.
	if r.SourceHash != "" {
		if rec, err := db.GetFileByPath(r.SourcePath); err == nil && rec != nil && rec.CRC32.Valid &&
			!strings.EqualFold(rec.CRC32.String, r.SourceHash) {
			return fail("stale", "source changed since linking (hash %s, was %s)", rec.CRC32.String, r.SourceHash)
		}
	}
	return f
}

func samePath(a, b string) bool {
	a, b = filepath.Clean(a), filepath.Clean(b)
	if runtime.GOOS == "windows" || runtime.GOOS == "darwin" {
		return strings.EqualFold(a, b)
	}
	return a == b
}

// Repair removes the outputs findings flagged as broken or stale so the
// next sync recreates them. It never touches sources: it deletes only
// symlinks, or regular files that are copies, strictly inside outputRoot.
// Returns how many outputs were removed.
func Repair(outputRoot string, findings []Finding) (int, error) {
	root, err := filepath.Abs(outputRoot)
	if err != nil {
		return 0, err
	}
	removed := 0
	for _, f := range findings {
		if f.Status == "ok" {
			continue
		}
		target := filepath.Join(root, f.TargetPath)
		if rel, err := filepath.Rel(root, target); err != nil || strings.HasPrefix(rel, "..") || rel == "." {
			return removed, fmt.Errorf("refusing to remove %s: outside %s", target, root)
		}
		if samePath(target, f.SourcePath) {
			return removed, fmt.Errorf("refusing to remove %s: it is the source itself", target)
		}
		li, err := os.Lstat(target)
		if err != nil {
			continue // already gone
		}
		if li.Mode()&os.ModeSymlink == 0 && !li.Mode().IsRegular() {
			continue
		}
		if err := os.Remove(target); err != nil {
			return removed, fmt.Errorf("remove %s: %w", target, err)
		}
		removed++
	}
	return removed, nil
}
