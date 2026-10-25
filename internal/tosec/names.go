// Package tosec discovers and downloads TOSEC datfiles. TOSEC publishes one
// zip per release holding a DAT for every system, category and image
// format; this package finds the latest release, downloads it, and imports
// the DATs belonging to one system.
package tosec

import (
	"path"
	"regexp"
	"strings"
)

// DatName is what a TOSEC DAT's file name says about it.
type DatName struct {
	Product  string // "Commodore Amiga"
	Category string // "Games", "Demos", "Applications", ...
	Format   string // "[ADF]" etc., "" if the name has none
	Version  string // "2025-01-30"
}

var versionPattern = regexp.MustCompile(`\(TOSEC-v([0-9-]+)(?:_\w+)?\)`)

// ParseDatName splits "Commodore Amiga - Games - [ADF] (TOSEC-v2025-01-30_CM).dat".
// ok is false if the name has no TOSEC version marker.
func ParseDatName(filename string) (DatName, bool) {
	base := strings.TrimSuffix(path.Base(strings.ReplaceAll(filename, "\\", "/")), path.Ext(filename))
	vm := versionPattern.FindStringSubmatch(base)
	if vm == nil {
		return DatName{}, false
	}
	stem := strings.TrimSpace(base[:strings.Index(base, vm[0])])
	parts := strings.Split(stem, " - ")
	n := DatName{Product: strings.TrimSpace(parts[0]), Version: vm[1]}
	if len(parts) > 1 {
		n.Category = strings.TrimSpace(parts[1])
	}
	if last := strings.TrimSpace(parts[len(parts)-1]); len(parts) > 2 && strings.HasPrefix(last, "[") {
		n.Format = last
	}
	return n, n.Product != ""
}

// productSystems maps a TOSEC product name to this app's system id.
// Deliberately partial: computers whose ROM collections TOSEC is the usual
// DAT source for. Unmapped products are skipped, not guessed.
var productSystems = map[string]string{
	"Commodore Amiga":      "amiga",
	"Commodore C64":        "c64",
	"Sinclair ZX Spectrum": "zxspectrum",
	"Atari ST":             "atarist",
	"Amstrad CPC":          "amstradcpc",
	"MSX MSX":              "msx",
	"MSX MSX2":             "msx",
	"Atari 8bit":           "atari800",
	"Apple II":             "apple2",
}

// SystemID returns the app system id for a TOSEC product name.
func SystemID(product string) (string, bool) {
	id, ok := productSystems[product]
	return id, ok
}

// Systems lists the app system ids this package can sync.
func Systems() []string {
	seen := map[string]bool{}
	var out []string
	for _, id := range productSystems {
		if !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	return out
}
