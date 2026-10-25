// Package romheader recognizes the copier/emulator headers some cartridge
// dumps carry in front of the ROM data. No-Intro DATs list the CRC of the ROM
// without that header (as clrmamepro's header skippers do), so a headered
// file only matches its DAT entry when the header is left out of the hash.
package romheader

import (
	"bytes"
	"fmt"
	"hash/crc32"
	"io"
	"path/filepath"
	"strings"
)

// rule describes one header format.
type rule struct {
	exts []string
	// skip returns the header length for a file starting with head (at most
	// probeBytes long) and of the given total size; 0 = no header.
	skip func(head []byte, size int64) int64
}

const probeBytes = 16

func magicAt(off int, magic string, n int64) func([]byte, int64) int64 {
	return func(head []byte, size int64) int64 {
		if len(head) >= off+len(magic) && bytes.Equal(head[off:off+len(magic)], []byte(magic)) && size > n {
			return n
		}
		return 0
	}
}

var rules = []rule{
	{exts: []string{".nes"}, skip: magicAt(0, "NES\x1a", 16)},          // iNES / NES 2.0
	{exts: []string{".fds"}, skip: magicAt(0, "FDS\x1a", 16)},          // fwNES
	{exts: []string{".a78"}, skip: magicAt(1, "ATARI7800", 128)},       // Atari 7800
	{exts: []string{".lnx"}, skip: magicAt(0, "LYNX\x00", 64)},         // Atari Lynx
	{exts: []string{".smc", ".sfc", ".swc", ".fig"}, skip: snesCopier}, // SNES copier (SMC/SWC/FIG)
}

// snesCopier: a 512-byte copier header makes the size 512 more than a multiple of 1 KiB.
func snesCopier(_ []byte, size int64) int64 {
	if size > 512 && size%1024 == 512 {
		return 512
	}
	return 0
}

func ruleFor(name string) *rule {
	ext := strings.ToLower(filepath.Ext(name))
	for i := range rules {
		for _, e := range rules[i].exts {
			if e == ext {
				return &rules[i]
			}
		}
	}
	return nil
}

// MayHaveHeader reports whether a file of this name can carry a header, i.e.
// whether it is worth reading it to find out.
func MayHaveHeader(name string) bool { return ruleFor(name) != nil }

// Result is the hash of a file's data without its header.
type Result struct {
	CRC32 string // lowercase hex, 8 digits
	Size  int64  // bytes after the header
}

// Hash reads r (the whole file, size bytes, named name) and returns the CRC32
// of everything after a recognized header. ok is false when the file has no
// header (its plain CRC is then the one to use).
func Hash(name string, r io.Reader, size int64) (res Result, ok bool, err error) {
	ru := ruleFor(name)
	if ru == nil {
		return Result{}, false, nil
	}
	head := make([]byte, probeBytes)
	n, err := io.ReadFull(r, head)
	if err != nil && err != io.ErrUnexpectedEOF && err != io.EOF {
		return Result{}, false, err
	}
	head = head[:n]
	skip := ru.skip(head, size)
	if skip == 0 {
		return Result{}, false, nil
	}
	h := crc32.NewIEEE()
	if int64(n) > skip {
		h.Write(head[skip:])
	} else if _, err := io.CopyN(io.Discard, r, skip-int64(n)); err != nil {
		return Result{}, false, err
	}
	if _, err := io.Copy(h, r); err != nil {
		return Result{}, false, err
	}
	return Result{CRC32: fmt.Sprintf("%08x", h.Sum32()), Size: size - skip}, true, nil
}
