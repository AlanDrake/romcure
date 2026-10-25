//go:build windows

package elevate

import "testing"

func TestRelaunchElevated(t *testing.T) {
	// Not run automatically: this triggers a real UAC consent prompt and
	// would hang any unattended test run waiting on a human. Verified
	// manually instead — see the commit message for this file's result on
	// a standard (non-admin) account.
	t.Skip("RelaunchElevated triggers a real UAC prompt; verify manually")
}
