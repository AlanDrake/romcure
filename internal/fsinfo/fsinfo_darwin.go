//go:build darwin

package fsinfo

import (
	"bytes"
	"fmt"
	"path/filepath"

	"golang.org/x/sys/unix"
)

func parseDarwinFSType(name string) FilesystemType {
	switch name {
	case "ntfs":
		return NTFS
	case "apfs":
		return APFS
	case "msdos":
		return FAT32
	case "exfat":
		return ExFAT
	default:
		return Unknown
	}
}

// nulTerminatedString converts a NUL-terminated byte buffer (as used by
// Darwin's statfs_t string fields) to a Go string.
func nulTerminatedString(b []byte) string {
	if i := bytes.IndexByte(b, 0); i >= 0 {
		b = b[:i]
	}
	return string(b)
}

func detect(path string) (Info, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return Info{}, fmt.Errorf("resolve absolute path %s: %w", path, err)
	}

	var stat unix.Statfs_t
	if err := unix.Statfs(abs, &stat); err != nil {
		return Info{}, fmt.Errorf("statfs %s: %w", abs, err)
	}

	fsType := nulTerminatedString(stat.Fstypename[:])

	return Info{
		Type: parseDarwinFSType(fsType),
		Raw:  fsType,
		// MNT_LOCAL is unset for network mounts (nfs, smbfs, afpfs, ...);
		// this also covers FUSE-backed network filesystems, unlike
		// matching on the type name.
		IsRemote:  stat.Flags&unix.MNT_LOCAL == 0,
		FreeBytes: uint64(stat.Bsize) * stat.Bavail,
	}, nil
}

func sameVolume(a, b string) (bool, error) {
	absA, err := filepath.Abs(a)
	if err != nil {
		return false, fmt.Errorf("resolve absolute path %s: %w", a, err)
	}
	absB, err := filepath.Abs(b)
	if err != nil {
		return false, fmt.Errorf("resolve absolute path %s: %w", b, err)
	}

	var stA, stB unix.Stat_t
	if err := unix.Stat(absA, &stA); err != nil {
		return false, fmt.Errorf("stat %s: %w", absA, err)
	}
	if err := unix.Stat(absB, &stB); err != nil {
		return false, fmt.Errorf("stat %s: %w", absB, err)
	}
	return stA.Dev == stB.Dev, nil
}
