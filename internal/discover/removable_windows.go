package discover

import (
	"fmt"

	"golang.org/x/sys/windows"
)

// removableDrives lists the roots of removable drives (SD cards, USB sticks).
func removableDrives() []string {
	mask, err := windows.GetLogicalDrives()
	if err != nil {
		return nil
	}
	var out []string
	for i := 0; i < 26; i++ {
		if mask&(1<<uint(i)) == 0 {
			continue
		}
		root := fmt.Sprintf("%c:\\", 'A'+i)
		p, err := windows.UTF16PtrFromString(root)
		if err != nil {
			continue
		}
		if windows.GetDriveType(p) == windows.DRIVE_REMOVABLE {
			out = append(out, root)
		}
	}
	return out
}
