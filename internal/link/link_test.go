package link

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"romcure/internal/fsinfo"
)

func writeTempFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
	return path
}

func TestSymlinkRelative(t *testing.T) {
	dir := t.TempDir()
	src := writeTempFile(t, dir, "src.rom", "rom-data")
	dst := filepath.Join(dir, "sub", "dst.rom")

	if err := SymlinkRelative(src, dst); err != nil {
		t.Skipf("symlink creation not permitted in this environment (enable Developer Mode or run elevated): %v", err)
	}

	target, err := os.Readlink(dst)
	if err != nil {
		t.Fatalf("Readlink: %v", err)
	}
	if filepath.IsAbs(target) {
		t.Errorf("link target %q is absolute, want relative", target)
	}

	got, err := os.ReadFile(dst)
	if err != nil {
		t.Fatalf("read through link: %v", err)
	}
	if string(got) != "rom-data" {
		t.Errorf("read through link = %q, want %q", got, "rom-data")
	}
}

// A destination folder reached through a symlink (macOS keeps /var -> /private/var,
// users link folders) must still get a link that works: ".." is followed on the
// real tree, not on the path as it was written.
func TestSymlinkRelativeFromAFolderReachedThroughASymlink(t *testing.T) {
	dir := t.TempDir()
	src := writeTempFile(t, dir, "src.rom", "rom-data")
	deep := filepath.Join(dir, "real", "deep")
	if err := os.MkdirAll(deep, 0o755); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(dir, "alias") // alias -> real/deep, one level shorter than its target
	if err := os.Symlink(deep, alias); err != nil {
		t.Skipf("symlink creation not permitted in this environment: %v", err)
	}
	dst := filepath.Join(alias, "out", "dst.rom")
	if err := SymlinkRelative(src, dst); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(dst)
	if err != nil || string(got) != "rom-data" {
		t.Errorf("read through link = %q, %v; want the source's content", got, err)
	}
}

func TestSymlinkAbsolute(t *testing.T) {
	dir := t.TempDir()
	src := writeTempFile(t, dir, "src.rom", "rom-data")
	dst := filepath.Join(dir, "sub", "dst.rom")

	if err := SymlinkAbsolute(src, dst); err != nil {
		t.Skipf("symlink creation not permitted in this environment (enable Developer Mode or run elevated): %v", err)
	}

	target, err := os.Readlink(dst)
	if err != nil {
		t.Fatalf("Readlink: %v", err)
	}
	if !filepath.IsAbs(target) {
		t.Errorf("link target %q is relative, want absolute", target)
	}
}

func TestHardlink(t *testing.T) {
	dir := t.TempDir()
	src := writeTempFile(t, dir, "src.rom", "rom-data")
	dst := filepath.Join(dir, "sub", "dst.rom")

	if err := Hardlink(src, dst); err != nil {
		t.Fatalf("Hardlink: %v", err)
	}

	// A hardlink *is* the same file: a write through dst must be visible
	// through src (spec §2.2's caveat is exactly this).
	if err := os.WriteFile(dst, []byte("changed"), 0o644); err != nil {
		t.Fatalf("write through hardlink: %v", err)
	}
	got, err := os.ReadFile(src)
	if err != nil {
		t.Fatalf("read src: %v", err)
	}
	if string(got) != "changed" {
		t.Errorf("read src after writing dst = %q, want %q (hardlink shares data)", got, "changed")
	}
}

func TestCopy(t *testing.T) {
	dir := t.TempDir()
	src := writeTempFile(t, dir, "src.rom", "rom-data")
	dst := filepath.Join(dir, "sub", "dst.rom")

	if err := Copy(src, dst); err != nil {
		t.Fatalf("Copy: %v", err)
	}

	// A copy is independent: a write through dst must NOT reach src.
	if err := os.WriteFile(dst, []byte("changed"), 0o644); err != nil {
		t.Fatalf("write to copy: %v", err)
	}
	got, err := os.ReadFile(src)
	if err != nil {
		t.Fatalf("read src: %v", err)
	}
	if string(got) != "rom-data" {
		t.Errorf("read src after writing copy = %q, want unchanged %q", got, "rom-data")
	}
}

