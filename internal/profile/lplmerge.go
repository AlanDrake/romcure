package profile

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// Merging into a playlist RetroArch already has (spec M8, option A): the
// entries whose path lies inside this output's folder are ours and are
// rewritten; every other entry, and every header field RetroArch or the user
// set (default core, sort mode, ...), is kept exactly as it was read.

// samePathKey makes paths comparable: slashes unified and, on Windows, case
// folded, since RetroArch writes the paths it was given with either slash.
func samePathKey(p string) string {
	p = strings.ReplaceAll(p, `\`, "/")
	if runtime.GOOS == "windows" {
		p = strings.ToLower(p)
	}
	return strings.TrimRight(p, "/")
}

// inside reports whether an item's path is the folder or something below it.
func inside(itemPath, folder string) bool {
	p, f := samePathKey(itemPath), samePathKey(folder)
	return f != "" && strings.HasPrefix(p, f+"/")
}

// mergePlaylist writes ours into the playlist at path, replacing the entries
// of this output (those inside outputRoot) and leaving all others alone. It
// returns whether the file changed: nothing is written when the result is the
// same bytes, and a playlist that does not exist is not created for no items.
func mergePlaylist(path, outputRoot string, ours []lplItem, core defaultCore) (bool, error) {
	old, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return false, err
	}
	next, err := mergePlaylistBytes(old, outputRoot, ours, core)
	if err != nil {
		return false, fmt.Errorf("playlist %s: %w", path, err)
	}
	if next == nil || bytes.Equal(old, next) {
		return false, nil
	}
	return true, writeFileAtomic(path, next)
}

// removeFromPlaylists takes this output's entries out of every playlist in dir
// (an output or a system was removed). Playlists with no entry of ours are not
// touched. It returns how many playlists changed.
func removeFromPlaylists(dir, outputRoot string) (int, error) {
	files, _ := filepath.Glob(filepath.Join(dir, "*.lpl"))
	changed := 0
	for _, f := range files {
		ok, err := mergePlaylist(f, outputRoot, nil, defaultCore{})
		if err != nil {
			return changed, err
		}
		if ok {
			changed++
		}
	}
	return changed, nil
}

// mergePlaylistBytes is the pure part of mergePlaylist. A nil result means
// there is nothing to write.
func mergePlaylistBytes(old []byte, outputRoot string, ours []lplItem, core defaultCore) ([]byte, error) {
	header := map[string]json.RawMessage{}
	var kept []json.RawMessage
	if len(bytes.TrimSpace(old)) > 0 {
		if err := json.Unmarshal(old, &header); err != nil {
			return nil, fmt.Errorf("not a RetroArch playlist: %w", err)
		}
		var items []json.RawMessage
		if raw, ok := header["items"]; ok {
			if err := json.Unmarshal(raw, &items); err != nil {
				return nil, fmt.Errorf("items: %w", err)
			}
		}
		removed := 0
		for _, it := range items {
			var p struct {
				Path string `json:"path"`
			}
			if json.Unmarshal(it, &p) == nil && inside(p.Path, outputRoot) {
				removed++
				continue
			}
			kept = append(kept, it)
		}
		if removed == 0 && len(ours) == 0 {
			return nil, nil // none of it is ours: leave the user's file as it is
		}
	} else {
		if len(ours) == 0 {
			return nil, nil
		}
		def := lplFile{Version: "1.5"}
		raw, _ := json.Marshal(def)
		_ = json.Unmarshal(raw, &header)
		delete(header, "items")
	}
	for _, it := range ours {
		raw, err := json.Marshal(it)
		if err != nil {
			return nil, err
		}
		kept = append(kept, raw)
	}
	if kept == nil {
		kept = []json.RawMessage{}
	}
	raw, err := json.Marshal(kept)
	if err != nil {
		return nil, err
	}
	header["items"] = raw
	core.fillIn(header)
	// json.Marshal sorts a map's keys, so the output is the same every time.
	return json.MarshalIndent(header, "", "  ")
}

// defaultCore is the core a new playlist should open its games with. The zero
// value means none.
type defaultCore struct{ Path, Name string }

// fillIn sets the playlist's default core, but only where it has none: a core
// the user picked in RetroArch is never replaced.
func (c defaultCore) fillIn(header map[string]json.RawMessage) {
	if c.Path == "" {
		return
	}
	var cur string
	_ = json.Unmarshal(header["default_core_path"], &cur)
	if cur != "" && cur != "DETECT" {
		return
	}
	path, _ := json.Marshal(c.Path)
	name, _ := json.Marshal(c.Name)
	header["default_core_path"], header["default_core_name"] = path, name
}

// writeFileAtomic replaces path through a temporary file beside it, so a
// reader (RetroArch) never sees half a playlist.
func writeFileAtomic(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".romcure-*.tmp")
	if err != nil {
		return err
	}
	_, werr := tmp.Write(data)
	cerr := tmp.Close()
	if werr != nil || cerr != nil {
		os.Remove(tmp.Name())
		return errors.Join(werr, cerr)
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	return nil
}

// RemoveFromPlaylists is removeFromPlaylists for the app: it takes an output's
// entries out of RetroArch's playlist folder.
func RemoveFromPlaylists(dir, outputRoot string) (int, error) {
	return removeFromPlaylists(dir, outputRoot)
}
