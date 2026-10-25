package support

import (
	"archive/zip"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func readZip(t *testing.T, path string) map[string]string {
	zr, err := zip.OpenReader(path)
	if err != nil {
		t.Fatal(err)
	}
	defer zr.Close()
	out := map[string]string{}
	for _, f := range zr.File {
		rc, _ := f.Open()
		b, _ := io.ReadAll(rc)
		rc.Close()
		out[f.Name] = string(b)
	}
	return out
}

func TestRedact(t *testing.T) {
	home := `C:\Users\someone`
	in := `error at C:\Users\someone\roms and C:/Users/someone/x and "C:\\Users\\someone\\y"`
	got := Redact(in, home)
	if strings.Contains(got, "someone") {
		t.Errorf("home path leaked: %s", got)
	}
	if Redact("unchanged", "") != "unchanged" {
		t.Error("empty home must be a no-op")
	}
}

func TestExportBundleContentsAndExclusions(t *testing.T) {
	app := t.TempDir()
	home, _ := os.UserHomeDir()
	os.WriteFile(filepath.Join(app, "romcure.log"), []byte("line 1 "+filepath.Join(home, "secret")+"\nline 2\n"), 0o644)
	os.WriteFile(filepath.Join(app, "settings.json"), []byte(`{"scratch_dir": "`+strings.ReplaceAll(filepath.Join(home, "scr"), `\`, `\\`)+`"}`), 0o644)
	os.WriteFile(filepath.Join(app, "library.db"), []byte("DATABASE"), 0o644)

	dest := filepath.Join(t.TempDir(), "bundle.zip")
	if err := ExportBundle(app, dest, map[string]string{"chdman": "0.289", "app": "dev"}); err != nil {
		t.Fatal(err)
	}
	files := readZip(t, dest)
	if _, ok := files["library.db"]; ok {
		t.Error("the database must never be bundled")
	}
	for _, n := range []string{"romcure.log", "settings.json", "system.txt"} {
		if _, ok := files[n]; !ok {
			t.Errorf("missing %s", n)
		}
	}
	if home != "" && (strings.Contains(files["romcure.log"], home) || strings.Contains(files["settings.json"], strings.ReplaceAll(home, `\`, `\\`))) {
		t.Error("home directory leaked into the bundle")
	}
	if !strings.Contains(files["system.txt"], "chdman: 0.289") || !strings.Contains(files["system.txt"], "os: ") {
		t.Errorf("system.txt = %q", files["system.txt"])
	}
	if _, err := os.Stat(dest + ".tmp"); err == nil {
		t.Error("temp file left behind")
	}
}

func TestExportBundleWithoutLogStillWorksAndHugeLogIsTruncated(t *testing.T) {
	app := t.TempDir()
	dest := filepath.Join(t.TempDir(), "b.zip")
	if err := ExportBundle(app, dest, nil); err != nil {
		t.Fatal(err)
	}
	if _, ok := readZip(t, dest)["romcure.log"]; ok {
		t.Error("no log file should mean no log entry")
	}

	big := strings.Repeat("0123456789abcdef\n", (maxLogBytes/17)+1000)
	os.WriteFile(filepath.Join(app, "romcure.log"), []byte(big), 0o644)
	if err := ExportBundle(app, dest, nil); err != nil {
		t.Fatal(err)
	}
	log := readZip(t, dest)["romcure.log"]
	if len(log) > maxLogBytes+200 || !strings.HasPrefix(log, "[log truncated") {
		t.Errorf("log len=%d prefix=%q", len(log), log[:30])
	}
}
