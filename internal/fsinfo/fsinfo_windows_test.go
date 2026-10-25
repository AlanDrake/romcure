//go:build windows

package fsinfo

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseWindowsFSType(t *testing.T) {
	tests := []struct {
		raw  string
		want FilesystemType
	}{
		{"NTFS", NTFS},
		{"FAT32", FAT32},
		{"exFAT", ExFAT},
		{"ReFS", ReFS},
		{"", Unknown},
		{"udf", Unknown},
	}
	for _, tt := range tests {
		t.Run(tt.raw, func(t *testing.T) {
			if got := parseWindowsFSType(tt.raw); got != tt.want {
				t.Errorf("parseWindowsFSType(%q) = %v, want %v", tt.raw, got, tt.want)
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
	if info.FreeBytes == 0 {
		t.Errorf("Detect(%s).FreeBytes = 0, want > 0", dir)
	}
	if info.Type == Unknown {
		t.Errorf("Detect(%s).Type = Unknown (raw %q), want a recognized Windows filesystem", dir, info.Raw)
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

func TestSameVolumeAcrossDriveLetters(t *testing.T) {
	// Only meaningful if the CI/dev machine has more than one drive letter;
	// skip rather than fail when it doesn't.
	var second string
	for _, letter := range "DEFGH" {
		root := string(letter) + `:\`
		if info, err := os.Stat(root); err == nil && info.IsDir() {
			second = root
			break
		}
	}
	if second == "" {
		t.Skip("no second drive letter available on this machine")
	}

	same, err := SameVolume(`C:\`, second)
	if err != nil {
		t.Fatalf("SameVolume: %v", err)
	}
	if same {
		t.Errorf("SameVolume(C:\\, %s) = true, want false (different drive letters)", second)
	}
}
