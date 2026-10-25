//go:build windows

package fsinfo

import (
	"fmt"
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows"
)

func parseWindowsFSType(name string) FilesystemType {
	switch name {
	case "NTFS":
		return NTFS
	case "FAT32":
		return FAT32
	case "exFAT":
		return ExFAT
	case "ReFS":
		return ReFS
	default:
		return Unknown
	}
}

// volumeRoot returns the mount point (e.g. "C:\" or "\\server\share\")
// containing absPath, as reported by GetVolumePathName.
func volumeRoot(absPath string) (string, error) {
	pathPtr, err := windows.UTF16PtrFromString(absPath)
	if err != nil {
		return "", fmt.Errorf("encode path %s: %w", absPath, err)
	}

	buf := make([]uint16, 260)
	if err := windows.GetVolumePathName(pathPtr, &buf[0], uint32(len(buf))); err != nil {
		return "", fmt.Errorf("GetVolumePathName(%s): %w", absPath, err)
	}
	return windows.UTF16ToString(buf), nil
}

func detect(path string) (Info, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return Info{}, fmt.Errorf("resolve absolute path %s: %w", path, err)
	}

	root, err := volumeRoot(abs)
	if err != nil {
		return Info{}, err
	}
	rootPtr, err := windows.UTF16PtrFromString(root)
	if err != nil {
		return Info{}, fmt.Errorf("encode volume root %s: %w", root, err)
	}

	driveType := windows.GetDriveType(rootPtr)
	isRemote := driveType == windows.DRIVE_REMOTE

	var volNameBuf, fsNameBuf [260]uint16
	var serial, maxCompLen, fsFlags uint32
	if err := windows.GetVolumeInformation(
		rootPtr,
		&volNameBuf[0], uint32(len(volNameBuf)),
		&serial, &maxCompLen, &fsFlags,
		&fsNameBuf[0], uint32(len(fsNameBuf)),
	); err != nil {
		return Info{}, fmt.Errorf("GetVolumeInformation(%s): %w", root, err)
	}
	fsName := windows.UTF16ToString(fsNameBuf[:])

	var freeBytesAvail, totalBytes, totalFreeBytes uint64
	if err := windows.GetDiskFreeSpaceEx(rootPtr, &freeBytesAvail, &totalBytes, &totalFreeBytes); err != nil {
		return Info{}, fmt.Errorf("GetDiskFreeSpaceEx(%s): %w", root, err)
	}

	return Info{
		Type:      parseWindowsFSType(fsName),
		Raw:       fsName,
		IsRemote:  isRemote,
		FreeBytes: freeBytesAvail,
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

	rootA, err := volumeRoot(absA)
	if err != nil {
		return false, err
	}
	rootB, err := volumeRoot(absB)
	if err != nil {
		return false, err
	}
	return strings.EqualFold(rootA, rootB), nil
}
