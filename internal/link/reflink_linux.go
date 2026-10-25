//go:build linux

package link

import (
	"fmt"
	"os"

	"golang.org/x/sys/unix"
)

// reflink clones src to dst via the FICLONE ioctl (Btrfs, XFS with reflink
// support). dst must not already exist.
func reflink(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("open %s: %w", src, err)
	}
	defer in.Close()

	out, err := os.OpenFile(dst, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("create %s: %w", dst, err)
	}
	defer out.Close()

	if err := unix.IoctlFileClone(int(out.Fd()), int(in.Fd())); err != nil {
		os.Remove(dst)
		return fmt.Errorf("FICLONE: %w", err)
	}
	return nil
}
