package transcode

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"os/exec"
	"regexp"
	"romcure/internal/proc"
	"strconv"
	"strings"
)

// Media is the kind of disc being converted. chdman's mode depends on it,
// not on the system alone (spec §2.10): PS2 has CD-based titles, PSP is UMD.
type Media int

const (
	MediaCD  Media = iota // PS1, Saturn, Sega CD, PCE-CD, Dreamcast GDI, CD-based PS2
	MediaDVD              // DVD PS2 titles, PSP/UMD
)

// Mode returns the chdman subcommand for the media.
func (m Media) Mode() string {
	if m == MediaDVD {
		return "createdvd"
	}
	return "createcd"
}

// DetectMedia chooses by the size of the uncompressed image: anything over
// a CD's capacity (~700 MiB, with headroom for 80-minute discs) is DVD.
func DetectMedia(imageBytes int64) Media {
	const cdMax = 900 << 20
	if imageBytes > cdMax {
		return MediaDVD
	}
	return MediaCD
}

// CodecProfile pins the codecs passed to chdman. Newer chdman defaults
// (e.g. zstd) aren't decodable by older cores, so flags are always
// explicit and part of the cache key.
type CodecProfile struct {
	Name  string
	Flags []string
}

var (
	// CodecsCompat is limited to codecs every libretro CHD-capable core
	// reads: lzma, zlib, huff, flac (CD uses cdlz,cdzl,cdfl).
	CodecsCompat = map[Media]CodecProfile{
		MediaCD:  {Name: "compat", Flags: []string{"-c", "cdlz,cdzl,cdfl"}},
		MediaDVD: {Name: "compat", Flags: []string{"-c", "lzma,zlib,huff,flac"}},
	}
)

// Chdman wraps a bundled chdman binary. Run only as a subprocess (HANDOFF §3).
type Chdman struct {
	Path    string
	Version string
}

// CreateArgs builds the argument list for converting input to output.
func (c *Chdman) CreateArgs(media Media, input, output string, codecs CodecProfile) []string {
	args := []string{media.Mode(), "-i", input, "-o", output}
	args = append(args, codecs.Flags...)
	return append(args, "--force")
}

var progressPattern = regexp.MustCompile(`(\d+(?:\.\d+)?)%\s+complete`)

// ParseProgress extracts a 0..100 percentage from one chdman status line
// such as "Compressing, 45.2% complete... (ratio=38.1%)".
func ParseProgress(line string) (float64, bool) {
	m := progressPattern.FindStringSubmatch(line)
	if m == nil {
		return 0, false
	}
	f, err := strconv.ParseFloat(m[1], 64)
	return f, err == nil
}

// splitCRorLF splits on \r as well as \n: chdman redraws its progress line
// with carriage returns.
func splitCRorLF(data []byte, atEOF bool) (int, []byte, error) {
	if atEOF && len(data) == 0 {
		return 0, nil, nil
	}
	if i := bytes.IndexAny(data, "\r\n"); i >= 0 {
		return i + 1, data[:i], nil
	}
	if atEOF {
		return len(data), data, nil
	}
	return 0, nil, nil
}

// run executes chdman, feeding progress percentages to onProgress.
// Cancelling ctx kills the process.
func (c *Chdman) run(ctx context.Context, args []string, onProgress func(float64)) (string, error) {
	cmd := proc.Quiet(exec.CommandContext(ctx, c.Path, args...))
	pr, pw := io.Pipe()
	cmd.Stdout, cmd.Stderr = pw, pw
	var out strings.Builder
	done := make(chan struct{})
	go func() {
		defer close(done)
		sc := bufio.NewScanner(pr)
		sc.Split(splitCRorLF)
		for sc.Scan() {
			line := sc.Text()
			if p, ok := ParseProgress(line); ok {
				if onProgress != nil {
					onProgress(p)
				}
				continue
			}
			out.WriteString(line + "\n")
		}
	}()
	err := cmd.Run()
	pw.Close()
	<-done
	if err != nil {
		return out.String(), fmt.Errorf("chdman %s: %w\n%s", args[0], err, out.String())
	}
	return out.String(), nil
}

// Create converts input (a .cue/.gdi/.iso) into a CHD at output.
func (c *Chdman) Create(ctx context.Context, media Media, input, output string, codecs CodecProfile, onProgress func(float64)) error {
	_, err := c.run(ctx, c.CreateArgs(media, input, output, codecs), onProgress)
	return err
}

// Verify runs `chdman verify`, which checks the CHD's internal checksums.
func (c *Chdman) Verify(ctx context.Context, chd string) error {
	_, err := c.run(ctx, []string{"verify", "-i", chd}, nil)
	return err
}

var sha1Pattern = regexp.MustCompile(`(?m)^\s*(Raw SHA1|Data SHA1|SHA1):\s+([0-9a-fA-F]{40})\s*$`)

// ParseInfoSHA1s extracts the SHA1 lines from `chdman info` output, keyed
// by their label ("Raw SHA1" = uncompressed data, "SHA1" = data+metadata).
func ParseInfoSHA1s(info string) map[string]string {
	out := map[string]string{}
	for _, m := range sha1Pattern.FindAllStringSubmatch(info, -1) {
		out[m[1]] = strings.ToLower(m[2])
	}
	return out
}

// RawSHA1 returns the CHD's embedded raw-data SHA1 so the output can be
// proven against the DAT (spec §2.10).
func (c *Chdman) RawSHA1(ctx context.Context, chd string) (string, error) {
	info, err := c.run(ctx, []string{"info", "-i", chd, "-v"}, nil)
	if err != nil {
		return "", err
	}
	s := ParseInfoSHA1s(info)
	for _, k := range []string{"Data SHA1", "Raw SHA1"} {
		if v, ok := s[k]; ok {
			return v, nil
		}
	}
	return "", fmt.Errorf("no raw SHA1 in chdman info output for %s", chd)
}

// Info returns the verbose `chdman info` output for a CHD.
func (c *Chdman) Info(ctx context.Context, chd string) (string, error) {
	return c.run(ctx, []string{"info", "-i", chd, "-v"}, nil)
}

// ExtractCD extracts a CD CHD to a .cue plus split per-track .bin files
// (outBin must contain %t for the track number).
func (c *Chdman) ExtractCD(ctx context.Context, chd, outCue, outBin string) error {
	_, err := c.run(ctx, []string{"extractcd", "-i", chd, "-o", outCue, "-ob", outBin, "-sb", "--force"}, nil)
	return err
}

// ExtractDVD extracts a DVD/UMD CHD to a raw .iso.
func (c *Chdman) ExtractDVD(ctx context.Context, chd, outISO string) error {
	_, err := c.run(ctx, []string{"extractdvd", "-i", chd, "-o", outISO, "--force"}, nil)
	return err
}
