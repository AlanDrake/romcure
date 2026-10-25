// Package proc holds small helpers for running the bundled converter tools
// as subprocesses (HANDOFF §3).
package proc

import "os/exec"

// Quiet stops cmd from opening a visible console window. It only matters on
// Windows, where a GUI app that starts a console program otherwise flashes a
// terminal for every run; elsewhere it does nothing.
func Quiet(cmd *exec.Cmd) *exec.Cmd {
	quiet(cmd)
	return cmd
}
