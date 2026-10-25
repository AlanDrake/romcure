//go:build linux

package fsinfo

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"golang.org/x/sys/unix"
)

// remoteFSTypes are /proc mountinfo filesystem-type strings that indicate a
// network mount (spec §2.4). statfs's f_type magic number is unreliable for
// this: FUSE-backed network filesystems (e.g. sshfs) all report
// FUSE_SUPER_MAGIC regardless of what's behind them, so we read the
// filesystem type string from mountinfo instead.
var remoteFSTypes = map[string]bool{
	"nfs": true, "nfs4": true,
	"cifs": true, "smb3": true, "smbfs": true,
	"9p": true, "afs": true, "ncpfs": true,
	"fuse.sshfs": true,
}

func parseLinuxFSType(name string) FilesystemType {
	switch name {
	case "ntfs", "ntfs3":
		return NTFS
	case "ext2", "ext3", "ext4":
		return Ext
	case "btrfs":
		return Btrfs
	case "xfs":
		return XFS
	case "vfat", "msdos", "fat":
		return FAT32
	case "exfat":
		return ExFAT
	default:
		return Unknown
	}
}

type mountEntry struct {
	mountPoint string
	fsType     string
}

// mountEntries parses /proc/self/mountinfo. Format (see proc(5)):
// ID PARENT-ID MAJOR:MINOR ROOT MOUNT-POINT OPTIONS... - FSTYPE SOURCE SUPER-OPTIONS
func mountEntries() ([]mountEntry, error) {
	f, err := os.Open("/proc/self/mountinfo")
	if err != nil {
		return nil, fmt.Errorf("open /proc/self/mountinfo: %w", err)
	}
	defer f.Close()

	var entries []mountEntry
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())

		sep := -1
		for i, field := range fields {
			if field == "-" {
				sep = i
				break
			}
		}
		if sep < 0 || sep+1 >= len(fields) || sep < 5 {
			continue
		}

		entries = append(entries, mountEntry{
			mountPoint: unescapeMountField(fields[4]),
			fsType:     fields[sep+1],
		})
	}
	return entries, scanner.Err()
}

// unescapeMountField undoes the octal escaping mountinfo uses for spaces,
// tabs, newlines, and backslashes in paths.
func unescapeMountField(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+3 < len(s) {
			if n, err := strconv.ParseUint(s[i+1:i+4], 8, 8); err == nil {
				b.WriteByte(byte(n))
				i += 3
				continue
			}
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

// mountForPath returns the mount entry with the longest mount point prefix
// matching absPath — i.e. the innermost mount covering it.
func mountForPath(absPath string) (mountEntry, error) {
	entries, err := mountEntries()
	if err != nil {
		return mountEntry{}, err
	}
	return pickMount(entries, absPath)
}

// covers reports whether the mount at mountPoint contains absPath. The root
// mount "/" contains everything; for others the path must be the mount point
// itself or lie below it (a plain prefix test would also match "/mnt/data2"
// for "/mnt/data").
func covers(mountPoint, absPath string) bool {
	if mountPoint == "/" {
		return strings.HasPrefix(absPath, "/")
	}
	return absPath == mountPoint || strings.HasPrefix(absPath, mountPoint+"/")
}

// pickMount returns the innermost entry covering absPath.
func pickMount(entries []mountEntry, absPath string) (mountEntry, error) {
	var best mountEntry
	bestLen := -1
	for _, e := range entries {
		if covers(e.mountPoint, absPath) && len(e.mountPoint) > bestLen {
			best = e
			bestLen = len(e.mountPoint)
		}
	}
	if bestLen < 0 {
		return mountEntry{}, fmt.Errorf("no mountinfo entry covers %s", absPath)
	}
	return best, nil
}

func detect(path string) (Info, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return Info{}, fmt.Errorf("resolve absolute path %s: %w", path, err)
	}

	mount, err := mountForPath(abs)
	if err != nil {
		return Info{}, err
	}

	var stat unix.Statfs_t
	if err := unix.Statfs(abs, &stat); err != nil {
		return Info{}, fmt.Errorf("statfs %s: %w", abs, err)
	}

	return Info{
		Type:      parseLinuxFSType(mount.fsType),
		Raw:       mount.fsType,
		IsRemote:  remoteFSTypes[mount.fsType],
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
