//go:build windows

package elevate

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

// SymlinkProbe reports whether the current process can create a symlink
// right now, by actually attempting one in a scratch location (and
// removing it). A false result with a nil error means "not right now, but
// nothing went wrong" — e.g. no privilege and Developer Mode is off.
func SymlinkProbe() (bool, error) {
	dir := os.TempDir()
	target := filepath.Join(dir, fmt.Sprintf(".romcure-elevate-probe-target-%d", os.Getpid()))
	link := filepath.Join(dir, fmt.Sprintf(".romcure-elevate-probe-link-%d", os.Getpid()))

	if err := os.WriteFile(target, nil, 0o644); err != nil {
		return false, fmt.Errorf("create probe target: %w", err)
	}
	defer os.Remove(target)

	if err := os.Symlink(target, link); err != nil {
		return false, nil
	}
	os.Remove(link)
	return true, nil
}

// IsDevModeEnabled reports whether Windows Developer Mode is on, which
// relaxes the SeCreateSymbolicLinkPrivilege requirement for unprivileged
// symlink creation.
func IsDevModeEnabled() (bool, error) {
	key, err := registry.OpenKey(registry.LOCAL_MACHINE, `SOFTWARE\Microsoft\Windows\CurrentVersion\AppModelUnlock`, registry.QUERY_VALUE)
	if err != nil {
		if errors.Is(err, registry.ErrNotExist) {
			return false, nil
		}
		return false, fmt.Errorf("open AppModelUnlock key: %w", err)
	}
	defer key.Close()

	val, _, err := key.GetIntegerValue("AllowDevelopmentWithoutDevLicense")
	if err != nil {
		if errors.Is(err, registry.ErrNotExist) {
			return false, nil
		}
		return false, fmt.Errorf("read AllowDevelopmentWithoutDevLicense: %w", err)
	}
	return val != 0, nil
}

// IsElevated reports whether the current process token is elevated
// (running as administrator).
func IsElevated() (bool, error) {
	return windows.GetCurrentProcessToken().IsElevated(), nil
}

// RelaunchElevated relaunches the current executable with the same
// arguments via ShellExecute's "runas" verb, prompting UAC. On success
// (err == nil) a new elevated process is on its way up; the caller should
// exit so the two don't both run. If the user declines the prompt,
// RelaunchElevated returns ErrUACDeclined.
func RelaunchElevated() error {
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolve current executable: %w", err)
	}
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("resolve working directory: %w", err)
	}

	verbPtr, err := windows.UTF16PtrFromString("runas")
	if err != nil {
		return fmt.Errorf("encode verb: %w", err)
	}
	exePtr, err := windows.UTF16PtrFromString(exe)
	if err != nil {
		return fmt.Errorf("encode executable path: %w", err)
	}
	argsPtr, err := windows.UTF16PtrFromString(joinArgs(os.Args[1:]))
	if err != nil {
		return fmt.Errorf("encode arguments: %w", err)
	}
	cwdPtr, err := windows.UTF16PtrFromString(cwd)
	if err != nil {
		return fmt.Errorf("encode working directory: %w", err)
	}

	if err := windows.ShellExecute(0, verbPtr, exePtr, argsPtr, cwdPtr, windows.SW_NORMAL); err != nil {
		if errors.Is(err, windows.ERROR_CANCELLED) {
			return ErrUACDeclined
		}
		return fmt.Errorf("ShellExecute runas %s: %w", exe, err)
	}
	return nil
}

// joinArgs quotes arguments containing whitespace or quotes for the
// command line ShellExecute passes to CreateProcess. Minimal today since
// the app takes no CLI arguments, but correct once it does.
func joinArgs(args []string) string {
	var b strings.Builder
	for i, a := range args {
		if i > 0 {
			b.WriteByte(' ')
		}
		if strings.ContainsAny(a, " \t\"") {
			b.WriteByte('"')
			b.WriteString(strings.ReplaceAll(a, `"`, `\"`))
			b.WriteByte('"')
		} else {
			b.WriteString(a)
		}
	}
	return b.String()
}
