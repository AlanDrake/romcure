//go:build windows

package link

import (
	"fmt"
	"os"
	"unsafe"

	"golang.org/x/sys/windows"
)

// fsctlDuplicateExtentsToFile is FSCTL_DUPLICATE_EXTENTS_TO_FILE:
// CTL_CODE(FILE_DEVICE_FILE_SYSTEM, 209, METHOD_BUFFERED, FILE_WRITE_DATA).
// Not exposed by golang.org/x/sys/windows, so defined here directly.
const fsctlDuplicateExtentsToFile = 0x00098344

// duplicateExtentsData mirrors DUPLICATE_EXTENTS_DATA from winioctl.h.
type duplicateExtentsData struct {
	FileHandle       windows.Handle
	SourceFileOffset int64
	TargetFileOffset int64
	ByteCount        int64
}

const (
	// reflinkChunkSize is the byte range cloned per ioctl call. Comfortably
	// under documented practical limits for FSCTL_DUPLICATE_EXTENTS_TO_FILE
	// while staying a small number of calls for multi-GB disc images.
	reflinkChunkSize = 1 << 30 // 1 GiB
	// reflinkAlignment covers common NTFS/ReFS cluster sizes (4/8/16/32/64 KiB).
	// The final chunk's ByteCount is rounded up to this boundary: both files
	// share the same cluster-granular allocation past their logical EOF, so
	// a slightly-overshooting clone of that trailing sliver is harmless.
	reflinkAlignment = 64 * 1024
)

// reflink clones src to dst via FSCTL_DUPLICATE_EXTENTS_TO_FILE (ReFS,
// including Dev Drive). dst must not already exist. Untested against real
// ReFS/Dev Drive hardware — this dev environment has none — so treat with
// extra scrutiny before relying on it.
func reflink(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("open %s: %w", src, err)
	}
	defer in.Close()

	stat, err := in.Stat()
	if err != nil {
		return fmt.Errorf("stat %s: %w", src, err)
	}
	size := stat.Size()

	out, err := os.OpenFile(dst, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0o644)
	if err != nil {
		return fmt.Errorf("create %s: %w", dst, err)
	}
	defer out.Close()

	if size > 0 {
		if err := out.Truncate(size); err != nil {
			os.Remove(dst)
			return fmt.Errorf("set size of %s: %w", dst, err)
		}
	}

	srcHandle := windows.Handle(in.Fd())
	dstHandle := windows.Handle(out.Fd())

	var offset int64
	for offset < size {
		remaining := size - offset
		advance := remaining
		if advance > reflinkChunkSize {
			advance = reflinkChunkSize
		}
		count := advance
		if rem := count % reflinkAlignment; rem != 0 {
			count += reflinkAlignment - rem
		}

		data := duplicateExtentsData{
			FileHandle:       srcHandle,
			SourceFileOffset: offset,
			TargetFileOffset: offset,
			ByteCount:        count,
		}
		var bytesReturned uint32
		if err := windows.DeviceIoControl(
			dstHandle, fsctlDuplicateExtentsToFile,
			(*byte)(unsafe.Pointer(&data)), uint32(unsafe.Sizeof(data)),
			nil, 0, &bytesReturned, nil,
		); err != nil {
			os.Remove(dst)
			return fmt.Errorf("FSCTL_DUPLICATE_EXTENTS_TO_FILE at offset %d: %w", offset, err)
		}

		offset += advance
	}

	return nil
}
