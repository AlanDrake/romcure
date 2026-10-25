// Package support builds the diagnostics bundle a user can attach to a bug
// report (spec M6 "log bundle export").
package support

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"
)

// maxLogBytes bounds how much of the log goes into the bundle (the tail,
// which is where the recent problem is).
const maxLogBytes = 5 << 20

// Redact replaces the user's home directory (and so their account name) in
// text with "~", in both slash styles, so a bundle can be shared without
// leaking personal paths. An empty home leaves text unchanged.
func Redact(text, home string) string {
	if home == "" {
		return text
	}
	for _, h := range []string{home, strings.ReplaceAll(home, `\`, `/`), strings.ReplaceAll(home, `\`, `\\`)} {
		text = strings.ReplaceAll(text, h, "~")
	}
	return text
}

// ExportBundle writes a zip at dest containing, from appDataDir, the tail of
// romcure.log and settings.json (both redacted), plus system.txt with the
// OS/arch and the caller-supplied info (tool versions, app version...).
// Library contents, ROM names and the database are never included.
func ExportBundle(appDataDir, dest string, info map[string]string) error {
	home, _ := os.UserHomeDir()

	tmp := dest + ".tmp"
	out, err := os.Create(tmp)
	if err != nil {
		return err
	}
	zw := zip.NewWriter(out)
	fail := func(err error) error {
		zw.Close()
		out.Close()
		os.Remove(tmp)
		return err
	}

	add := func(name, content string) error {
		w, err := zw.Create(name)
		if err != nil {
			return err
		}
		_, err = io.WriteString(w, Redact(content, home))
		return err
	}

	if logTail, err := readTail(filepath.Join(appDataDir, "romcure.log"), maxLogBytes); err == nil {
		if err := add("romcure.log", logTail); err != nil {
			return fail(err)
		}
	} else if !os.IsNotExist(err) {
		return fail(err)
	}
	if b, err := os.ReadFile(filepath.Join(appDataDir, "settings.json")); err == nil {
		if err := add("settings.json", string(b)); err != nil {
			return fail(err)
		}
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "generated: %s\nos: %s/%s\ngo: %s\n", time.Now().UTC().Format(time.RFC3339), runtime.GOOS, runtime.GOARCH, runtime.Version())
	keys := make([]string, 0, len(info))
	for k := range info {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		fmt.Fprintf(&sb, "%s: %s\n", k, info[k])
	}
	if err := add("system.txt", sb.String()); err != nil {
		return fail(err)
	}

	if err := zw.Close(); err != nil {
		return fail(err)
	}
	if err := out.Close(); err != nil {
		os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, dest)
}

// readTail returns at most max bytes from the end of path, starting at a line
// boundary when it had to cut.
func readTail(path string, max int64) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return "", err
	}
	cut := st.Size() > max
	if cut {
		if _, err := f.Seek(-max, io.SeekEnd); err != nil {
			return "", err
		}
	}
	b, err := io.ReadAll(f)
	if err != nil {
		return "", err
	}
	s := string(b)
	if cut {
		if i := strings.IndexByte(s, '\n'); i >= 0 {
			s = s[i+1:]
		}
		s = "[log truncated to the last part]\n" + s
	}
	return s, nil
}
