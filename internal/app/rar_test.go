package app

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"romcure/internal/rararc/rartest"
	"romcure/internal/store"
)

func TestSyncReadsGamesInsideARARAndExtractsThemIntoTheOutput(t *testing.T) {
	a, src := selectionApp(t)
	a.Tools.SevenZip = rartest.SevenZip(t)
	_, out := addOutput(t, a, store.Profile{Name: "Plain"})
	rartest.Write(t, filepath.Join(src, "Delta (USA).rar"), []string{"Delta (USA).nes"}, map[string][]byte{"Delta (USA).nes": []byte("delta")})
	// A later volume is read through the first one; on its own it is not an archive.
	os.WriteFile(filepath.Join(src, "Big Game.part2.rar"), []byte("second volume"), 0o644)

	report, err := a.Sync(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Unreadable) != 0 {
		t.Errorf("unreadable = %+v", report.Unreadable)
	}
	if got := outputs(t, out); !equal(got, []string{"Alpha.nes", "Beta.nes", "Delta.nes", "Gamma.nes"}) {
		t.Fatalf("output = %v", got)
	}
	for name, want := range map[string]string{"Delta.nes": "delta", "Alpha.nes": "alpha"} {
		b, err := os.ReadFile(filepath.Join(out, "nes", name))
		if err != nil || string(b) != want {
			li, _ := os.Lstat(filepath.Join(out, "nes", name))
			dest, _ := os.Readlink(filepath.Join(out, "nes", name))
			t.Errorf("%s = %q, err %v (lstat %v, link -> %q), want %q", name, b, err, li, dest, want)
		}
	}
}

func TestARARWithoutSevenZipIsReportedAndTheRestStillSyncs(t *testing.T) {
	a, src := selectionApp(t)
	a.Tools.SevenZip = ""
	_, out := addOutput(t, a, store.Profile{Name: "Plain"})
	rartest.Write(t, filepath.Join(src, "Delta (USA).rar"), []string{"Delta (USA).nes"}, map[string][]byte{"Delta (USA).nes": []byte("delta")})

	report, err := a.Sync(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Unreadable) != 1 || filepath.Base(report.Unreadable[0].Path) != "Delta (USA).rar" {
		t.Fatalf("unreadable = %+v", report.Unreadable)
	}
	if got := outputs(t, out); !equal(got, []string{"Alpha.nes", "Beta.nes", "Gamma.nes"}) {
		t.Errorf("output = %v", got)
	}
}
