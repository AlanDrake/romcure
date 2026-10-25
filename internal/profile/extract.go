package profile

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"archive/zip"

	"github.com/bodgit/sevenzip"

	"romcure/internal/rararc"
	"romcure/internal/store"
)

// extractMember extracts memberName from archivePath into destPath,
// writing to a temp file first and renaming into place so a failed or
// interrupted extraction never leaves a partial file at destPath. Used
// by the Extract archive policy (spec §2.8): RetroArch generally can't
// read 7z, and can't passthrough a zip holding more than the one ROM.
func extractMember(ctx context.Context, sevenZip, archivePath, memberName, destPath string) error {
	switch strings.ToLower(filepath.Ext(archivePath)) {
	case ".zip":
		err := extractWith(ctx, archivePath, memberName, destPath, openZipMember)
		if errors.Is(err, zip.ErrAlgorithm) {
			return extractZipMemberWithTool(ctx, sevenZip, archivePath, memberName, destPath)
		}
		return err
	case ".7z":
		return extractWith(ctx, archivePath, memberName, destPath, openSevenZipMember)
	case ".rar":
		return rararc.Tool{Path: sevenZip}.Extract(ctx, archivePath, memberName, destPath)
	default:
		return fmt.Errorf("don't know how to extract from %s", archivePath)
	}
}

// opener opens one member of an archive for reading, plus a closer for
// the archive handle itself (the member's own Close is the caller's
// responsibility via the returned io.ReadCloser).
type opener func(archivePath, memberName string) (io.ReadCloser, func() error, error)

func extractWith(ctx context.Context, archivePath, memberName, destPath string, open opener) error {
	src, closeArchive, err := open(archivePath, memberName)
	if err != nil {
		return err
	}
	defer closeArchive()
	defer src.Close()

	if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
		return fmt.Errorf("create dir for %s: %w", destPath, err)
	}

	tmp := destPath + ".romcure-tmp"
	out, err := os.OpenFile(tmp, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("create %s: %w", tmp, err)
	}

	if _, err := io.Copy(out, ctxReader{ctx, src}); err != nil {
		out.Close()
		os.Remove(tmp)
		return fmt.Errorf("extract %s -> %s: %w", memberName, tmp, err)
	}
	if err := out.Close(); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("close %s: %w", tmp, err)
	}
	if err := os.Rename(tmp, destPath); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("rename %s -> %s: %w", tmp, destPath, err)
	}
	return nil
}

// ctxReader stops a long copy when its context is cancelled.
type ctxReader struct {
	ctx context.Context
	r   io.Reader
}

func (c ctxReader) Read(p []byte) (int, error) {
	if err := c.ctx.Err(); err != nil {
		return 0, err
	}
	return c.r.Read(p)
}

func openZipMember(archivePath, memberName string) (io.ReadCloser, func() error, error) {
	zr, err := zip.OpenReader(archivePath)
	if err != nil {
		return nil, nil, fmt.Errorf("open zip %s: %w", archivePath, err)
	}
	f, err := zr.Open(memberName)
	if err != nil {
		zr.Close()
		return nil, nil, fmt.Errorf("open member %s in %s: %w", memberName, archivePath, err)
	}
	return f, zr.Close, nil
}

func openSevenZipMember(archivePath, memberName string) (io.ReadCloser, func() error, error) {
	r, err := sevenzip.OpenReader(archivePath)
	if err != nil {
		return nil, nil, fmt.Errorf("open 7z %s: %w", archivePath, err)
	}
	f, err := r.Open(memberName)
	if err != nil {
		r.Close()
		return nil, nil, fmt.Errorf("open member %s in %s: %w", memberName, archivePath, err)
	}
	return f, r.Close, nil
}

// zipOldMethod names the first compression method in the zip at path that Go's
// zip reader (and RetroArch) cannot decompress: anything but stored or
// deflated. "" when every member is stored or deflated, or the file is not a
// readable zip (that fails later with its own error).
func zipOldMethod(path string) string {
	zr, err := zip.OpenReader(path)
	if err != nil {
		return ""
	}
	defer zr.Close()
	for _, f := range zr.File {
		if f.Method != zip.Store && f.Method != zip.Deflate {
			return store.ZipMethodName(f.Method)
		}
	}
	return ""
}

// extractZipMemberWithTool is the fallback for a zip member Go cannot
// decompress: 7-Zip reads the old PKZIP methods. Without it the error says
// what is wrong, instead of "unsupported compression algorithm".
func extractZipMemberWithTool(ctx context.Context, sevenZip, archivePath, memberName, destPath string) error {
	if sevenZip == "" {
		method := zipOldMethod(archivePath)
		if method == "" {
			method = "an unsupported method"
		}
		return fmt.Errorf("%s is compressed with an old method (%s) that needs 7-Zip, which was not found next to the app", filepath.Base(archivePath), method)
	}
	return rararc.Tool{Path: sevenZip}.Extract(ctx, archivePath, memberName, destPath)
}
