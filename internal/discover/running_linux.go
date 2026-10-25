package discover

import (
	"os"
	"path/filepath"
	"strings"
)

// runningRetroArchDirs returns the folders of the retroarch processes that
// are open now, from /proc/<pid>/exe. A Flatpak or Snap RetroArch shows a
// path inside its sandbox; those installs come from the usual places instead.
func runningRetroArchDirs() []string {
	exes, _ := filepath.Glob("/proc/[0-9]*/exe")
	var dirs []string
	for _, link := range exes {
		target, err := os.Readlink(link)
		if err != nil || !strings.HasPrefix(strings.ToLower(filepath.Base(target)), "retroarch") {
			continue
		}
		dirs = append(dirs, filepath.Dir(target))
	}
	return dirs
}
