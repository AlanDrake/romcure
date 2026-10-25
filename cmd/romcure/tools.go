package main

import (
	"os"
	"path/filepath"
)

// findTool returns the absolute path of the first of names found in dirs (in
// order), or else on the PATH (lookPath), or "" when it is nowhere. A tool
// shipped with the app always wins over one the system happens to have.
func findTool(dirs, names []string, lookPath func(string) (string, error)) string {
	for _, d := range dirs {
		for _, n := range names {
			p := filepath.Join(d, n)
			if st, err := os.Stat(p); err == nil && !st.IsDir() {
				abs, _ := filepath.Abs(p)
				return abs
			}
		}
	}
	for _, n := range names {
		if p, err := lookPath(n); err == nil {
			abs, _ := filepath.Abs(p)
			return abs
		}
	}
	return ""
}
