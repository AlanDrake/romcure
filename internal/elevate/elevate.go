// Package elevate implements the Windows runtime UAC relaunch flow for
// symlink creation (no-op on other OSes). See spec §2.3.
//
// SymlinkProbe is the ground truth for "can this process create a symlink
// right now" — it actually attempts one. IsDevModeEnabled and IsElevated
// are diagnostic signals for explaining *why* to the user, not substitutes
// for the probe.
package elevate

import "errors"

// ErrUACDeclined is returned by RelaunchElevated when the user declines
// the UAC prompt. The caller should fall back to the next link tier for
// the rest of the session and remember the choice (spec §2.3).
var ErrUACDeclined = errors.New("user declined the UAC elevation prompt")
