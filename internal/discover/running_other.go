//go:build !windows && !linux

package discover

import (
	"os/exec"
	"path/filepath"
	"strings"

	"romcure/internal/proc"
)

// runningRetroArchDirs returns the folders of the RetroArch processes that
// are open now, from the executable paths `ps` lists.
func runningRetroArchDirs() []string {
	out, err := proc.Quiet(exec.Command("ps", "-axo", "comm=")).Output()
	if err != nil {
		return nil
	}
	var dirs []string
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(strings.ToLower(filepath.Base(line)), "retroarch") {
			dirs = append(dirs, filepath.Dir(line))
		}
	}
	return dirs
}
