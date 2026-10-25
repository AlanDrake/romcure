package transcode

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"romcure/internal/proc"
	"strings"
	"time"
)

// DolphinTool wraps the dolphin-tool binary (GameCube/Wii → RVZ).
type DolphinTool struct {
	Path    string
	Version string
	// UserDir is dolphin-tool's temp/user folder (-u); empty lets it make
	// its own. Should be on a volume with room for scratch files.
	UserDir string
	// Timeout bounds each invocation (default 2h). dolphin-tool reports some
	// failures as a GUI message box that waits for a click, which would hang
	// a headless run forever; the timeout kills it.
	Timeout time.Duration
}

// RVZProfile is the RVZ codec profile Dolphin itself suggests: zstd level
// 5 with 128 KiB blocks. It is part of the cache key, like chdman flags.
var RVZProfile = CodecProfile{Name: "rvz-zstd5", Flags: []string{"-f", "rvz", "-c", "zstd", "-l", "5", "-b", "131072"}}

func (d *DolphinTool) Name() string         { return "dolphin-tool" }
func (d *DolphinTool) ToolVersion() string  { return d.Version }
func (d *DolphinTool) OutputFormat() Format { return FormatRVZ }

func (d *DolphinTool) run(ctx context.Context, args ...string) (string, error) {
	timeout := d.Timeout
	if timeout == 0 {
		timeout = 2 * time.Hour
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	if d.UserDir != "" {
		if err := os.MkdirAll(d.UserDir, 0o755); err != nil {
			return "", err
		}
		args = append(args, "-u", d.UserDir)
	}
	out, err := proc.Quiet(exec.CommandContext(ctx, d.Path, args...)).CombinedOutput()
	text := strings.TrimSpace(string(out))
	if ctx.Err() == context.DeadlineExceeded {
		return text, fmt.Errorf("dolphin-tool %s timed out after %s (it may be waiting on an error dialog)", args[0], timeout)
	}
	if err != nil {
		return text, fmt.Errorf("dolphin-tool %s: %w\n%s", args[0], err, text)
	}
	return text, nil
}

// needsISOFirst reports formats dolphin-tool refuses to convert straight to
// RVZ because they don't record the original disc size accurately (its
// "DataSizeType::Accurate" assertion): WBFS and CISO. They must be expanded
// to an ISO first, which needs a full disc image of scratch space.
func needsISOFirst(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".wbfs", ".ciso":
		return true
	}
	return false
}

// NeedsISOFirst is the exported form for callers sizing scratch space.
func NeedsISOFirst(path string) bool { return needsISOFirst(path) }

// ToISO expands any dolphin-readable image to a plain ISO at output.
func (d *DolphinTool) ToISO(ctx context.Context, input, output string) error {
	_, err := d.run(ctx, "convert", "-i", input, "-o", output, "-f", "iso")
	return err
}

// Convert implements DiscTool. WBFS/CISO inputs are expanded to an ISO next
// to output first (and removed afterwards).
func (d *DolphinTool) Convert(ctx context.Context, j Job, input, output string, onProgress func(float64)) error {
	if needsISOFirst(input) {
		iso := output + ".iso.tmp"
		defer os.Remove(iso)
		if err := d.ToISO(ctx, input, iso); err != nil {
			return err
		}
		input = iso
	}
	args := append([]string{"convert", "-i", input, "-o", output}, j.Codecs.Flags...)
	_, err := d.run(ctx, args...)
	return err
}

// Verify runs dolphin-tool's own integrity check (a full read of the image
// computing its hashes) and treats a nonzero exit as failure.
func (d *DolphinTool) Verify(ctx context.Context, output string) error {
	_, err := d.run(ctx, "verify", "-i", output, "-a", "sha1")
	return err
}

var hexSHA1 = regexp.MustCompile(`(?m)^\s*([0-9a-fA-F]{40})\s*$`)

// RawSHA1 returns the SHA1 of the disc contents (identical for the RVZ and
// the original ISO, which is how the conversion is proven lossless).
func (d *DolphinTool) RawSHA1(ctx context.Context, output string) (string, error) {
	text, err := d.run(ctx, "verify", "-i", output, "-a", "sha1")
	if err != nil {
		return "", err
	}
	m := hexSHA1.FindStringSubmatch(text)
	if m == nil {
		return "", fmt.Errorf("no sha1 in dolphin-tool output for %s", output)
	}
	return strings.ToLower(m[1]), nil
}
