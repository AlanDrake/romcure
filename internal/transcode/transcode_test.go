package transcode

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"romcure/internal/link"
	"romcure/internal/store"
)

// TestMain doubles as a fake chdman: when FAKE_CHDMAN is set the test
// binary behaves like the tool, so no real chdman is needed.
func TestMain(m *testing.M) {
	if os.Getenv("FAKE_CHDMAN") == "1" {
		os.Exit(fakeChdman(os.Args[1:]))
	}
	if os.Getenv("FAKE_DOLPHIN") == "1" {
		os.Exit(fakeDolphin(os.Args[1:]))
	}
	os.Exit(m.Run())
}

func fakeChdman(args []string) int {
	switch args[0] {
	case "createcd", "createdvd":
		var in, out string
		for i, a := range args {
			if a == "-i" {
				in = args[i+1]
			}
			if a == "-o" {
				out = args[i+1]
			}
		}
		data, err := os.ReadFile(in)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		fmt.Print("Compressing, 50.0% complete... (ratio=40.0%)\rCompressing, 100.0% complete...\r")
		if os.WriteFile(out, append([]byte("CHD:"+args[0]+":"), data...), 0o644) != nil {
			return 1
		}
	case "verify":
		if os.Getenv("FAKE_CHDMAN_BAD_VERIFY") == "1" {
			fmt.Println("Error: data mismatch")
			return 1
		}
		fmt.Println("Verification successful")
	case "info":
		fmt.Println("Raw SHA1:     " + os.Getenv("FAKE_CHDMAN_SHA1"))
	}
	return 0
}

// converter runs Convert against a fresh cache and scratch folder.
type converter struct {
	tool    DiscTool
	cache   *Cache
	scratch string
}

func (c converter) run(j Job) (*CacheEntry, error) {
	return Convert(context.Background(), c.cache, c.tool, c.scratch, j, nil)
}

func newConverter(t *testing.T) converter {
	t.Helper()
	t.Setenv("FAKE_CHDMAN", "1")
	exe, _ := os.Executable()
	return newConverterWith(t, &Chdman{Path: exe, Version: "0.test"})
}

func newConverterWith(t *testing.T, tool DiscTool) converter {
	t.Helper()
	db, err := store.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	t.Cleanup(func() { link.ClearSourceRoots() })
	return converter{tool: tool, cache: &Cache{Dir: filepath.Join(t.TempDir(), "cache"), DB: db}, scratch: t.TempDir()}
}

func jobWith(t *testing.T, hash string) Job {
	in := filepath.Join(t.TempDir(), "disc.iso")
	os.WriteFile(in, []byte("disc"), 0o644)
	return Job{ID: hash, ContentHash: hash, Media: MediaDVD, Codecs: CodecsCompat[MediaDVD],
		Prepare: func(string) (string, error) { return in, nil }}
}

func TestModeByMedia(t *testing.T) {
	if DetectMedia(650<<20) != MediaCD || DetectMedia(4<<30) != MediaDVD {
		t.Fatal("media detection by size wrong")
	}
	if MediaCD.Mode() != "createcd" || MediaDVD.Mode() != "createdvd" {
		t.Fatal("wrong chdman mode")
	}
}

func TestParseProgress(t *testing.T) {
	p, ok := ParseProgress("Compressing, 45.2% complete... (ratio=38.1%)")
	if !ok || p != 45.2 {
		t.Fatalf("got %v %v", p, ok)
	}
	if _, ok := ParseProgress("Input file: x"); ok {
		t.Fatal("false positive")
	}
}

func TestCacheKeyIgnoresSourceButNotFlags(t *testing.T) {
	a := CacheKey("ABC", FormatCHD, "0.1", []string{"-c", "lzma"})
	if a != CacheKey("abc", FormatCHD, "0.1", []string{"-c", "lzma"}) {
		t.Fatal("hash case should not matter")
	}
	if a == CacheKey("abc", FormatCHD, "0.1", []string{"-c", "zstd"}) || a == CacheKey("abc", FormatCHD, "0.2", []string{"-c", "lzma"}) {
		t.Fatal("flags/tool version must change the key")
	}
}

