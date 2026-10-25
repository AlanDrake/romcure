// Package dat parses Logiqx XML and ClrMamePro text DAT files and detects
// parent/clone vs. non-P/C No-Intro DATs. See spec §2.9.
package dat

import (
	"bytes"
	"fmt"
	"io"
	"os"
)

// Format identifies which DAT syntax was parsed.
type Format string

const (
	FormatLogiqxXML  Format = "logiqx_xml"
	FormatClrMamePro Format = "clrmamepro"
)

// Rom is one ROM entry within a Game. CRC32/SHA1/MD5 are lowercase hex, ""
// if the DAT didn't supply that hash.
type Rom struct {
	Name   string
	Size   int64
	CRC32  string
	SHA1   string
	MD5    string
	Status string // e.g. "baddump", "verified"; "" if not specified
	// Serial is the game code (GBA/NDS/N64 cartridge ID); "" if the DAT has none.
	Serial string
}

// Game is one game (or ClrMamePro "machine") entry in a DAT.
type Game struct {
	Name string
	// CloneOf is the parent game's Name, or "" if this game isn't marked
	// as a clone. Only the separately-offered No-Intro Parent/Clone XML
	// DATs populate this; standard No-Intro, Redump, and TOSEC DATs never
	// do (spec §2.9) — the Rule Engine needs a fallback for those.
	CloneOf string
	Roms    []Rom
}

// Datafile is a parsed DAT file.
type Datafile struct {
	Format     Format
	SystemName string
	// HasParentClone is true if any Game in this DAT has a non-empty
	// CloneOf. Lets the caller detect whether it loaded a P/C DAT or a
	// plain one and tell the user, per spec §2.9.
	HasParentClone bool
	Games          []Game
}

// ParseFile reads and parses path, detecting Logiqx XML vs. ClrMamePro
// text automatically.
func ParseFile(path string) (*Datafile, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	defer f.Close()
	return Parse(f)
}

// Parse reads and parses r, detecting Logiqx XML vs. ClrMamePro text
// automatically.
func Parse(r io.Reader) (*Datafile, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("read dat: %w", err)
	}

	data = bytes.TrimPrefix(data, []byte{0xEF, 0xBB, 0xBF}) // UTF-8 BOM
	trimmed := bytes.TrimLeft(data, " \t\r\n")

	if len(trimmed) > 0 && trimmed[0] == '<' {
		return parseLogiqxXML(data)
	}
	return parseClrMamePro(data)
}
