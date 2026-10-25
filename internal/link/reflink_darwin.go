//go:build darwin

package link

import (
	"fmt"

	"golang.org/x/sys/unix"
)

// reflink clones src to dst via clonefile(2) (APFS). dst must not already
// exist — unix.Clonefile requires that itself.
func reflink(src, dst string) error {
	if err := unix.Clonefile(src, dst, 0); err != nil {
		return fmt.Errorf("clonefile: %w", err)
	}
	return nil
}
