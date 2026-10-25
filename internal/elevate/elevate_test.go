package elevate

import "testing"

// These smoke tests exercise every function on whichever OS they run on
// (the real implementation on Windows, the no-op stubs elsewhere) and log
// what they found — this is the "document the result" manual test note
// HANDOFF asks for M0 task 5, captured as a reusable test rather than a
// one-off record.

func TestSymlinkProbe(t *testing.T) {
	ok, err := SymlinkProbe()
	if err != nil {
		t.Fatalf("SymlinkProbe: %v", err)
	}
	t.Logf("SymlinkProbe: %v", ok)
}

func TestIsDevModeEnabled(t *testing.T) {
	ok, err := IsDevModeEnabled()
	if err != nil {
		t.Fatalf("IsDevModeEnabled: %v", err)
	}
	t.Logf("IsDevModeEnabled: %v", ok)
}

func TestIsElevated(t *testing.T) {
	ok, err := IsElevated()
	if err != nil {
		t.Fatalf("IsElevated: %v", err)
	}
	t.Logf("IsElevated: %v", ok)
}
