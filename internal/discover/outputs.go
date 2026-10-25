package discover

import (
	"os"
	"path/filepath"
	"runtime"
)

// Suggestion is a place an output could go, found on this computer.
type Suggestion struct {
	Kind  string // an output kind: "esde", "retroarch", "device"
	Label string
	Path  string // the folder to create the output in
	Note  string
}

func exists(p string) bool {
	st, err := os.Stat(p)
	return err == nil && st.IsDir()
}

// Outputs looks for installed frontends and removable drives and proposes an
// output folder for each.
func Outputs() []Suggestion {
	var out []Suggestion
	home, _ := os.UserHomeDir()

	for _, p := range []string{filepath.Join(home, "ES-DE", "ROMs"), filepath.Join(home, "EmulationStation-DE", "ROMs")} {
		if exists(p) {
			out = append(out, Suggestion{Kind: "esde", Label: "ES-DE", Path: p, Note: "Your ES-DE games folder"})
			break
		}
	}

	var ra []string
	switch runtime.GOOS {
	case "windows":
		ra = []string{filepath.Join(os.Getenv("APPDATA"), "RetroArch"), `C:\RetroArch`, `C:\RetroArch-Win64`, filepath.Join(os.Getenv("ProgramFiles"), "RetroArch")}
	case "darwin":
		ra = []string{filepath.Join(home, "Library", "Application Support", "RetroArch")}
	default:
		ra = []string{filepath.Join(home, ".config", "retroarch"), filepath.Join(home, ".var", "app", "org.libretro.RetroArch")}
	}
	for _, p := range ra {
		if p != "" && exists(p) {
			out = append(out, Suggestion{Kind: "retroarch", Label: "RetroArch", Path: filepath.Join(home, "RomCure", "RetroArch"),
				Note: "RetroArch found at " + p + ". Point its playlist and content folders at this one."})
			break
		}
	}

	for _, drive := range removableDrives() {
		out = append(out, Suggestion{Kind: "device", Label: "SD card or USB drive " + drive, Path: filepath.Join(drive, "Roms"),
			Note: "Games are copied, since these drives can't hold links"})
	}
	return out
}
