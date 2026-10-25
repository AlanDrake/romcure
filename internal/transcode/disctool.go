package transcode

import "context"

// DiscTool is a converter that turns a disc image into a compressed
// format in the transcode cache: chdman (CHD) or dolphin-tool (RVZ). Each
// is only ever run as a subprocess (HANDOFF §3).
type DiscTool interface {
	Name() string
	ToolVersion() string
	OutputFormat() Format
	// Convert writes the converted image to output. j carries the media
	// kind and codec profile.
	Convert(ctx context.Context, j Job, input, output string, onProgress func(float64)) error
	// Verify checks the output's internal integrity.
	Verify(ctx context.Context, output string) error
	// RawSHA1 returns the SHA1 of the uncompressed data embedded in or
	// computed from output, for comparison with the DAT.
	RawSHA1(ctx context.Context, output string) (string, error)
}

func (c *Chdman) Name() string         { return "chdman" }
func (c *Chdman) ToolVersion() string  { return c.Version }
func (c *Chdman) OutputFormat() Format { return FormatCHD }

func (c *Chdman) Convert(ctx context.Context, j Job, input, output string, onProgress func(float64)) error {
	return c.Create(ctx, j.Media, input, output, j.Codecs, onProgress)
}
