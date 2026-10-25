//go:build darwin

package fsinfo

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseDarwinFSType(t *testing.T) {
	tests := []struct {
		raw  string
		want FilesystemType
	}{
		{"apfs", APFS},
		{"ntfs", NTFS},
		{"msdos", FAT32},
		{"exfat", ExFAT},
		{"", Unknown},
		{"hfs", Unknown},
		{"nfs", Unknown},
		{"smbfs", Unknown},
	}
	for _, tt := range tests {
		t.Run(tt.raw, func(t *testing.T) {
			if got := parseDarwinFSType(tt.raw); got != tt.want {
				t.Errorf("parseDarwinFSType(%q) = %v, want %v", tt.raw, got, tt.want)
			}
		})
	}
}

func TestNulTerminatedString(t *testing.T) {
	tests := []struct {
		name string
		in   []byte
		want string
	}{
		{"no NUL", []byte("apfs"), "apfs"},
		{"NUL-padded", append([]byte("apfs"), make([]byte, 12)...), "apfs"},
		{"empty", []byte{}, ""},
		{"all NUL", make([]byte, 16), ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := nulTerminatedString(tt.in); got != tt.want {
				t.Errorf("nulTerminatedString(%v) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestDetectLocalTempDir(t *testing.T) {
	dir := t.TempDir()

	info, err := Detect(dir)
	if err != nil {
		t.Fatalf("Detect(%s): %v", dir, err)
	}
	if info.IsRemote {
		t.Errorf("Detect(%s).IsRemote = true, want false for a local temp dir", dir)
	}
	if info.Raw == "" {
		t.Errorf("Detect(%s).Raw is empty, want the statfs fstypename", dir)
	}
}

func TestSameVolume(t *testing.T) {
	dir := t.TempDir()
	fileA := filepath.Join(dir, "a.txt")
	fileB := filepath.Join(dir, "sub", "b.txt")

	if err := os.WriteFile(fileA, []byte("a"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(fileB), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(fileB, []byte("b"), 0o644); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name string
		a, b string
		want bool
	}{
		{"same dir, two files", fileA, fileB, true},
		{"dir vs file within it", dir, fileA, true},
		{"path vs itself", fileA, fileA, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := SameVolume(tt.a, tt.b)
			if err != nil {
				t.Fatalf("SameVolume(%s, %s): %v", tt.a, tt.b, err)
			}
			if got != tt.want {
				t.Errorf("SameVolume(%s, %s) = %v, want %v", tt.a, tt.b, got, tt.want)
			}
		})
	}
}
