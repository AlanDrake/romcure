package app

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"romcure/internal/store"
)

func TestGroupsShowVariantsReasonsAndOverrides(t *testing.T) {
	a := newApp(t)
	src := filepath.Join(t.TempDir(), "nes")
	writeROMs(t, src)
	a.AddSource(src, "nes")
	out := filepath.Join(t.TempDir(), "out")
	a.AddProfile(store.Profile{Name: "raw", Kind: "raw", OutputRoot: out})
	if _, err := a.Sync(context.Background(), nil); err != nil {
		t.Fatal(err)
	}

	groups, err := a.Groups("nes")
	if err != nil {
		t.Fatal(err)
	}
	if len(groups) != 2 {
		t.Fatalf("groups = %+v", groups)
	}
	alpha := groups[0]
	if alpha.Name != "Alpha" || len(alpha.Variants) != 2 || !alpha.Variants[0].Selected || alpha.Variants[1].Selected {
		t.Fatalf("alpha = %+v", alpha)
	}
	if alpha.Variants[0].Region != "USA" || !strings.Contains(alpha.Variants[0].Title, "USA") {
		t.Errorf("selected variant = %+v", alpha.Variants[0])
	}
	if alpha.SelectionReason == "" || alpha.Overridden {
		t.Errorf("reason=%q overridden=%v", alpha.SelectionReason, alpha.Overridden)
	}

	// Force the European version, sync, and the output follows.
	eu := alpha.Variants[1].GameKey
	if err := a.PickVariant("nes", eu); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Sync(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
	groups, _ = a.Groups("nes")
	alpha = groups[0]
	if !alpha.Overridden || alpha.OverrideKey != eu || alpha.Variants[0].GameKey != eu || !strings.Contains(alpha.SelectionReason, "chosen by you") {
		t.Errorf("after override: %+v", alpha)
	}
	if b, _ := os.ReadFile(filepath.Join(out, "nes", "Alpha.nes")); string(b) != "alpha-eu" {
		t.Errorf("output not switched: %q", b)
	}

	// Clearing the override hands the decision back to the rules.
	if err := a.ClearPick("nes", eu); err != nil {
		t.Fatal(err)
	}
	a.Sync(context.Background(), nil)
	groups, _ = a.Groups("nes")
	if groups[0].Overridden || groups[0].Variants[0].Region != "USA" {
		t.Errorf("after clear: %+v", groups[0])
	}
	if b, _ := os.ReadFile(filepath.Join(out, "nes", "Alpha.nes")); string(b) != "alpha" {
		t.Errorf("output not switched back: %q", b)
	}

	if err := a.PickVariant("nes", "no-such-key"); err == nil {
		t.Error("unknown game must be an error")
	}
}
