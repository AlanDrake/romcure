//go:build windows

package profile

import (
	"fmt"

	"golang.org/x/sys/windows"
)

// setHiddenAttribute sets the Windows hidden file attribute on path, in
// addition to the dot-prefix every OS honors (spec §2.5 — the .discs/
// sibling folder is "dot-prefixed + Windows hidden attribute").
func setHiddenAttribute(path string) error {
	ptr, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return fmt.Errorf("encode path %s: %w", path, err)
	}
	attrs, err := windows.GetFileAttributes(ptr)
	if err != nil {
		return fmt.Errorf("get attributes of %s: %w", path, err)
	}
	if err := windows.SetFileAttributes(ptr, attrs|windows.FILE_ATTRIBUTE_HIDDEN); err != nil {
		return fmt.Errorf("set hidden attribute on %s: %w", path, err)
	}
	return nil
}
