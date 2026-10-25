// Package nointro keeps the No-Intro DATs of cartridge systems up to date
// from the copies the libretro-database project publishes on GitHub
// (metadat/no-intro), downloaded on request. No-Intro's own site has no
// download the app can use, so without this every cartridge system needed a
// DAT downloaded by hand.
//
// libretro's copies are plain (no parent/clone data) ClrMamePro DATs that
// repeat a game once per file variant (a headered .nes and a headerless
// .unh, for instance); matching is per file, so the repeats are harmless.
// They trail No-Intro by days to weeks.
package nointro

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"

	"romcure/internal/dat"
	"romcure/internal/profile"
	"romcure/internal/store"
)

// IndexURL lists libretro-database's No-Intro DATs (GitHub's contents API).
const IndexURL = "https://api.github.com/repos/libretro/libretro-database/contents/metadat/no-intro"

// UserAgent identifies us to GitHub, which requires one.
const UserAgent = "romcure (+local ROM library manager)"

const (
	remoteSource  = "no-intro"
	maxIndexBytes = 4 << 20
	maxDatBytes   = 256 << 20 // the largest (Xbox 360) is ~5 MB
)

// File is one DAT in the index. SHA is its git blob hash, which changes
// exactly when the file does, so it serves as the version.
type File struct {
	Name        string `json:"name"`
	SHA         string `json:"sha"`
	Size        int64  `json:"size"`
	DownloadURL string `json:"download_url"`
}

// Index is the list of DATs, by file name.
type Index map[string]File

// FetchIndex downloads the list of DATs from indexURL ("" = IndexURL).
func FetchIndex(ctx context.Context, client *http.Client, indexURL string) (Index, error) {
	if indexURL == "" {
		indexURL = IndexURL
	}
	body, err := get(ctx, client, indexURL, maxIndexBytes)
	if err != nil {
		return nil, fmt.Errorf("list the No-Intro DATs: %w", err)
	}
	defer body.Close()
	var files []File
	if err := json.NewDecoder(body).Decode(&files); err != nil {
		return nil, fmt.Errorf("list the No-Intro DATs: %w", err)
	}
	idx := Index{}
	for _, f := range files {
		if f.DownloadURL != "" {
			idx[f.Name] = f
		}
	}
	return idx, nil
}

// Find returns the DAT of systemID: the file named after its libretro
// database name, which is also No-Intro's name for the system.
func (idx Index) Find(systemID string) (File, bool) {
	name, ok := profile.LibretroDBName(systemID)
	if !ok {
		return File{}, false
	}
	f, ok := idx[name+".dat"]
	return f, ok
}

// Result reports what SyncSystem did.
type Result struct {
	Updated bool
	Reason  string
	Games   int
}

// Syncer downloads DATs into Dir and imports them.
type Syncer struct {
	DB     *store.DB
	Client *http.Client
	Dir    string // app-data root: DATs go to Dir/dats/no-intro
}

// SyncSystem brings systemID's DAT up to date with f: nothing is downloaded
// when the imported one is the same version; otherwise the new one is
// downloaded, imported, and the one it replaces removed.
func (s *Syncer) SyncSystem(ctx context.Context, systemID string, f File) (Result, error) {
	have, err := storedVersion(s.DB, systemID)
	if err != nil {
		return Result{}, err
	}
	if have == f.SHA {
		return Result{Reason: "already up to date"}, nil
	}
	path, err := s.download(ctx, f)
	if err != nil {
		return Result{}, err
	}
	df, err := dat.ParseFile(path)
	if err != nil {
		return Result{}, fmt.Errorf("read %s: %w", f.Name, err)
	}
	if len(df.Games) == 0 {
		return Result{}, fmt.Errorf("%s lists no games", f.Name)
	}
	id, err := dat.Import(s.DB, df, systemID, path)
	if err != nil {
		return Result{}, err
	}
	if _, err := s.DB.Exec(`UPDATE dats SET remote_source = ?, remote_slug = ?, remote_version = ? WHERE id = ?`,
		remoteSource, systemID, f.SHA, id); err != nil {
		return Result{}, fmt.Errorf("record remote version: %w", err)
	}
	// Prune superseded imports only after the new one fully succeeded.
	if _, err := s.DB.Exec(`DELETE FROM dats WHERE remote_source = ? AND remote_slug = ? AND id <> ?`,
		remoteSource, systemID, id); err != nil {
		return Result{}, fmt.Errorf("prune old imports: %w", err)
	}
	res := Result{Updated: true, Games: len(df.Games), Reason: fmt.Sprintf("imported (%d games)", len(df.Games))}
	if have != "" {
		res.Reason = fmt.Sprintf("updated (%d games)", len(df.Games))
	}
	return res, nil
}

func storedVersion(db *store.DB, systemID string) (string, error) {
	var v sql.NullString
	err := db.QueryRow(`SELECT remote_version FROM dats WHERE remote_source = ? AND remote_slug = ?
		ORDER BY id DESC LIMIT 1`, remoteSource, systemID).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return v.String, err
}

// download saves f as Dir/dats/no-intro/<name>, replacing an older copy
// only once the new one is complete.
func (s *Syncer) download(ctx context.Context, f File) (string, error) {
	dir := filepath.Join(s.Dir, "dats", "no-intro")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	body, err := get(ctx, s.Client, f.DownloadURL, maxDatBytes)
	if err != nil {
		return "", fmt.Errorf("download %s: %w", f.Name, err)
	}
	defer body.Close()
	tmp, err := os.CreateTemp(dir, ".download-*.dat")
	if err != nil {
		return "", err
	}
	n, err := io.Copy(tmp, io.LimitReader(body, maxDatBytes+1))
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err == nil && n > maxDatBytes {
		err = errors.New("exceeds the size limit")
	}
	path := filepath.Join(dir, filepath.Base(f.Name))
	if err == nil {
		err = os.Rename(tmp.Name(), path)
	}
	if err != nil {
		os.Remove(tmp.Name())
		return "", fmt.Errorf("download %s: %w", f.Name, err)
	}
	return path, nil
}

func get(ctx context.Context, client *http.Client, u string, limit int64) (io.ReadCloser, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", UserAgent)
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("HTTP %s", resp.Status)
	}
	if resp.ContentLength > limit {
		resp.Body.Close()
		return nil, fmt.Errorf("%d bytes exceeds the size limit", resp.ContentLength)
	}
	return resp.Body, nil
}
