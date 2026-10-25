// Package fsinfo detects destination filesystem type (remote/FAT32/exFAT/
// CoW-capable/etc.), same-volume identity, and free space. See spec §2.4.
package fsinfo

// FilesystemType identifies a filesystem's on-disk format, as far as this
// app needs to distinguish (spec §2.2, §2.4). Types not in this list (e.g.
// HFS+) report Unknown; Info.Raw carries the OS-reported name regardless.
type FilesystemType int

const (
	Unknown FilesystemType = iota
	NTFS
	APFS
	Ext
	Btrfs
	XFS
	ReFS
	FAT32
	ExFAT
)

func (t FilesystemType) String() string {
	switch t {
	case NTFS:
		return "NTFS"
	case APFS:
		return "APFS"
	case Ext:
		return "ext"
	case Btrfs:
		return "Btrfs"
	case XFS:
		return "XFS"
	case ReFS:
		return "ReFS"
	case FAT32:
		return "FAT32"
	case ExFAT:
		return "exFAT"
	default:
		return "unknown"
	}
}

// CoWCapable reports whether this filesystem type supports copy-on-write
// clones, making the reflink tier (spec §2.2) available.
func (t FilesystemType) CoWCapable() bool {
	switch t {
	case APFS, Btrfs, XFS, ReFS:
		return true
	default:
		return false
	}
}

// Info describes the filesystem containing a path, for link-tier selection
// (spec §2.2) and destination classification (spec §2.4).
type Info struct {
	Type FilesystemType
	// Raw is the OS-reported filesystem name (e.g. "ntfs3", "smbfs"),
	// kept for diagnostics even when Type is Unknown.
	Raw string
	// IsRemote is true for a network share: a UNC path, an SMB/NFS mount,
	// or a mapped network drive. Such destinations are blocked (spec §2.4)
	// regardless of Type.
	IsRemote bool
	// FreeBytes is the space available to the current user on the volume
	// containing the detected path.
	FreeBytes uint64
}

// Detect reports filesystem info for the volume/mount containing path.
// path must exist. Implemented per-OS in fsinfo_<os>.go.
func Detect(path string) (Info, error) {
	return detect(path)
}

// SameVolume reports whether a and b reside on the same volume/mount. This
// governs the relative-vs-absolute symlink choice and hardlink/reflink
// eligibility (spec §2.2). Both paths must exist. Implemented per-OS in
// fsinfo_<os>.go.
func SameVolume(a, b string) (bool, error) {
	return sameVolume(a, b)
}
