// Package link provides the link primitives (symlink, reflink, hardlink,
// copy) and capability detection used to choose a tier per spec §2.2.
//
// Every write primitive (SymlinkRelative, SymlinkAbsolute, Hardlink,
// Reflink, Copy) fails if dst already exists — callers must remove a stale
// link themselves before replacing it — and refuses to write inside a
// registered source root (see RegisterSourceRoot, guard.go).
package link

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// SymlinkRelative creates a symlink at dst pointing to src, with the link
// target written as a path relative to dst's directory. Only valid when
// src and dst are on the same volume (spec §2.2); the caller is
// responsible for having checked that with fsinfo.SameVolume.
func SymlinkRelative(src, dst string) error {
	if err := guardWritePath(dst); err != nil {
		return err
	}

	absSrc, err := filepath.Abs(src)
	if err != nil {
		return fmt.Errorf("resolve absolute path %s: %w", src, err)
	}
	absDst, err := filepath.Abs(dst)
	if err != nil {
		return fmt.Errorf("resolve absolute path %s: %w", dst, err)
	}

	if err := os.MkdirAll(filepath.Dir(absDst), 0o755); err != nil {
		return fmt.Errorf("create parent dir for %s: %w", dst, err)
	}
	// ".." in the link is followed on the real directory tree, so the relative path
	// must be worked out between the real locations: a folder reached through a
	// symlink (macOS /var -> /private/var, a linked output folder) would otherwise
	// give a link that is one or more levels short.
	rel, err := filepath.Rel(realDir(filepath.Dir(absDst)), filepath.Join(realDir(filepath.Dir(absSrc)), filepath.Base(absSrc)))
	if err != nil {
		return fmt.Errorf("compute path from %s to %s: %w", dst, src, err)
	}
	if err := os.Symlink(rel, absDst); err != nil {
		return fmt.Errorf("symlink %s -> %s (relative): %w", dst, rel, err)
	}
	return nil
}

// realDir resolves symlinks in dir; if that is not possible it is returned as it is.
func realDir(dir string) string {
	if r, err := filepath.EvalSymlinks(dir); err == nil {
		return r
	}
	return dir
}

// SymlinkAbsolute creates a symlink at dst pointing to the absolute path of
// src. Used across volumes, where a relative target is impossible on
// Windows and fragile elsewhere (spec §2.2).
func SymlinkAbsolute(src, dst string) error {
	if err := guardWritePath(dst); err != nil {
		return err
	}

	absSrc, err := filepath.Abs(src)
	if err != nil {
		return fmt.Errorf("resolve absolute path %s: %w", src, err)
	}
	absDst, err := filepath.Abs(dst)
	if err != nil {
		return fmt.Errorf("resolve absolute path %s: %w", dst, err)
	}

	if err := os.MkdirAll(filepath.Dir(absDst), 0o755); err != nil {
		return fmt.Errorf("create parent dir for %s: %w", dst, err)
	}
	if err := os.Symlink(absSrc, absDst); err != nil {
		return fmt.Errorf("symlink %s -> %s (absolute): %w", dst, absSrc, err)
	}
	return nil
}

// Hardlink creates dst as a hardlink to src. Same volume only; dst *is*
// src's data afterward, so this tier is opt-in and must never be chosen for
// a target known to write in place (spec §2.2).
func Hardlink(src, dst string) error {
	if err := guardWritePath(dst); err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return fmt.Errorf("create parent dir for %s: %w", dst, err)
	}
	if err := os.Link(src, dst); err != nil {
		return fmt.Errorf("hardlink %s -> %s: %w", dst, src, err)
	}
	return nil
}

// Reflink creates dst as a copy-on-write clone of src: an independent file
// that shares storage with src until either is written to. Same
// CoW-capable volume only (spec §2.2); implemented per-OS in
// reflink_<os>.go.
func Reflink(src, dst string) error {
	if err := guardWritePath(dst); err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return fmt.Errorf("create parent dir for %s: %w", dst, err)
	}
	if err := reflink(src, dst); err != nil {
		return fmt.Errorf("reflink %s -> %s: %w", dst, src, err)
	}
	return nil
}

// Copy copies src to dst. Always available, at the cost of real disk usage
// (spec §2.2 tier 4); used for FAT32/exFAT device export (spec §2.4) and as
// the universal fallback. Writes to a temp file in dst's directory first
// and renames into place, so a failed or interrupted copy never leaves a
// partial file at dst.
func Copy(src, dst string) error {
	if err := guardWritePath(dst); err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return fmt.Errorf("create parent dir for %s: %w", dst, err)
	}

	in, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("open %s: %w", src, err)
	}
	defer in.Close()

	tmp := dst + ".romcure-tmp"
	out, err := os.OpenFile(tmp, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("create %s: %w", tmp, err)
	}

	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		os.Remove(tmp)
		return fmt.Errorf("copy %s -> %s: %w", src, tmp, err)
	}
	if err := out.Close(); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("close %s: %w", tmp, err)
	}
	if err := os.Rename(tmp, dst); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("rename %s -> %s: %w", tmp, dst, err)
	}
	return nil
}
