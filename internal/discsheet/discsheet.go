package discsheet

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var cueFileLinePattern = regexp.MustCompile(`(?i)^\s*FILE\s+"([^"]+)"`)

// cueReferencedFiles returns every filename a .cue sheet's FILE lines
// reference, in order. Spec §2.5: "BIN/CUE sets are multi-file even for
// one disc ... every member file must be linked with its exact original
// name into the same directory. Never link the .cue alone."
func cueReferencedFiles(cuePath string) ([]string, error) {
	f, err := os.Open(cuePath)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", cuePath, err)
	}
	defer f.Close()

	var files []string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		if m := cueFileLinePattern.FindStringSubmatch(scanner.Text()); m != nil {
			files = append(files, m[1])
		}
	}
	return files, scanner.Err()
}

var gdiFieldPattern = regexp.MustCompile(`"([^"]+)"|(\S+)`)

// gdiReferencedFiles returns every track filename a .gdi sheet
// (Dreamcast) references. Format: a track-count line, then one line per
// track as whitespace-separated fields with the filename 5th
// (1-indexed) — see spec §2.5, "same applies to GDI + tracks."
func gdiReferencedFiles(gdiPath string) ([]string, error) {
	f, err := os.Open(gdiPath)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", gdiPath, err)
	}
	defer f.Close()

	var files []string
	scanner := bufio.NewScanner(f)
	skippedCountLine := false
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		if !skippedCountLine {
			skippedCountLine = true
			continue
		}
		fields := tokenizeGDILine(line)
		if len(fields) >= 5 {
			files = append(files, fields[4])
		}
	}
	return files, scanner.Err()
}

func tokenizeGDILine(line string) []string {
	matches := gdiFieldPattern.FindAllStringSubmatch(line, -1)
	out := make([]string, 0, len(matches))
	for _, m := range matches {
		if m[1] != "" {
			out = append(out, m[1])
		} else {
			out = append(out, m[2])
		}
	}
	return out
}

// ReferencedFiles dispatches to cueReferencedFiles or
// gdiReferencedFiles by extension, or returns nil for anything else
// (not a multi-file sheet).
func ReferencedFiles(sheetPath string) ([]string, error) {
	switch strings.ToLower(filepath.Ext(sheetPath)) {
	case ".cue":
		return cueReferencedFiles(sheetPath)
	case ".gdi":
		return gdiReferencedFiles(sheetPath)
	default:
		return nil, nil
	}
}
