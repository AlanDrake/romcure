package discover

import (
	"path/filepath"
	"unsafe"

	"golang.org/x/sys/windows"
)

// runningRetroArchDirs returns the folders of the retroarch*.exe processes
// that are open now, through a process snapshot (no console window, no
// subprocess).
func runningRetroArchDirs() []string {
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return nil
	}
	defer windows.CloseHandle(snap)
	var e windows.ProcessEntry32
	e.Size = uint32(unsafe.Sizeof(e))
	var dirs []string
	for err := windows.Process32First(snap, &e); err == nil; err = windows.Process32Next(snap, &e) {
		if !isRetroArchExe(windows.UTF16ToString(e.ExeFile[:])) {
			continue
		}
		// Limited rights are enough to read the image path, even of a process
		// started by another user or elevated.
		h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, e.ProcessID)
		if err != nil {
			continue
		}
		buf := make([]uint16, windows.MAX_PATH*2)
		n := uint32(len(buf))
		if windows.QueryFullProcessImageName(h, 0, &buf[0], &n) == nil {
			dirs = append(dirs, filepath.Dir(windows.UTF16ToString(buf[:n])))
		}
		windows.CloseHandle(h)
	}
	return dirs
}
