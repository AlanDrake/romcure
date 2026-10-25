//go:build !windows

package elevate

import "testing"

func TestRelaunchElevatedIsANoOp(t *testing.T) {
	if err := RelaunchElevated(); err == nil {
		t.Error("RelaunchElevated() = nil error on a non-Windows OS, want an error (there is nothing to relaunch into)")
	}
}
