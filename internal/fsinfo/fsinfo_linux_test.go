//go:build linux

package fsinfo

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseLinuxFSType(t *testing.T) {
	tests := []struct {
		raw  string
		want FilesystemType
	}{
		{"ext4", Ext},
		{"ext3", Ext},
		{"btrfs", Btrfs},
		{"xfs", XFS},
		{"vfat", FAT32},
		{"exfat", ExFAT},
		{"ntfs3", NTFS},
		{"", Unknown},
		{"tmpfs", Unknown},
	}
	for _, tt := range tests {
		t.Run(tt.raw, func(t *testing.T) {
			if got := parseLinuxFSType(tt.raw); got != tt.want {
				t.Errorf("parseLinuxFSType(%q) = %v, want %v", tt.raw, got, tt.want)
			}
		})
	}
}

func TestRemoteFSTypes(t *testing.T) {
	tests := []struct {
		raw  string
		want bool
	}{
		{"nfs", true},
		{"nfs4", true},
		{"cifs", true},
		{"smb3", true},
		{"ext4", false},
		{"btrfs", false},
		{"tmpfs", false},
	}
	for _, tt := range tests {
		t.Run(tt.raw, func(t *testing.T) {
			if got := remoteFSTypes[tt.raw]; got != tt.want {
				t.Errorf("remoteFSTypes[%q] = %v, want %v", tt.raw, got, tt.want)
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
		t.Errorf("Detect(%s).Raw is empty, want the mountinfo filesystem type", dir)
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

func TestSameVolumeAcrossMounts(t *testing.T) {
	// /dev/shm is tmpfs and almost always a separate mount from /tmp's
	// underlying filesystem; skip rather than fail if that's not true.
	const shm = "/dev/shm"
	if info, err := os.Stat(shm); err != nil || !info.IsDir() {
		t.Skip("/dev/shm not available on this machine")
	}

	dir := t.TempDir()
	same, err := SameVolume(dir, shm)
	if err != nil {
		t.Fatalf("SameVolume: %v", err)
	}
	if same {
		t.Skip("/dev/shm turned out to share a mount with the temp dir on this machine")
	}
}

func TestPickMountRootAndNested(t *testing.T) {
	entries := []mountEntry{
		{mountPoint: "/", fsType: "ext4"},
		{mountPoint: "/mnt/data", fsType: "ntfs3"},
		{mountPoint: "/mnt/data2", fsType: "cifs"},
	}
	for path, want := range map[string]string{
		"/tmp/x":            "ext4", // only the root mount covers this
		"/":                 "ext4",
		"/mnt/data":         "ntfs3",
		"/mnt/data/games/a": "ntfs3",
		"/mnt/data2/b":      "cifs", // not confused with /mnt/data
		"/mnt/other":        "ext4",
	} {
		got, err := pickMount(entries, path)
		if err != nil || got.fsType != want {
			t.Errorf("pickMount(%q) = %+v err=%v, want %s", path, got, err, want)
		}
	}
	if _, err := pickMount([]mountEntry{{mountPoint: "/mnt/x", fsType: "ext4"}}, "/tmp"); err == nil {
		t.Error("no covering mount must be an error")
	}
}
