package profile

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"

	"romcure/internal/discsheet"
	"romcure/internal/link"
	"romcure/internal/store"
)

// Putting one game in an output: from a plain file, out of an archive, or
// through a conversion, by the best link method the folders allow.

func linkOneGame(db *store.DB, destDir, systemID string, g store.SelectedGame, opts Options) (string, error) {
	dst, err := linkOneGameFile(db, destDir, systemID, g, opts)
	if err != nil {
		return "", err
	}
	if err := linkSBI(db, systemID, g, dst, opts); err != nil {
		return "", err
	}
	return dst, nil
}

func linkOneGameFile(db *store.DB, destDir, systemID string, g store.SelectedGame, opts Options) (string, error) {
	switch g.UltimateSourceKind {
	case "raw_file":
		if t := opts.Transcode; t != nil && IsDiscSystem(systemID) && isRawDiscImage(systemID, g.UltimateSourceRef) && t.available(systemID) {
			return transcodeRaw(db, destDir, systemID, g, opts)
		}
		dst := filepath.Join(destDir, cleanFileName(g.DisplayName, g.UltimateSourceRef))
		if err := linkFile(db, systemID, g.GameKey, g.UltimateSourceRef, dst, "raw_file", opts); err != nil {
			return "", err
		}
		if err := linkMultiFileMembers(db, systemID, g.GameKey, g.UltimateSourceRef, dst, opts); err != nil {
			return "", err
		}
		return dst, nil

	case "archive":
		return linkArchiveMember(db, destDir, systemID, g, opts)

	default:
		return "", fmt.Errorf("unsupported ultimate_source_kind %q", g.UltimateSourceKind)
	}
}

// linkMultiFileMembers links every file a .cue or .gdi sheet at srcPath
// references, with its exact original name, into dstSheetPath's
// directory (spec §2.5). A no-op for any other extension.
func linkMultiFileMembers(db *store.DB, systemID, gameKey, srcPath, dstSheetPath string, opts Options) error {
	members, err := discsheet.ReferencedFiles(srcPath)
	if err != nil {
		return fmt.Errorf("read member list from %s: %w", srcPath, err)
	}

	srcDir := filepath.Dir(srcPath)
	dstDir := filepath.Dir(dstSheetPath)
	for _, member := range members {
		memberSrc := filepath.Join(srcDir, member)
		memberDst := filepath.Join(dstDir, member)
		if err := linkFile(db, systemID, gameKey, memberSrc, memberDst, "raw_file", opts); err != nil {
			return fmt.Errorf("link member %s: %w", member, err)
		}
	}
	return nil
}

func linkArchiveMember(db *store.DB, destDir, systemID string, g store.SelectedGame, opts Options) (string, error) {
	archiveRec, err := db.GetFileByPath(g.UltimateSourceRef)
	if err != nil {
		return "", fmt.Errorf("look up archive %s: %w", g.UltimateSourceRef, err)
	}
	if archiveRec == nil {
		return "", fmt.Errorf("archive %s is not in the hash cache (re-scan needed?)", g.UltimateSourceRef)
	}
	members, err := db.ListArchiveMembers(archiveRec.ID)
	if err != nil {
		return "", fmt.Errorf("list members of %s: %w", g.UltimateSourceRef, err)
	}

	policy := archivePolicyFor(*archiveRec, len(members) == 1, systemID)

	switch policy {
	case PolicyPassthrough:
		dst := filepath.Join(destDir, cleanFileName(g.DisplayName, g.UltimateSourceRef))
		if err := linkFile(db, systemID, g.GameKey, g.UltimateSourceRef, dst, "archive", opts); err != nil {
			return "", err
		}
		return dst, nil

	case PolicyExtract:
		if opts.ExtractCacheDir == "" {
			return "", fmt.Errorf("%s needs extracting from %s but ExtractCacheDir is unset", g.MemberName, g.UltimateSourceRef)
		}
		cachePath := filepath.Join(opts.ExtractCacheDir, g.GameKey+filepath.Ext(g.MemberName))
		if _, err := os.Stat(cachePath); err != nil {
			if err := extractMember(opts.ctx(), opts.SevenZip, g.UltimateSourceRef, g.MemberName, cachePath); err != nil {
				return "", fmt.Errorf("extract %s from %s: %w", g.MemberName, g.UltimateSourceRef, err)
			}
		}
		dst := filepath.Join(destDir, cleanFileName(g.DisplayName, g.MemberName))
		if err := linkFile(db, systemID, g.GameKey, cachePath, dst, "archive_member_extracted", opts); err != nil {
			return "", err
		}
		return dst, nil

	default: // PolicyTranscode
		return transcodeArchive(db, destDir, systemID, g, members, opts)
	}
}

