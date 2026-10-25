package proc

import (
	"os/exec"
	"testing"
)

func TestQuietHidesTheConsoleWindow(t *testing.T) {
	cmd := Quiet(exec.Command("whatever"))
	if cmd.SysProcAttr == nil || !cmd.SysProcAttr.HideWindow || cmd.SysProcAttr.CreationFlags&createNoWindow == 0 {
		t.Errorf("console window not suppressed: %+v", cmd.SysProcAttr)
	}
}
