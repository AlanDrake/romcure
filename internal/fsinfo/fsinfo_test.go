package fsinfo

import "testing"

func TestFilesystemTypeStringAndCoWCapable(t *testing.T) {
	tests := []struct {
		name       string
		fsType     FilesystemType
		wantString string
		wantCoW    bool
	}{
		{"unknown", Unknown, "unknown", false},
		{"ntfs", NTFS, "NTFS", false},
		{"apfs", APFS, "APFS", true},
		{"ext", Ext, "ext", false},
		{"btrfs", Btrfs, "Btrfs", true},
		{"xfs", XFS, "XFS", true},
		{"refs", ReFS, "ReFS", true},
		{"fat32", FAT32, "FAT32", false},
		{"exfat", ExFAT, "exFAT", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.fsType.String(); got != tt.wantString {
				t.Errorf("String() = %q, want %q", got, tt.wantString)
			}
			if got := tt.fsType.CoWCapable(); got != tt.wantCoW {
				t.Errorf("CoWCapable() = %v, want %v", got, tt.wantCoW)
			}
		})
	}
}
