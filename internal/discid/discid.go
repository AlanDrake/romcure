// Package discid reads the game ID and title that GameCube and Wii discs carry
// in their header, so a disc no DAT knows can still be named properly instead
// of by its (often cryptic, scene-style) file name. It understands plain images
// (.iso/.gcm) and the WBFS, CISO and RVZ/WIA containers, which all keep a copy
// of the disc header near the start of the file.
package discid

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Info is what a disc header says about the game.
type Info struct {
	ID    string // e.g. "GEXE01": game code (4), maker (2)
	Title string // as stored in the header; "" when unreadable
	Disc  int    // disc number, 1 for the first (or only) disc
}

// probe is how much of the file Read needs at most (CISO keeps the disc
// header after its 32 KiB block map).
const probe = 0x8000 + 0x80

var exts = map[string]bool{".iso": true, ".gcm": true, ".rvz": true, ".wia": true, ".wbfs": true, ".ciso": true}

// MayHave reports whether a file of this name can hold a GameCube/Wii disc.
func MayHave(name string) bool { return exts[extOf(name)] }

func extOf(name string) string { return strings.ToLower(filepath.Ext(name)) }

// Read looks for a GameCube or Wii disc header at the start of r. ok is false
// when there is none (another console's ISO, a damaged file).
func Read(r io.Reader) (Info, bool) {
	buf := make([]byte, probe)
	n, _ := io.ReadFull(r, buf)
	buf = buf[:n]

	at := 0
	switch {
	case bytes.HasPrefix(buf, []byte("WBFS")):
		at = 0x200
	case bytes.HasPrefix(buf, []byte("RVZ\x01")), bytes.HasPrefix(buf, []byte("WIA\x01")):
		at = 0x58 // header 1 (0x48 bytes), then header 2 whose disc header copy starts at +0x10
	case bytes.HasPrefix(buf, []byte("CISO")):
		at = 0x8000
	}
	if len(buf) < at+0x80 {
		return Info{}, false
	}
	return parse(buf[at : at+0x80])
}

const (
	wiiMagic = 0x5D1C9EA3 // at 0x18
	gcMagic  = 0xC2339F3D // at 0x1C
)

func parse(h []byte) (Info, bool) {
	if binary.BigEndian.Uint32(h[0x18:]) != wiiMagic && binary.BigEndian.Uint32(h[0x1C:]) != gcMagic {
		return Info{}, false
	}
	id := string(h[:6])
	for _, c := range id {
		if !(c >= 'A' && c <= 'Z' || c >= '0' && c <= '9') {
			return Info{}, false
		}
	}
	title := h[0x20:0x80]
	if i := bytes.IndexByte(title, 0); i >= 0 {
		title = title[:i]
	}
	t := strings.Join(strings.Fields(string(title)), " ")
	// Japanese discs store their title in Shift-JIS: keep only a clean ASCII/UTF-8 one.
	if !utf8.ValidString(t) || strings.IndexFunc(t, func(r rune) bool { return !unicode.IsPrint(r) }) >= 0 {
		t = ""
	}
	return Info{ID: id, Title: t, Disc: int(h[6]) + 1}, true
}

// Region names the region the game code's fourth letter stands for, in the
// words DAT names use ("" when unknown).
func (i Info) Region() string {
	if len(i.ID) < 4 {
		return ""
	}
	switch i.ID[3] {
	case 'E':
		return "USA"
	case 'P', 'X', 'Y', 'Z':
		return "Europe"
	case 'J':
		return "Japan"
	case 'K':
		return "Korea"
	case 'D':
		return "Germany"
	case 'F':
		return "France"
	case 'S':
		return "Spain"
	case 'I':
		return "Italy"
	case 'U':
		return "Australia"
	case 'H':
		return "Netherlands"
	case 'W':
		return "Taiwan"
	}
	return ""
}

// Name is a display name in DAT style, "Title (Region)", or "" without a title.
func (i Info) Name() string {
	if i.Title == "" {
		return ""
	}
	if r := i.Region(); r != "" {
		return i.Title + " (" + r + ")"
	}
	return i.Title
}

// DiscTag is " (Disc N)" for a game's second or later disc, "" otherwise.
// (The first disc's header does not say whether more discs follow.)
func (i Info) DiscTag() string {
	if i.Disc > 1 {
		return fmt.Sprintf(" (Disc %d)", i.Disc)
	}
	return ""
}
