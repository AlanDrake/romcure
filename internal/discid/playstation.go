package discid

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"io"
	"regexp"
	"strings"
)

// PlayStation and PlayStation 2 discs carry no title, only the serial of the
// executable SYSTEM.CNF boots ("BOOT = cdrom:\SLUS_005.94;1"). The serial is
// turned into a name with a serial list (store.SerialName).

// sectorLayout is how 2048-byte data sectors sit in an image file.
type sectorLayout struct {
	size, offset int64 // bytes per sector in the file, data offset within it
}

var layouts = []sectorLayout{
	{2048, 0},  // .iso: plain data sectors
	{2352, 24}, // raw Mode 2 (PlayStation .bin)
	{2352, 16}, // raw Mode 1
}

var bootLine = regexp.MustCompile(`(?im)^\s*BOOT2?\s*=\s*cdrom0?:\\?([^;\r\n]+)`)

// psExts are names that may hold a PlayStation data track.
var psExts = map[string]bool{".iso": true, ".bin": true, ".img": true}

// ReadPlayStation finds the serial in a PlayStation or PlayStation 2 image
// ("SLUS-00594"). r is the image (a raw track or an ISO), size its length.
func ReadPlayStation(r io.ReaderAt, size int64) (string, bool) {
	for _, l := range layouts {
		sector := func(lba int64) []byte {
			b := make([]byte, 2048)
			off := lba*l.size + l.offset
			if off+2048 > size {
				return nil
			}
			if _, err := r.ReadAt(b, off); err != nil && err != io.EOF {
				return nil
			}
			return b
		}
		pvd := sector(16)
		if pvd == nil || !bytes.Equal(pvd[:6], []byte("\x01CD001")) {
			continue
		}
		root := pvd[156:]
		lba, length := int64(binary.LittleEndian.Uint32(root[2:])), int64(binary.LittleEndian.Uint32(root[10:]))
		// The boot executable sits in the root directory under the serial's
		// name ("SLES_123.45"); it is the answer when SYSTEM.CNF lies beyond
		// what was read (a PS2 disc read from an archive).
		fallback := ""
		for i := int64(0); i*2048 < length && i < 16; i++ {
			dir := sector(lba + i)
			if dir == nil {
				break
			}
			for p := 0; p < len(dir); {
				n := int(dir[p])
				if n == 0 || p+n > len(dir) || n < 34 {
					break
				}
				nameLen := int(dir[p+32])
				if p+33+nameLen > len(dir) {
					break
				}
				name := strings.ToUpper(string(dir[p+33 : p+33+nameLen]))
				if name == "SYSTEM.CNF;1" || name == "SYSTEM.CNF" {
					if cnf := sector(int64(binary.LittleEndian.Uint32(dir[p+2:]))); cnf != nil {
						return serialFromCNF(cnf)
					}
				} else if fallback == "" && exeName.MatchString(name) {
					fallback, _ = serialFromExe(name)
				}
				p += n
			}
		}
		return fallback, fallback != ""
	}
	return "", false
}

var exeName = regexp.MustCompile(`^[A-Z]{4}_\d{3}\.\d{2}(;1)?$`)

// serialFromCNF reads the boot executable's name from SYSTEM.CNF and turns it
// into the serial as lists write it: "SLUS_005.94" -> "SLUS-00594".
func serialFromCNF(cnf []byte) (string, bool) {
	if i := bytes.IndexByte(cnf, 0); i >= 0 {
		cnf = cnf[:i]
	}
	sc := bufio.NewScanner(bytes.NewReader(cnf))
	for sc.Scan() {
		m := bootLine.FindStringSubmatch(sc.Text())
		if m == nil {
			continue
		}
		exe := strings.TrimSpace(m[1])
		if i := strings.LastIndexAny(exe, `\/`); i >= 0 {
			exe = exe[i+1:]
		}
		return serialFromExe(exe)
	}
	return "", false
}

// serialFromExe: "SLUS_005.94;1" -> "SLUS-00594".
func serialFromExe(exe string) (string, bool) {
	exe, _, _ = strings.Cut(strings.ToUpper(exe), ";")
	code, num, ok := strings.Cut(exe, "_")
	if !ok || len(code) != 4 {
		return "", false
	}
	num = strings.ReplaceAll(num, ".", "")
	if num == "" {
		return "", false
	}
	return code + "-" + num, true
}

// readerAt serves ReadAt from the start of a stream that was read into memory
// (an archive member: a PlayStation disc's file system starts in its first
// sectors).
type readerAt []byte

func (b readerAt) ReadAt(p []byte, off int64) (int, error) {
	if off >= int64(len(b)) {
		return 0, io.EOF
	}
	n := copy(p, b[off:])
	if n < len(p) {
		return n, io.EOF
	}
	return n, nil
}

// psProbe is how much of a stream is kept to look for SYSTEM.CNF in it.
const psProbe = 4 << 20

// ReadAny looks for any disc header this package knows in a stream: GameCube
// and Wii first, then PlayStation (its serial goes in ID, no title). name
// decides which kinds are worth trying.
func ReadAny(name string, r io.Reader) (Info, bool) {
	ext := extOf(name)
	if cartExts[ext] != nil {
		return ReadCartridge(name, r)
	}
	var head []byte
	if exts[ext] {
		buf := make([]byte, probe)
		n, _ := io.ReadFull(r, buf)
		head = buf[:n]
		if info, ok := Read(bytes.NewReader(head)); ok {
			return info, true
		}
	}
	if !psExts[ext] {
		return Info{}, false
	}
	rest := make([]byte, psProbe-len(head))
	n, _ := io.ReadFull(r, rest)
	all := append(head, rest[:n]...)
	if serial, ok := ReadPlayStation(readerAt(all), int64(len(all))); ok {
		return Info{ID: serial}, true
	}
	return Info{}, false
}

// ReadAt is ReadAny for a file that can be read anywhere (a raw image on
// disk): no limit on where SYSTEM.CNF lies.
func ReadAt(name string, r io.ReaderAt, size int64) (Info, bool) {
	ext := extOf(name)
	if cartExts[ext] != nil {
		return ReadCartridge(name, io.NewSectionReader(r, 0, size))
	}
	if exts[ext] {
		if info, ok := Read(io.NewSectionReader(r, 0, size)); ok {
			return info, true
		}
	}
	if psExts[ext] {
		if serial, ok := ReadPlayStation(r, size); ok {
			return Info{ID: serial}, true
		}
	}
	return Info{}, false
}

// MayHaveAny reports whether ReadAny is worth trying on a file of this name.
func MayHaveAny(name string) bool {
	e := extOf(name)
	return exts[e] || psExts[e] || cartExts[e] != nil
}

// IsPlayStationSerial tells a PlayStation serial ("SLUS-00594") from a
// GameCube/Wii game ID ("GEXE01").
func IsPlayStationSerial(id string) bool { return strings.Contains(id, "-") }
