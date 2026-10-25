package discid

import (
	"bytes"
	"encoding/binary"
	"io"
	"strings"
)

// This file also reads the game code of Game Boy Advance, Nintendo DS and
// Nintendo 64 cartridge dumps. They are not discs, but they share the backfill
// path (files.disc_id, the scan's header read) and the code is used the same
// way: to name a dump no DAT matched, here through the serial the DAT lists.

// cartExts maps a cartridge file extension to its reader.
var cartExts = map[string]func([]byte) (Info, bool){
	".gba": parseGBA, ".agb": parseGBA,
	".nds": parseNDS,
	".n64": parseN64, ".v64": parseN64, ".z64": parseN64,
}

// cartProbe is how much of a dump the readers need (the NDS header is the longest).
const cartProbe = 0x160

// IsCartridgeSystem reports whether systemID is a system whose dumps carry a
// game code read by this file.
func IsCartridgeSystem(systemID string) bool {
	return systemID == "gba" || systemID == "nds" || systemID == "n64"
}

// ReadCartridge reads the game code of a GBA, NDS or N64 dump from the start
// of r. ok is false when the name is not such a file or its header does not
// check out (a bad checksum, so the code cannot be trusted).
func ReadCartridge(name string, r io.Reader) (Info, bool) {
	parse := cartExts[extOf(name)]
	if parse == nil {
		return Info{}, false
	}
	buf := make([]byte, cartProbe)
	n, _ := io.ReadFull(r, buf)
	return parse(buf[:n])
}

func validCode(c []byte) bool {
	for _, b := range c {
		if !(b >= 'A' && b <= 'Z' || b >= '0' && b <= '9') {
			return false
		}
	}
	return len(c) > 0
}

// headerText cleans a fixed-width title field: cut at the first NUL, printable
// ASCII only (anything else makes the title unusable), spaces collapsed.
func headerText(b []byte) string {
	if i := bytes.IndexByte(b, 0); i >= 0 {
		b = b[:i]
	}
	for _, c := range b {
		if c < 0x20 || c > 0x7E {
			return ""
		}
	}
	return strings.Join(strings.Fields(string(b)), " ")
}

// parseGBA: title at 0xA0 (12), code at 0xAC (4), fixed byte 0x96 at 0xB2 and a
// complement check at 0xBD over 0xA0..0xBC.
func parseGBA(h []byte) (Info, bool) {
	if len(h) < 0xC0 || h[0xB2] != 0x96 {
		return Info{}, false
	}
	var sum int
	for _, b := range h[0xA0:0xBD] {
		sum += int(b)
	}
	if byte(-sum-0x19) != h[0xBD] || !validCode(h[0xAC:0xB0]) {
		return Info{}, false
	}
	return Info{ID: string(h[0xAC:0xB0]), Title: headerText(h[0xA0:0xAC])}, true
}

// parseNDS: title at 0 (12), code at 0x0C (4), CRC-16 of 0..0x15D at 0x15E.
func parseNDS(h []byte) (Info, bool) {
	if len(h) < 0x160 || !validCode(h[0x0C:0x10]) {
		return Info{}, false
	}
	crc := uint16(0xFFFF)
	for _, b := range h[:0x15E] {
		crc ^= uint16(b)
		for i := 0; i < 8; i++ {
			if crc&1 != 0 {
				crc = crc>>1 ^ 0xA001
			} else {
				crc >>= 1
			}
		}
	}
	if crc != binary.LittleEndian.Uint16(h[0x15E:]) {
		return Info{}, false
	}
	return Info{ID: string(h[0x0C:0x10]), Title: headerText(h[:0x0C])}, true
}

// parseN64: byte order from the first word (z64 big-endian, v64 byte-swapped,
// n64 little-endian), then title at 0x20 (20) and code at 0x3B (4).
func parseN64(h []byte) (Info, bool) {
	if len(h) < 0x40 {
		return Info{}, false
	}
	b := append([]byte(nil), h[:0x40]...)
	switch binary.BigEndian.Uint32(b) {
	case 0x80371240: // z64
	case 0x37804012: // v64: every pair swapped
		for i := 0; i < len(b); i += 2 {
			b[i], b[i+1] = b[i+1], b[i]
		}
	case 0x40123780: // n64: every word reversed
		for i := 0; i < len(b); i += 4 {
			b[i], b[i+1], b[i+2], b[i+3] = b[i+3], b[i+2], b[i+1], b[i]
		}
	default:
		return Info{}, false
	}
	if !validCode(b[0x3B:0x3F]) {
		return Info{}, false
	}
	return Info{ID: string(b[0x3B:0x3F]), Title: headerText(b[0x20:0x34])}, true
}
