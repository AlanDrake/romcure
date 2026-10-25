package profile

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"romcure/internal/transcode"
)

// fakeChdman stands in for chdman when the test binary is run with
// ROMCURE_FAKE_CHDMAN_LOG set: "verify" succeeds at once, "create..." logs
// its start and end times and arguments to the log file, takes 600 ms and
// writes a small file (or exits 1 when ROMCURE_FAKE_CHDMAN_FAIL is set).
func fakeChdman(logPath string) {
	args := os.Args[1:]
	if len(args) == 0 || args[0] == "verify" {
		os.Exit(0)
	}
	logf := func(format string, a ...any) {
		f, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
		if err == nil {
			fmt.Fprintf(f, format+"\n", a...)
			f.Close()
		}
	}
	logf("start %d %s", time.Now().UnixNano(), strings.Join(args, " "))
	fmt.Println("Compressing, 50.0% complete... (ratio=38.1%)")
	time.Sleep(600 * time.Millisecond)
	logf("end %d", time.Now().UnixNano())
	if os.Getenv("ROMCURE_FAKE_CHDMAN_FAIL") != "" {
		os.Exit(1)
	}
	for i, a := range args {
		if a == "-o" && i+1 < len(args) {
			os.WriteFile(args[i+1], []byte("fake chd"), 0o644)
		}
	}
	os.Exit(0)
}

// maxOverlap returns the largest number of conversions running at once.
func maxOverlap(t *testing.T, logPath string) (starts int, overlap int) {
	data, _ := os.ReadFile(logPath)
	type ev struct {
		at    int64
		delta int
	}
	var evs []ev
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		f := strings.Fields(line)
		if len(f) < 2 {
			continue
		}
		n, _ := strconv.ParseInt(f[1], 10, 64)
		if f[0] == "start" {
			evs = append(evs, ev{n, 1})
			starts++
		} else {
			evs = append(evs, ev{n, -1})
		}
	}
	for i := range evs { // insertion sort: a handful of events
		for j := i; j > 0 && evs[j].at < evs[j-1].at; j-- {
			evs[j], evs[j-1] = evs[j-1], evs[j]
		}
	}
	cur := 0
	for _, e := range evs {
		cur += e.delta
		if cur > overlap {
			overlap = cur
		}
	}
	return
}

func runThreeDiscs(t *testing.T, parallelism int, profile func(transcode.Media) transcode.CodecProfile) (Report, string) {
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	logPath := filepath.Join(t.TempDir(), "chdman.log")
	t.Setenv("ROMCURE_FAKE_CHDMAN_LOG", logPath)

	root := t.TempDir()
	for i, name := range []string{"Disc A", "Disc B", "Disc C"} {
		bin := make([]byte, 2352*50)
		for j := range bin {
			bin[j] = byte(j*(i+3) + i)
		}
		writeZip(t, filepath.Join(root, name+" (USA).zip"), map[string]string{
			name + " (USA).cue": fmt.Sprintf("FILE \"%s (USA).bin\" BINARY\n  TRACK 01 MODE1/2352\n    INDEX 01 00:00:00\n", name),
			name + " (USA).bin": string(bin),
		})
	}
	db := openTestDB(t)
	syncSystem(t, db, root, "psx")
	report, err := GenerateRetroArch(db, "psx", Options{
		OutputRoot: t.TempDir(), Parallelism: parallelism,
		Transcode: &TranscodeOptions{
			Cache:      &transcode.Cache{Dir: filepath.Join(t.TempDir(), "cache"), DB: db},
			Tool:       &transcode.Chdman{Path: self, Version: "fake"},
			ScratchDir: t.TempDir(),
			Profile:    profile,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	return report, logPath
}

func TestParallelismRunsConversionsAtTheSameTime(t *testing.T) {
	report, logPath := runThreeDiscs(t, 3, nil)
	if len(report.Linked) != 3 || len(report.Errors) != 0 {
		t.Fatalf("linked %d, errors %v", len(report.Linked), report.Errors)
	}
	if starts, overlap := maxOverlap(t, logPath); starts != 3 || overlap < 2 {
		t.Errorf("3 discs with parallelism 3: %d conversions started, at most %d at once", starts, overlap)
	}
}

func TestOneAtATimeByDefault(t *testing.T) {
	report, logPath := runThreeDiscs(t, 1, nil)
	if len(report.Linked) != 3 {
		t.Fatalf("linked %d, errors %v", len(report.Linked), report.Errors)
	}
	if starts, overlap := maxOverlap(t, logPath); starts != 3 || overlap != 1 {
		t.Errorf("parallelism 1: %d started, %d at once", starts, overlap)
	}
}

func TestCodecSettingReachesChdman(t *testing.T) {
	_, logPath := runThreeDiscs(t, 1, func(m transcode.Media) transcode.CodecProfile {
		return transcode.CodecProfile{Name: "custom", Flags: []string{"-c", "cdzs"}}
	})
	data, _ := os.ReadFile(logPath)
	if !strings.Contains(string(data), "-c cdzs") || strings.Contains(string(data), "cdlz") {
		t.Errorf("chdman was not given the user's codecs:\n%s", data)
	}
}

func TestAFailedConversionIsNotRetriedAfterThePrewarmPass(t *testing.T) {
	t.Setenv("ROMCURE_FAKE_CHDMAN_FAIL", "1")
	report, logPath := runThreeDiscs(t, 3, nil)
	if len(report.Errors) != 3 {
		t.Errorf("errors = %v, want 3", report.Errors)
	}
	if starts, _ := maxOverlap(t, logPath); starts != 3 {
		t.Errorf("%d conversions started for 3 discs: failures must not be retried", starts)
	}
}

func TestConversionPercentReachesTheCallback(t *testing.T) {
	var got []string
	var mu sync.Mutex
	self, _ := os.Executable()
	t.Setenv("ROMCURE_FAKE_CHDMAN_LOG", filepath.Join(t.TempDir(), "log"))
	root := t.TempDir()
	writeZip(t, filepath.Join(root, "Disc A (USA).zip"), map[string]string{
		"Disc A (USA).cue": "FILE \"Disc A (USA).bin\" BINARY\n  TRACK 01 MODE1/2352\n    INDEX 01 00:00:00\n",
		"Disc A (USA).bin": string(make([]byte, 2352*50)),
	})
	db := openTestDB(t)
	syncSystem(t, db, root, "psx")
	_, err := GenerateRetroArch(db, "psx", Options{
		OutputRoot: t.TempDir(),
		OnProgress: func(item string, pct float64) {
			mu.Lock()
			got = append(got, fmt.Sprintf("%s %.0f", item, pct))
			mu.Unlock()
		},
		Transcode: &TranscodeOptions{
			Cache:      &transcode.Cache{Dir: filepath.Join(t.TempDir(), "cache"), DB: db},
			Tool:       &transcode.Chdman{Path: self, Version: "fake"},
			ScratchDir: t.TempDir(),
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, g := range got {
		found = found || g == "Disc A 50"
	}
	if !found {
		t.Errorf("progress events = %v, want one for \"Disc A\" at 50", got)
	}
}
