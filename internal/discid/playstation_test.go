package discid

import (
	"bytes"
	"encoding/binary"
	"testing"
)

// psImage builds a minimal ISO 9660 image (2048-byte sectors) whose root
// directory holds SYSTEM.CNF with the given text; raw lays it out in 2352-byte
// Mode 2 sectors instead, as a PlayStation .bin track is.
func psImage(cnf string, raw bool) []byte { return psImageExe(cnf, "PSX.EXE;1", raw) }

func psImageExe(cnf, exe string, raw bool) []byte {
	const sectors = 24
	data := make([][]byte, sectors)
	for i := range data {
		data[i] = make([]byte, 2048)
	}
	pvd := data[16]
	copy(pvd, "\x01CD001")
	root := pvd[156:]
	root[0] = 34
	binary.LittleEndian.PutUint32(root[2:], 20)    // root directory at sector 20
	binary.LittleEndian.PutUint32(root[10:], 2048) // one sector long
	dir := data[20]
	rec := func(p int, name string, lba uint32) int {
		n := 33 + len(name)
		if n%2 == 1 {
			n++
		}
		dir[p] = byte(n)
		binary.LittleEndian.PutUint32(dir[p+2:], lba)
		dir[p+32] = byte(len(name))
		copy(dir[p+33:], name)
		return p + n
	}
	p := rec(0, "\x00", 20)
	p = rec(p, exe, 22)
	rec(p, "SYSTEM.CNF;1", 21)
	copy(data[21], cnf)

	var out []byte
	for _, s := range data {
		if raw {
			sec := make([]byte, 2352)
			copy(sec[24:], s)
			out = append(out, sec...)
		} else {
			out = append(out, s...)
		}
	}
	return out
}

func TestReadPlayStationSerial(t *testing.T) {
	cases := []struct {
		name, cnf, want string
		raw             bool
	}{
		{"PS1 bin", "BOOT = cdrom:" + `\` + "SLUS_005.94;1\r\nTCB = 4\r\n", "SLUS-00594", true},
		{"PS1 iso", "BOOT=cdrom:" + `\` + "SCES_021.05;1\n", "SCES-02105", false},
		{"PS2 iso", "BOOT2 = cdrom0:" + `\` + "SLUS_200.62;1\nVER = 1.00\n", "SLUS-20062", false},
		{"subfolder", "BOOT = cdrom:" + `\GAME\` + "SLPS_011.11;1\n", "SLPS-01111", true},
	}
	for _, c := range cases {
		img := psImage(c.cnf, c.raw)
		got, ok := ReadPlayStation(bytes.NewReader(img), int64(len(img)))
		if !ok || got != c.want {
			t.Errorf("%s: %q %v, want %q", c.name, got, ok, c.want)
		}
		// The streaming path (archive members) finds it too.
		name := "game.iso"
		if c.raw {
			name = "game.bin"
		}
		if info, ok := ReadAny(name, bytes.NewReader(img)); !ok || info.ID != c.want || !IsPlayStationSerial(info.ID) {
			t.Errorf("%s via ReadAny: %+v %v", c.name, info, ok)
		}
	}
}

func TestReadPlayStationRejectsOtherFiles(t *testing.T) {
	for name, data := range map[string][]byte{
		"cartridge bin": bytes.Repeat([]byte{0xFF}, 1<<16),
		"no SYSTEM.CNF": psImage("", false)[:20*2048],
		"PC CD":         psImage("not a boot file", false),
	} {
		if s, ok := ReadPlayStation(bytes.NewReader(data), int64(len(data))); ok {
			t.Errorf("%s: %q", name, s)
		}
	}
	if IsPlayStationSerial("GEXE01") {
		t.Error("a GameCube ID is not a PlayStation serial")
	}
}

// When SYSTEM.CNF lies beyond what was read (a PS2 disc streamed from an
// archive), the boot executable's name in the root directory gives the serial.
func TestSerialFromTheBootFileNameWhenSYSTEMCNFIsOutOfReach(t *testing.T) {
	img := psImageExe("BOOT2 = cdrom0:"+`\`+"SLES_123.45;1\n", "SLES_123.45;1", false)
	head := img[:21*2048] // up to the root directory; SYSTEM.CNF (sector 21) is cut off
	if got, ok := ReadPlayStation(bytes.NewReader(head), int64(len(head))); !ok || got != "SLES-12345" {
		t.Errorf("got %q %v", got, ok)
	}
}
