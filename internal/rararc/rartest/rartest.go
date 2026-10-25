// Package rartest builds RAR archives and finds 7-Zip for tests: no RAR
// writer is available, so a small stored-only RAR5 file is assembled by hand.
package rartest

import (
	"encoding/binary"
	"hash/crc32"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// vint is RAR5's variable-length integer.
func vint(n uint64) []byte {
	var b []byte
	for n >= 0x80 {
		b = append(b, byte(n)|0x80)
		n >>= 7
	}
	return append(b, byte(n))
}

// block builds one RAR5 header block: CRC32, size, then the body.
func block(body []byte) []byte {
	sized := append(vint(uint64(len(body))), body...)
	out := make([]byte, 4, 4+len(sized))
	binary.LittleEndian.PutUint32(out, crc32.ChecksumIEEE(sized))
	return append(out, sized...)
}

// Write writes an archive that stores (does not compress) the given files,
// in the order of names. Enough of the format for 7-Zip to read back.
func Write(t *testing.T, path string, names []string, data map[string][]byte) {
	t.Helper()
	out := []byte("Rar!\x1a\x07\x01\x00")
	out = append(out, block(append(append(vint(1), vint(0)...), vint(0)...))...) // main header, no flags
	for _, name := range names {
		d := data[name]
		body := vint(2)                              // type: file
		body = append(body, vint(0x02)...)           // header flags: has data area
		body = append(body, vint(uint64(len(d)))...) // data size
		body = append(body, vint(0x04)...)           // file flags: CRC32 present
		body = append(body, vint(uint64(len(d)))...) // unpacked size
		body = append(body, vint(0x20)...)           // attributes
		crc := make([]byte, 4)
		binary.LittleEndian.PutUint32(crc, crc32.ChecksumIEEE(d))
		body = append(body, crc...)
		body = append(body, vint(0)...) // compression: version 0, method store
		body = append(body, vint(0)...) // host OS: Windows
		body = append(body, vint(uint64(len(name)))...)
		body = append(body, name...)
		out = append(out, block(body)...)
		out = append(out, d...)
	}
	out = append(out, block(append(append(vint(5), vint(0)...), vint(0)...))...) // end of archive
	if err := os.WriteFile(path, out, 0o644); err != nil {
		t.Fatal(err)
	}
}

// SevenZip returns the path of the development copy of 7-Zip, or skips the test.
func SevenZip(t *testing.T) string {
	t.Helper()
	dir := filepath.Join("..", "..", "tools", "bin", runtime.GOOS+"-"+runtime.GOARCH)
	for _, n := range []string{"7zz", "7z", "7z.exe"} {
		p := filepath.Join(dir, n)
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			abs, _ := filepath.Abs(p)
			return abs
		}
	}
	t.Skip("7-Zip not in tools/bin (run tools/fetch-7zip.sh)")
	return ""
}
