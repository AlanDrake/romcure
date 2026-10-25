//go:build !windows

package discover

import (
	"os"
	"path/filepath"
	"runtime"
)

// removableDrives lists mounted external volumes: /Volumes on macOS,
// /media/<user> and /run/media/<user> on Linux.
func removableDrives() []string {
	var roots []string
	if runtime.GOOS == "darwin" {
		roots = []string{"/Volumes"}
	} else if u := os.Getenv("USER"); u != "" {
		roots = []string{filepath.Join("/media", u), filepath.Join("/run/media", u)}
	}
	var out []string
	for _, r := range roots {
		entries, _ := os.ReadDir(r)
		for _, e := range entries {
			if e.IsDir() && e.Name() != "Macintosh HD" {
				out = append(out, filepath.Join(r, e.Name()))
			}
		}
	}
	return out
}
