package profile

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"romcure/internal/transcode"
)

// TestMain lets the test binary stand in for chdman: a conversion that takes
// a minute, so a test can cancel it midway.
func TestMain(m *testing.M) {
	if os.Getenv("ROMCURE_FAKE_SLOW_CHDMAN") != "" {
		time.Sleep(time.Minute)
		os.Exit(0)
	}
	if log := os.Getenv("ROMCURE_FAKE_CHDMAN_LOG"); log != "" {
		fakeChdman(log)
	}
	os.Exit(m.Run())
}

func TestCancelStopsARunningConversion(t *testing.T) {
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("ROMCURE_FAKE_SLOW_CHDMAN", "1")

	root := t.TempDir()
	writeZip(t, filepath.Join(root, "Disc Game (USA).zip"), map[string]string{
		"Disc Game (USA).cue": "FILE \"Disc Game (USA).bin\" BINARY\n  TRACK 01 MODE1/2352\n    INDEX 01 00:00:00\n",
		"Disc Game (USA).bin": string(make([]byte, 2352*100)),
	})
	db := openTestDB(t)
	syncSystem(t, db, root, "psx")

	scratch := t.TempDir()
	ctx, cancel := context.WithCancel(context.Background())
	time.AfterFunc(1500*time.Millisecond, cancel)
	start := time.Now()
	_, err = GenerateRetroArch(db, "psx", Options{
		OutputRoot: t.TempDir(), Context: ctx,
		Transcode: &TranscodeOptions{
			Cache:      &transcode.Cache{Dir: filepath.Join(t.TempDir(), "cache"), DB: db},
			Tool:       &transcode.Chdman{Path: self, Version: "fake"},
			ScratchDir: scratch,
		},
	})
	if took := time.Since(start); took > 20*time.Second {
		t.Fatalf("cancel did not stop the conversion: returned after %v", took)
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("err = %v, want context.Canceled", err)
	}
	if left, _ := os.ReadDir(scratch); len(left) != 0 {
		t.Errorf("scratch not cleaned after cancel: %v", left)
	}
}

func TestCancelBeforeExtractionStopsAtOnce(t *testing.T) {
	root := t.TempDir()
	writeZip(t, filepath.Join(root, "Disc Game (USA).zip"), map[string]string{
		"Disc Game (USA).cue": "FILE \"Disc Game (USA).bin\" BINARY\n  TRACK 01 MODE1/2352\n    INDEX 01 00:00:00\n",
		"Disc Game (USA).bin": string(make([]byte, 2352*100)),
	})
	db := openTestDB(t)
	syncSystem(t, db, root, "psx")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := GenerateRetroArch(db, "psx", Options{OutputRoot: t.TempDir(), Context: ctx})
	if !errors.Is(err, context.Canceled) {
		t.Errorf("err = %v, want context.Canceled", err)
	}
}
