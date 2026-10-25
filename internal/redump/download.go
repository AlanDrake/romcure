package redump

import (
	"archive/zip"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

// Limits protect against a misbehaving server or a zip bomb. They sit well
// above the real largest files (PS2 DAT is tens of MB).
const (
	maxZipBytes     = 256 << 20 // compressed download
	maxDatBytes     = 1 << 30   // one decompressed .dat
	maxSBIFileBytes = 1 << 20   // one .sbi is ~450 bytes
	maxSBIFiles     = 20000
)

// Downloader fetches Redump archives into Dir. It makes one request at a
// time and only on explicit calls; callers decide when to hit the network.
type Downloader struct {
	Client  *http.Client
	BaseURL string // BaseURL (redump.info) when empty
	Dir     string // app-data root: dats go to Dir/dats/redump/<slug>, sbis to Dir/sbi/<slug>
}

func (d *Downloader) base() string {
	if d.BaseURL != "" {
		return d.BaseURL
	}
	return BaseURL
}

// downloadZip fetches kind/slug into a temp file inside dir and returns its
// path. The caller removes it.
func (d *Downloader) downloadZip(ctx context.Context, kind, slug, dir string) (string, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	u := d.base() + "/" + kind + "/" + url.PathEscape(slug)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", UserAgent)
	resp, err := d.Client.Do(req)
	if err != nil {
		return "", fmt.Errorf("download %s: %w", u, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download %s: HTTP %s", u, resp.Status)
	}
	if resp.ContentLength > maxZipBytes {
		return "", fmt.Errorf("download %s: %d bytes exceeds limit", u, resp.ContentLength)
	}
	tmp, err := os.CreateTemp(dir, ".download-*.zip")
	if err != nil {
		return "", err
	}
	n, err := io.Copy(tmp, io.LimitReader(resp.Body, maxZipBytes+1))
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err == nil && n > maxZipBytes {
		err = errors.New("download exceeds size limit")
	}
	if err != nil {
		os.Remove(tmp.Name())
		return "", fmt.Errorf("download %s: %w", u, err)
	}
	return tmp.Name(), nil
}

// writeAtomic copies r (at most limit bytes) to dest via a temp file.
func writeAtomic(dest string, r io.Reader, limit int64) error {
	tmp := dest + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	n, err := io.Copy(f, io.LimitReader(r, limit+1))
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err == nil && n > limit {
		err = errors.New("file exceeds size limit")
	}
	if err != nil {
		os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, dest)
}

// safeBase reduces a zip entry name to a plain file name, rejecting
// anything that tries to escape the destination (zip-slip).
func safeBase(name string) (string, bool) {
	name = strings.ReplaceAll(name, "\\", "/")
	base := filepath.Base(filepath.FromSlash(name))
	if base == "." || base == ".." || base == string(filepath.Separator) || strings.ContainsAny(base, `:<>"|?*`) {
		return "", false
	}
	return base, true
}

// FetchDat downloads the datfile zip for slug and extracts its single .dat
// into Dir/dats/redump/<slug>/, returning the path. Older DATs for the slug
// stay on disk until the caller prunes them.
func (d *Downloader) FetchDat(ctx context.Context, slug string) (string, error) {
	outDir := filepath.Join(d.Dir, "dats", "redump", slug)
	zpath, err := d.downloadZip(ctx, "datfile", slug, outDir)
	if err != nil {
		return "", err
	}
	defer os.Remove(zpath)

	zr, err := zip.OpenReader(zpath)
	if err != nil {
		return "", fmt.Errorf("open datfile zip for %s: %w", slug, err)
	}
	defer zr.Close()
	for _, f := range zr.File {
		base, ok := safeBase(f.Name)
		if !ok || f.FileInfo().IsDir() || !strings.EqualFold(filepath.Ext(base), ".dat") {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return "", err
		}
		dest := filepath.Join(outDir, base)
		err = writeAtomic(dest, rc, maxDatBytes)
		rc.Close()
		if err != nil {
			return "", fmt.Errorf("extract %s: %w", base, err)
		}
		return dest, nil
	}
	return "", fmt.Errorf("datfile zip for %s contains no .dat", slug)
}

// FetchSBI downloads the SBI archive for slug and extracts every .sbi into
// Dir/sbi/<slug>/ (flat, original names, which equal the DAT game names).
// It returns how many files were written.
func (d *Downloader) FetchSBI(ctx context.Context, slug string) (int, error) {
	outDir := filepath.Join(d.Dir, "sbi", slug)
	zpath, err := d.downloadZip(ctx, "sbi", slug, outDir)
	if err != nil {
		return 0, err
	}
	defer os.Remove(zpath)

	zr, err := zip.OpenReader(zpath)
	if err != nil {
		return 0, fmt.Errorf("open sbi zip for %s: %w", slug, err)
	}
	defer zr.Close()
	count := 0
	for _, f := range zr.File {
		base, ok := safeBase(f.Name)
		if !ok || f.FileInfo().IsDir() || !strings.EqualFold(filepath.Ext(base), ".sbi") {
			continue
		}
		if count >= maxSBIFiles {
			return count, errors.New("sbi archive has too many files")
		}
		rc, err := f.Open()
		if err != nil {
			return count, err
		}
		err = writeAtomic(filepath.Join(outDir, base), rc, maxSBIFileBytes)
		rc.Close()
		if err != nil {
			return count, fmt.Errorf("extract %s: %w", base, err)
		}
		count++
	}
	return count, nil
}
