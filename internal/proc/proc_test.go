package proc

import (
	"os/exec"
	"testing"
)

func TestQuietReturnsTheCommand(t *testing.T) {
	cmd := exec.Command("whatever")
	if Quiet(cmd) != cmd {
		t.Fatal("Quiet must return the command it was given")
	}
}
