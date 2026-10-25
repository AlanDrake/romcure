//go:build !windows

package profile

// setHiddenAttribute is a no-op on macOS/Linux: the dot-prefix alone is
// the convention there (spec §2.5).
func setHiddenAttribute(path string) error {
	return nil
}
