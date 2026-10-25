package scan

import (
	"archive/zip"
	"errors"
	"fmt"
	"io"

	"romcure/internal/store"
)

// readZipMembers lists a zip's contents from its central directory —
// CRC32 and uncompressed size are stored there, so this never decompresses
// a member (spec §2.8). oldMethod names the first compression method Go (and
// RetroArch) cannot read, "" when every member is stored or deflated.
func readZipMembers(path string) (members []store.ArchiveMemberRecord, oldMethod string, err error) {
	zr, err := zip.OpenReader(path)
	if err != nil {
		return nil, "", fmt.Errorf("open zip %s: %w", path, err)
	}
	defer zr.Close()

	for _, f := range zr.File {
		if f.FileInfo().IsDir() {
			continue
		}
		if oldMethod == "" && f.Method != zip.Store && f.Method != zip.Deflate {
			oldMethod = store.ZipMethodName(f.Method)
		}
		members = append(members, store.ArchiveMemberRecord{
			MemberName: f.Name,
			Size:       int64(f.UncompressedSize64),
			CRC32:      fmt.Sprintf("%08x", f.CRC32),
		})
	}
	// A member an old method packed cannot be read for its header; its CRC is
	// still known, so the archive stays (see skippable).
	err = addHeaderless(members, func(name string) (io.ReadCloser, error) { return zr.Open(name) })
	return members, oldMethod, err
}

// zipOldMethod reads only the central directory of the zip at path.
func zipOldMethod(path string) (string, error) {
	zr, err := zip.OpenReader(path)
	if err != nil {
		return "", fmt.Errorf("open zip %s: %w", path, err)
	}
	defer zr.Close()
	for _, f := range zr.File {
		if !f.FileInfo().IsDir() && f.Method != zip.Store && f.Method != zip.Deflate {
			return store.ZipMethodName(f.Method), nil
		}
	}
	return "", nil
}

// skippable reports whether err only says a member uses a compression method
// Go cannot decompress: its header is then left unread instead of failing the
// whole archive.
func skippable(err error) bool { return errors.Is(err, zip.ErrAlgorithm) }
