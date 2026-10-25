package discover

import (
	"bufio"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"
)

// RetroArchInstall is a RetroArch configuration found on this computer: the
// folder holding the retroarch.cfg that RetroArch loads, and what it says.
type RetroArchInstall struct {
	Dir       string    // the folder holding retroarch.cfg
	Config    string    // path of retroarch.cfg
	Playlists string    // the playlist folder, with defaults and ":\" resolved
	Cores     string    // the core folder, resolved the same way
	Used      time.Time // when retroarch.cfg last changed (RetroArch saves it on exit)
	Running   bool      // set by the caller when this install's RetroArch is open
}

// ParseRetroArchConfig reads retroarch.cfg (key = "value" lines) and resolves
// playlist_directory and libretro_directory. dir is the folder holding the
// config: ":\" in a value means that folder, and a blank or "default"
// playlist folder means <dir>/playlists.
func ParseRetroArchConfig(path string) (RetroArchInstall, error) {
	f, err := os.Open(path)
	if err != nil {
		return RetroArchInstall{}, err
	}
	defer f.Close()
	dir := filepath.Dir(path)
	in := RetroArchInstall{Dir: dir, Config: path}
	if st, err := f.Stat(); err == nil {
		in.Used = st.ModTime()
	}
	var playlists, cores string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		k, v, ok := strings.Cut(sc.Text(), "=")
		if !ok {
			continue
		}
		v = strings.Trim(strings.TrimSpace(v), `"`)
		switch strings.TrimSpace(k) {
		case "playlist_directory":
			playlists = v
		case "libretro_directory":
			cores = v
		}
	}
	if err := sc.Err(); err != nil {
		return RetroArchInstall{}, err
	}
	in.Playlists = resolveRetroArchDir(dir, playlists, "playlists")
	in.Cores = resolveRetroArchDir(dir, cores, "cores")
	return in, nil
}

// resolveRetroArchDir turns a config value into a path: "" and "default" are
// the stock sub-folder, and a leading ":" (":\playlists", ":/cores") is the
// RetroArch folder itself.
func resolveRetroArchDir(dir, v, stock string) string {
	if v == "" || v == "default" {
		return filepath.Join(dir, stock)
	}
	if strings.HasPrefix(v, ":") {
		rest := strings.TrimLeft(v[1:], `\/`)
		return filepath.Join(dir, filepath.FromSlash(strings.ReplaceAll(rest, `\`, "/")))
	}
	return filepath.FromSlash(v)
}

// retroArchDirs lists the folders where a RetroArch configuration usually
// lives on this OS. The folder of a portable install can be anywhere; the
// caller adds those from a running process or a Browse choice.
func retroArchDirs(home string) []string {
	switch runtime.GOOS {
	case "windows":
		pf := os.Getenv("ProgramFiles")
		return []string{
			filepath.Join(os.Getenv("APPDATA"), "RetroArch"),
			`C:\RetroArch`, `C:\RetroArch-Win64`,
			filepath.Join(pf, "RetroArch"),
			filepath.Join(pf, "Steam", "steamapps", "common", "RetroArch"),
			filepath.Join(os.Getenv("ProgramFiles(x86)"), "Steam", "steamapps", "common", "RetroArch"),
		}
	case "darwin":
		return []string{filepath.Join(home, "Library", "Application Support", "RetroArch")}
	default:
		return []string{
			filepath.Join(home, ".config", "retroarch"),
			filepath.Join(home, ".var", "app", "org.libretro.RetroArch", "config", "retroarch"),
			filepath.Join(home, "snap", "retroarch", "current", ".config", "retroarch"),
		}
	}
}

// RetroArchInstalls returns every RetroArch configuration found in the usual
// places plus the extra folders given (a running process's folder, a Browse
// choice), most recently used first. Only folders with a readable
// retroarch.cfg count: an empty folder is not an install.
func RetroArchInstalls(extra ...string) []RetroArchInstall {
	home, _ := os.UserHomeDir()
	return retroArchInstallsIn(append(extra, retroArchDirs(home)...))
}

func retroArchInstallsIn(dirs []string) []RetroArchInstall {
	var out []RetroArchInstall
	seen := map[string]bool{}
	for _, d := range dirs {
		if d == "" {
			continue
		}
		cfg := filepath.Join(d, "retroarch.cfg")
		key := strings.ToLower(filepath.Clean(cfg))
		if seen[key] {
			continue
		}
		seen[key] = true
		if in, err := ParseRetroArchConfig(cfg); err == nil {
			out = append(out, in)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Used.After(out[j].Used) })
	return out
}
