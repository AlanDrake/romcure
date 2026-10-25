package discover

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func writeCfg(t *testing.T, dir, body string) string {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(dir, "retroarch.cfg")
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestParseRetroArchConfigResolvesDirectories(t *testing.T) {
	root := t.TempDir()
	for name, c := range map[string]struct{ body, playlists, cores string }{
		"portable": {"playlist_directory = \":\\playlists\"\nlibretro_directory = \":\\cores\"\n",
			filepath.Join(root, "portable", "playlists"), filepath.Join(root, "portable", "cores")},
		"default": {"playlist_directory = \"default\"\nlibretro_directory = \"\"\n",
			filepath.Join(root, "default", "playlists"), filepath.Join(root, "default", "cores")},
		"missing": {"video_fullscreen = \"false\"\n",
			filepath.Join(root, "missing", "playlists"), filepath.Join(root, "missing", "cores")},
		"custom": {"playlist_directory = \"" + filepath.ToSlash(filepath.Join(root, "elsewhere")) + "\"\n",
			filepath.Join(root, "elsewhere"), filepath.Join(root, "custom", "cores")},
	} {
		in, err := ParseRetroArchConfig(writeCfg(t, filepath.Join(root, name), c.body))
		if err != nil {
			t.Fatal(err)
		}
		if in.Playlists != c.playlists || in.Cores != c.cores {
			t.Errorf("%s: playlists %q cores %q, want %q %q", name, in.Playlists, in.Cores, c.playlists, c.cores)
		}
	}
}

func TestRetroArchInstallsNeedAConfigAndComeMostRecentFirst(t *testing.T) {
	root := t.TempDir()
	old := writeCfg(t, filepath.Join(root, "old"), "")
	recent := writeCfg(t, filepath.Join(root, "recent"), "")
	if err := os.Chtimes(old, time.Now(), time.Now().Add(-48*time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "empty"), 0o755); err != nil {
		t.Fatal(err)
	}
	got := retroArchInstallsIn([]string{
		filepath.Join(root, "old"), filepath.Join(root, "empty"), filepath.Join(root, "recent"),
		filepath.Join(root, "recent"), "", filepath.Join(root, "nowhere"),
	})
	if len(got) != 2 || got[0].Config != recent || got[1].Config != old {
		t.Fatalf("got %+v, want [recent old]", got)
	}
}
