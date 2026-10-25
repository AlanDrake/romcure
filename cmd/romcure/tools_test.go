package main

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestFindToolPrefersTheBundledOneAndFallsBackToPATH(t *testing.T) {
	dir := t.TempDir()
	bundled := filepath.Join(dir, "chdman")
	os.WriteFile(bundled, []byte("x"), 0o755)
	system := filepath.Join(t.TempDir(), "chdman")
	os.WriteFile(system, []byte("y"), 0o755)
	onPath := func(name string) (string, error) {
		if name == "chdman" || name == "7zz" {
			return system, nil
		}
		return "", errors.New("not found")
	}

	if got := findTool([]string{dir}, []string{"chdman"}, onPath); got != bundled {
		t.Errorf("bundled = %q, want %q", got, bundled)
	}
	if got := findTool([]string{t.TempDir()}, []string{"chdman"}, onPath); got != system {
		t.Errorf("PATH fallback = %q, want %q", got, system)
	}
	if got := findTool([]string{dir}, []string{"7zz", "7z"}, onPath); got != system {
		t.Errorf("the second name must be tried on the PATH too: %q", got)
	}
	if got := findTool([]string{dir}, []string{"dolphin-tool"}, onPath); got != "" {
		t.Errorf("missing tool = %q, want \"\"", got)
	}
}