// linkFile links src to dst using the best available tier (spec §2.2),
// records the outcome in link_state, and does nothing if dst already
// exists (see GenerateRetroArch's doc comment on incremental re-sync).
func linkFile(db *store.DB, systemID, gameKey, src, dst, sourceKind string, opts Options) error {
	if li, err := os.Lstat(dst); err == nil {
		// An existing output is kept only if it is still what we want. It is
		// replaced when it was ours but now stands for a different source
		// (the 1G1R pick changed, e.g. after switching region preset, while
		// the clean output name stayed the same), or when it is a copy that
		// no longer matches its source. A file we never recorded is left
		// alone: it may be the user's.
		replace := false
		if rel, rerr := filepath.Rel(opts.OutputRoot, dst); rerr == nil {
			if rec, _ := db.GetLinkState(profileIDOf(opts), rel); rec != nil && !sameFile(rec.SourcePath, src) {
				replace = true
			}
		}
		if !replace && opts.ForceCopy && !copyIsCurrent(src, dst) {
			replace = true
		}
		if !replace {
			return nil
		}
		if li.Mode()&os.ModeSymlink == 0 && !li.Mode().IsRegular() {
			return fmt.Errorf("%s is in the way and is not a file or link", dst)
		}
		if err := link.GuardWrite(dst); err != nil {
			return err
		}
		if err := os.Remove(dst); err != nil {
			return fmt.Errorf("replace outdated output %s: %w", dst, err)
		}
	}

	// The destination directory must exist before probing (link.Capabilities
	// checks volumes and writes probe files there); macOS and Linux fail the
	// volume check on a missing directory, Windows happens not to.
	if err := link.GuardWrite(filepath.Dir(dst)); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return fmt.Errorf("create %s: %w", filepath.Dir(dst), err)
	}

	var caps link.TierCapabilities
	var sel link.Selection
	if opts.ForceCopy {
		// Export to a device: never probe link support (the target is often
		// FAT32/exFAT) and refuse files the target filesystem can't hold.
		if st, err := os.Stat(src); err == nil && opts.MaxFileBytes > 0 && st.Size() > opts.MaxFileBytes {
			return fmt.Errorf("%s is %d bytes, over the %d-byte file limit of the destination filesystem", filepath.Base(src), st.Size(), opts.MaxFileBytes)
		}
		sel = link.Selection{Tier: link.TierCopy, Reason: "export profile: files are copied"}
	} else {
		var err error
		caps, err = link.Capabilities(src, dst)
		if err != nil {
			return fmt.Errorf("probe link capabilities for %s: %w", dst, err)
		}
		sel = link.SelectTier(caps, link.Preferences{AllowHardlink: opts.AllowHardlink})
	}

	var linkErr error
	var linkType string
	switch sel.Tier {
	case link.TierSymlink:
		if caps.SameVolume {
			linkType, linkErr = "symlink_rel", link.SymlinkRelative(src, dst)
		} else {
			linkType, linkErr = "symlink_abs", link.SymlinkAbsolute(src, dst)
		}
	case link.TierReflink:
		linkType, linkErr = "reflink", link.Reflink(src, dst)
	case link.TierHardlink:
		linkType, linkErr = "hardlink", link.Hardlink(src, dst)
	default:
		linkType, linkErr = "copy", link.Copy(src, dst)
	}
	if linkErr != nil {
		return fmt.Errorf("%s: %w", sel.Reason, linkErr)
	}

	sourceHash := ""
	if srcRec, err := db.GetFileByPath(src); err == nil && srcRec != nil && srcRec.CRC32.Valid {
		sourceHash = srcRec.CRC32.String
	}

	// target_path is relative to the profile output root (spec §2.7), so
	// it stays valid if the whole profile output is moved.
	relTarget, err := filepath.Rel(opts.OutputRoot, dst)
	if err != nil {
		return fmt.Errorf("compute target path for %s relative to %s: %w", dst, opts.OutputRoot, err)
	}

	return db.UpsertLinkState(store.LinkStateRecord{
		ProfileID: profileIDOf(opts), SystemID: systemID, GameKey: gameKey, TargetPath: relTarget,
		SourceKind: sourceKind, SourcePath: src, SourceHash: sourceHash,
		LinkType: linkType, LinkStatus: "ok",
	})
}

var invalidFileNameChars = regexp.MustCompile(`[<>:"/\\|?*\x00-\x1f]`)

// cleanFileName builds the linked file's name: displayName (sanitized
// for filesystem safety) with originalPath's extension.
func cleanFileName(displayName, originalPath string) string {
	ext := filepath.Ext(originalPath)
	return invalidFileNameChars.ReplaceAllString(displayName, "_") + ext
}

// copyIsCurrent reports whether dst is a plain file with the same size as
// src: good enough to skip re-copying on an incremental export.
func copyIsCurrent(src, dst string) bool {
	s, err1 := os.Stat(src)
	d, err2 := os.Stat(dst)
	return err1 == nil && err2 == nil && d.Mode().IsRegular() && s.Size() == d.Size()
}

// sameFile compares two paths the way the filesystem does: case-insensitively
// on Windows and macOS.
func sameFile(a, b string) bool {
	a, b = filepath.Clean(a), filepath.Clean(b)
	if runtime.GOOS == "windows" || runtime.GOOS == "darwin" {
		return strings.EqualFold(a, b)
	}
	return a == b
}
