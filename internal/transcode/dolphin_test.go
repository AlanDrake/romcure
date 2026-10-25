package transcode

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// fakeDolphin mimics dolphin-tool for tests: it behaves like the real tool
// in the two ways that matter here (refuses WBFS→RVZ with the same
// assertion-style failure; `verify -a sha1` prints only the digest).
func fakeDolphin(args []string) int {
	opt := func(name string) string {
		for i, a := range args {
			if a == name && i+1 < len(args) {
				return args[i+1]
			}
		}
		return ""
	}
	switch args[0] {
	case "convert":
		in, out, format := opt("-i"), opt("-o"), opt("-f")
		if strings.HasSuffix(in, ".wbfs") && format == "rvz" {
			fmt.Fprintln(os.Stderr, "Condition: infile->GetDataSizeType() == DataSizeType::Accurate")
			return 1
		}
		if os.Getenv("FAKE_DOLPHIN_HANG") == "1" {
			time.Sleep(time.Minute)
		}
		data, err := os.ReadFile(in)
		if err != nil {
			fmt.Fprintln(os.Stderr, "Error: Unable to open input file")
			return 1
		}
		os.WriteFile(out, append([]byte("FMT:"+format+":"), data...), 0o644)
	case "verify":
		data, err := os.ReadFile(opt("-i"))
		if err != nil {
			return 1
		}
		// The "disc contents" are everything after the second colon.
		body := string(data)
		if strings.HasPrefix(body, "FMT:") {
			body = strings.SplitN(body, ":", 3)[2]
		}
		sum := sha1.Sum([]byte(body))
		fmt.Println(hex.EncodeToString(sum[:]))
	}
	return 0
}

func dolphinFor(t *testing.T) *DolphinTool {
	t.Helper()
	t.Setenv("FAKE_DOLPHIN", "1")
	exe, _ := os.Executable()
	return &DolphinTool{Path: exe, Version: "test", UserDir: t.TempDir()}
}

func TestDolphinConvertVerifyAndSHA1(t *testing.T) {
	d := dolphinFor(t)
	dir := t.TempDir()
	in := filepath.Join(dir, "g.iso")
	os.WriteFile(in, []byte("disc-contents"), 0o644)
	out := filepath.Join(dir, "g.rvz")
	if err := d.Convert(context.Background(), Job{Codecs: RVZProfile}, in, out, nil); err != nil {
		t.Fatal(err)
	}
	if err := d.Verify(context.Background(), out); err != nil {
		t.Fatal(err)
	}
	sum := sha1.Sum([]byte("disc-contents"))
	got, err := d.RawSHA1(context.Background(), out)
	if err != nil || got != hex.EncodeToString(sum[:]) {
		t.Errorf("sha1 = %q err=%v", got, err)
	}
}

func TestDolphinWBFSGoesThroughISO(t *testing.T) {
	d := dolphinFor(t)
	dir := t.TempDir()
	in := filepath.Join(dir, "g.wbfs")
	os.WriteFile(in, []byte("wbfs-disc"), 0o644)
	out := filepath.Join(dir, "g.rvz")
	if err := d.Convert(context.Background(), Job{Codecs: RVZProfile}, in, out, nil); err != nil {
		t.Fatalf("wbfs must be expanded to an ISO first: %v", err)
	}
	if !NeedsISOFirst(in) || NeedsISOFirst("x.iso") {
		t.Error("NeedsISOFirst wrong")
	}
	if left, _ := filepath.Glob(out + "*.tmp"); len(left) != 0 {
		t.Errorf("temp ISO left behind: %v", left)
	}
}

func TestDolphinTimeoutKillsAHungTool(t *testing.T) {
	d := dolphinFor(t)
	d.Timeout = 300 * time.Millisecond
	t.Setenv("FAKE_DOLPHIN_HANG", "1")
	dir := t.TempDir()
	in := filepath.Join(dir, "g.iso")
	os.WriteFile(in, []byte("x"), 0o644)
	err := d.Convert(context.Background(), Job{Codecs: RVZProfile}, in, filepath.Join(dir, "o.rvz"), nil)
	if err == nil || !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("err = %v, want timeout", err)
	}
}

func TestConvertWithDolphinIntoRVZCache(t *testing.T) {
	d := dolphinFor(t)
	c := newConverterWith(t, d)
	in := filepath.Join(t.TempDir(), "g.iso")
	os.WriteFile(in, []byte("wii-disc"), 0o644)
	sum := sha1.Sum([]byte("wii-disc"))
	entry, err := c.run(Job{ID: "w", ContentHash: "wiihash", Codecs: RVZProfile, ExpectedSHA1: hex.EncodeToString(sum[:]),
		Prepare: func(string) (string, error) { return in, nil }})
	if err != nil {
		t.Fatal(err)
	}
	if entry.Format != FormatRVZ || entry.ToolName != "dolphin-tool" || !strings.HasSuffix(entry.Path, ".rvz") {
		t.Errorf("entry = %+v", entry)
	}
	if us, _ := c.cache.Usage(); us.Entries != 1 {
		t.Errorf("usage = %+v", us)
	}
}
