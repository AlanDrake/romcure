package transcode

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

// realChdman locates the dev chdman under tools/bin (see tools/README.md);
// tests using it skip when it isn't present.
func realChdman(t *testing.T) *Chdman {
	t.Helper()
	p, err := filepath.Abs(filepath.Join("..", "..", "tools", "bin", runtime.GOOS+"-"+runtime.GOARCH, "chdman"))
	if runtime.GOOS == "windows" {
		p += ".exe"
	}
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(p); err != nil {
		t.Skip("real chdman not present: " + p)
	}
	return &Chdman{Path: p, Version: "real"}
}

func TestRealChdmanCDRoundTrip(t *testing.T) {
	c := realChdman(t)
	dir := t.TempDir() // point TEMP/TMP at a fast scratch disk to keep these writes off the system drive
	// One MODE1/2352 track of 20000 sectors of deterministic data.
	bin := make([]byte, 2352*20000)
	for i := range bin {
		bin[i] = byte(i * 7)
	}
	os.WriteFile(filepath.Join(dir, "t.bin"), bin, 0o644)
	cue := filepath.Join(dir, "t.cue")
	os.WriteFile(cue, []byte("FILE \"t.bin\" BINARY\n  TRACK 01 MODE1/2352\n    INDEX 01 00:00:00\n"), 0o644)
	out := filepath.Join(dir, "t.chd")

	var last float64
	start := time.Now()
	if err := c.Create(context.Background(), MediaCD, cue, out, CodecsCompat[MediaCD], func(p float64) { last = p }); err != nil {
		t.Fatal(err)
	}
	// chdman prints its first progress line about half a second in; on a fast
	// disk this small image can finish before that, so only a run that lasted
	// well past it must have reported progress.
	if took := time.Since(start); last <= 0 && took > 2*time.Second {
		t.Fatalf("progress callback saw max %v after %v, want > 0", last, took)
	}
	if err := c.Verify(context.Background(), out); err != nil {
		t.Fatal(err)
	}
	sha, err := c.RawSHA1(context.Background(), out)
	if err != nil {
		t.Fatal(err)
	}
	if len(sha) != 40 {
		t.Fatalf("bad sha1 %q", sha)
	}
}