func TestConvertVerifiesAndCaches(t *testing.T) {
	c := newConverter(t)
	t.Setenv("FAKE_CHDMAN_SHA1", "da39a3ee5e6b4b0d3255bfef95601890afd80709")
	j := jobWith(t, "hash1")
	j.ExpectedSHA1 = "DA39A3EE5E6B4B0D3255BFEF95601890AFD80709"
	var progress []float64
	entry, err := Convert(context.Background(), c.cache, c.tool, c.scratch, j, func(p float64) { progress = append(progress, p) })
	if err != nil {
		t.Fatal(err)
	}
	if !entry.Verified {
		t.Fatal("expected verified entry")
	}
	if len(progress) == 0 || progress[len(progress)-1] != 100 {
		t.Errorf("progress = %v, want it to reach 100", progress)
	}
	if b, _ := os.ReadFile(entry.Path); string(b) != "CHD:createdvd:disc" {
		t.Fatalf("output = %q", b)
	}
	if us, _ := c.cache.Usage(); us.Entries != 1 || us.Bytes == 0 {
		t.Fatalf("usage = %+v", us)
	}

	// Second run is a cache hit: Prepare must not be called.
	j.Prepare = func(string) (string, error) { t.Error("prepare called on cache hit"); return "", nil }
	if hit, err := c.run(j); err != nil || hit.Path != entry.Path {
		t.Fatalf("cache hit = %+v, %v", hit, err)
	}

	if err := c.cache.Clear(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(entry.Path); !os.IsNotExist(err) {
		t.Fatal("cache file not removed")
	}
}

func TestSHA1MismatchNotCached(t *testing.T) {
	c := newConverter(t)
	t.Setenv("FAKE_CHDMAN_SHA1", "da39a3ee5e6b4b0d3255bfef95601890afd80709")
	j := jobWith(t, "hash2")
	j.ExpectedSHA1 = "0000000000000000000000000000000000000000"
	if _, err := c.run(j); err == nil {
		t.Fatal("a SHA-1 mismatch must fail")
	}
	if us, _ := c.cache.Usage(); us.Entries != 0 {
		t.Fatal("unverified output must not be cached")
	}
	if m, _ := filepath.Glob(filepath.Join(c.cache.Dir, "*", "*")); len(m) != 0 {
		t.Fatalf("leftover files: %v", m)
	}
}

func TestVerifyFailureNotCached(t *testing.T) {
	c := newConverter(t)
	t.Setenv("FAKE_CHDMAN_BAD_VERIFY", "1")
	if _, err := c.run(jobWith(t, "hash3")); err == nil {
		t.Fatal("a failed verify must fail")
	}
	if us, _ := c.cache.Usage(); us.Entries != 0 {
		t.Fatal("failed verify must not be cached")
	}
}

func TestConvertStopsWhenCancelled(t *testing.T) {
	c := newConverter(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Convert(ctx, c.cache, c.tool, c.scratch, jobWith(t, "hash5"), nil); err == nil {
		t.Fatal("a cancelled conversion must fail")
	}
	if us, _ := c.cache.Usage(); us.Entries != 0 {
		t.Fatal("a cancelled conversion must not be cached")
	}
}

func TestRefusesWriteIntoSourceRoot(t *testing.T) {
	c := newConverter(t)
	if err := link.RegisterSourceRoot(c.cache.Dir); err != nil {
		t.Fatal(err)
	}
	if _, err := c.run(jobWith(t, "hash6")); err == nil {
		t.Fatal("expected guard error")
	}
}

func TestCheckScratchSpace(t *testing.T) {
	if err := CheckScratchSpace(t.TempDir(), 1<<20, 1); err != nil {
		t.Fatal(err)
	}
	if err := CheckScratchSpace(t.TempDir(), 1<<62, 1); err == nil {
		t.Fatal("expected insufficient space")
	}
}
