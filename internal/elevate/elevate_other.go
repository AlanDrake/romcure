//go:build !windows

package elevate

import "errors"

// SymlinkProbe always reports true: symlink creation needs no special
// privilege on macOS/Linux (spec §2.2).
func SymlinkProbe() (bool, error) { return true, nil }

// IsDevModeEnabled is a Windows-only concept; always false elsewhere.
func IsDevModeEnabled() (bool, error) { return false, nil }

// IsElevated is not meaningful here: this package's elevation model is
// Windows-specific (spec §2.3), so callers on other OSes should never
// need to ask. Always reports true so a caller that checks anyway doesn't
// mistakenly think it needs to relaunch.
func IsElevated() (bool, error) { return true, nil }

// RelaunchElevated always fails: there is nothing to relaunch into.
// Callers must gate any call to this behind a Windows-specific code path;
// this stub exists only so shared call sites compile cross-platform.
func RelaunchElevated() error {
	return errors.New("elevate.RelaunchElevated is only applicable on Windows")
}