func TestReflinkWhereSupported(t *testing.T) {
	dir := t.TempDir()
	src := writeTempFile(t, dir, "src.rom", "rom-data")
	dst := filepath.Join(dir, "sub")
	if err := os.MkdirAll(dst, 0o755); err != nil {
		t.Fatal(err)
	}
	dstFile := filepath.Join(dst, "dst.rom")

	info, err := fsinfo.Detect(dir)
	if err != nil {
		t.Fatalf("fsinfo.Detect: %v", err)
	}
	if !info.Type.CoWCapable() {
		t.Skipf("temp dir filesystem %s is not CoW-capable; skipping reflink test", info.Raw)
	}

	if err := Reflink(src, dstFile); err != nil {
		t.Fatalf("Reflink: %v", err)
	}

	// A reflink is independent after creation, same as a copy.
	if err := os.WriteFile(dstFile, []byte("changed"), 0o644); err != nil {
		t.Fatalf("write to reflink: %v", err)
	}
	got, err := os.ReadFile(src)
	if err != nil {
		t.Fatalf("read src: %v", err)
	}
	if string(got) != "rom-data" {
		t.Errorf("read src after writing reflink = %q, want unchanged %q", got, "rom-data")
	}
}

func TestWritePrimitivesRefuseSourceRoot(t *testing.T) {
	root := t.TempDir()
	src := writeTempFile(t, root, "src.rom", "rom-data")

	if err := RegisterSourceRoot(root); err != nil {
		t.Fatalf("RegisterSourceRoot: %v", err)
	}
	t.Cleanup(ClearSourceRoots)

	insideRoot := filepath.Join(root, "dst.rom")

	tests := []struct {
		name string
		fn   func() error
	}{
		{"SymlinkRelative", func() error { return SymlinkRelative(src, insideRoot) }},
		{"SymlinkAbsolute", func() error { return SymlinkAbsolute(src, insideRoot) }},
		{"Hardlink", func() error { return Hardlink(src, insideRoot) }},
		{"Reflink", func() error { return Reflink(src, insideRoot) }},
		{"Copy", func() error { return Copy(src, insideRoot) }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.fn()
			if err == nil {
				t.Fatalf("%s into a registered source root: got nil error, want ErrWriteIntoSourceRoot", tt.name)
			}
			if !errors.Is(err, ErrWriteIntoSourceRoot) {
				t.Errorf("%s error = %v, want it to wrap ErrWriteIntoSourceRoot", tt.name, err)
			}
			if _, statErr := os.Lstat(insideRoot); statErr == nil {
				t.Errorf("%s: a file was created at %s despite the guard", tt.name, insideRoot)
			}
		})
	}
}

func TestCapabilitiesStructure(t *testing.T) {
	dir := t.TempDir()
	src := writeTempFile(t, dir, "src.rom", "rom-data")
	destDir := filepath.Join(dir, "dest")
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		t.Fatal(err)
	}
	dst := filepath.Join(destDir, "dst.rom")

	caps, err := Capabilities(src, dst)
	if err != nil {
		t.Fatalf("Capabilities: %v", err)
	}

	wantSameVolume, err := fsinfo.SameVolume(src, destDir)
	if err != nil {
		t.Fatalf("fsinfo.SameVolume: %v", err)
	}
	if caps.SameVolume != wantSameVolume {
		t.Errorf("SameVolume = %v, want %v", caps.SameVolume, wantSameVolume)
	}

	if len(caps.Tiers) != 4 {
		t.Fatalf("len(Tiers) = %d, want 4", len(caps.Tiers))
	}
	wantOrder := []Tier{TierSymlink, TierReflink, TierHardlink, TierCopy}
	for i, want := range wantOrder {
		got := caps.Tiers[i]
		if got.Tier != want {
			t.Errorf("Tiers[%d].Tier = %v, want %v", i, got.Tier, want)
		}
		if !got.Available && got.Reason == "" {
			t.Errorf("Tiers[%d] (%v) is unavailable but has no reason", i, got.Tier)
		}
		if got.Tier == TierCopy && !got.Available {
			t.Errorf("TierCopy must always be available")
		}
	}

	// The probe must not leave anything behind in the destination directory.
	entries, err := os.ReadDir(destDir)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("destDir has %d leftover entries after Capabilities, want 0: %v", len(entries), entries)
	}
}
