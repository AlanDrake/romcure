package tosec

import (
	"archive/zip"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"

	"romcure/internal/dat"
	"romcure/internal/store"
)

const (
	remoteSource = "tosec"
	maxPackBytes = 512 << 20
	maxDatBytes  = 1 << 30
)

// Syncer downloads TOSEC releases into Dir and imports their DATs.
type Syncer struct {
	DB     *store.DB
	Client *http.Client
	Dir    string // app-data root: packs in Dir/tosec, extracted DATs in Dir/dats/tosec/<system>
}

// Result reports what SyncSystem did.
type Result struct {
	SystemID string
	Release  string
	Updated  bool
	Reason   string
	Dats     int // DAT files imported
	Games    int
}

func versionKey(rel Release, categories []string) string {
	return rel.Date + "|" + strings.Join(categories, ",")
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

// ensurePack downloads rel's Complete pack once (reused across systems of
// the same release) and returns its path. Older packs are removed.
func (s *Syncer) ensurePack(ctx context.Context, rel Release) (string, error) {
	dir := filepath.Join(s.Dir, "tosec")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	pack := filepath.Join(dir, "pack-"+rel.Date+".zip")
	if st, err := os.Stat(pack); err == nil && st.Size() > 0 {
		return pack, nil
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rel.PackURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", UserAgent)
	resp, err := s.Client.Do(req)
	if err != nil {
		return "", fmt.Errorf("download %s: %w", rel.PackURL, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download %s: HTTP %s", rel.PackURL, resp.Status)
	}
	if resp.ContentLength > maxPackBytes {
		return "", fmt.Errorf("download %s: %d bytes exceeds limit", rel.PackURL, resp.ContentLength)
	}
	tmp, err := os.CreateTemp(dir, ".pack-*.zip")
	if err != nil {
		return "", err
	}
	n, err := io.Copy(tmp, io.LimitReader(resp.Body, maxPackBytes+1))
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err == nil && n > maxPackBytes {
		err = errors.New("pack exceeds size limit")
	}
	if err != nil {
		os.Remove(tmp.Name())
		return "", fmt.Errorf("download %s: %w", rel.PackURL, err)
	}
	if err := os.Rename(tmp.Name(), pack); err != nil {
		os.Remove(tmp.Name())
		return "", err
	}
	if old, _ := filepath.Glob(filepath.Join(dir, "pack-*.zip")); len(old) > 1 {
		for _, o := range old {
			if o != pack {
				os.Remove(o)
			}
		}
	}
	return pack, nil
}

// SyncSystem imports every DAT in rel that belongs to systemID (restricted
// to categories when non-empty, e.g. {"Games"}), skipping the download and
// import when that release is already imported for the same selection. The
// previous import is pruned only after the new one fully succeeded.
func (s *Syncer) SyncSystem(ctx context.Context, rel Release, systemID string, categories []string) (Result, error) {
	res := Result{SystemID: systemID, Release: rel.Date}
	want := versionKey(rel, categories)
	have, err := storedVersion(s.DB, systemID)
	if err != nil {
		return res, err
	}
	if have == want {
		res.Reason = "already up to date (" + rel.Date + ")"
		return res, nil
	}

	pack, err := s.ensurePack(ctx, rel)
	if err != nil {
		return res, err
	}
	zr, err := zip.OpenReader(pack)
	if err != nil {
		os.Remove(pack) // a corrupt cached pack must not stick
		return res, fmt.Errorf("open pack: %w", err)
	}
	defer zr.Close()

	outDir := filepath.Join(s.Dir, "dats", "tosec", systemID)
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return res, err
	}
	var newIDs []int64
	for _, f := range zr.File {
		if f.FileInfo().IsDir() || !strings.EqualFold(path.Ext(f.Name), ".dat") {
			continue
		}
		name := strings.ReplaceAll(f.Name, "\\", "/")
		// Only the main and ISO trees hold game DATs; PIX is scans.
		if !strings.HasPrefix(name, "TOSEC/") && !strings.HasPrefix(name, "ISO/") {
			continue
		}
		dn, ok := ParseDatName(name)
		if !ok {
			continue
		}
		if id, ok := SystemID(dn.Product); !ok || id != systemID {
			continue
		}
		if len(categories) > 0 && !contains(categories, dn.Category) {
			continue
		}
		dest := filepath.Join(outDir, strings.Split(name, "/")[0]+" - "+path.Base(name))
		if err := extract(f, dest); err != nil {
			return res, err
		}
		df, err := dat.ParseFile(dest)
		if err != nil {
			return res, fmt.Errorf("parse %s: %w", dest, err)
		}
		id, err := dat.Import(s.DB, df, systemID, dest)
		if err != nil {
			return res, err
		}
		if _, err := s.DB.Exec(`UPDATE dats SET remote_source = ?, remote_slug = ?, remote_version = ? WHERE id = ?`,
			remoteSource, systemID, want, id); err != nil {
			return res, err
		}
		newIDs = append(newIDs, id)
		res.Dats++
		res.Games += len(df.Games)
	}
	if res.Dats == 0 {
		res.Reason = "release has no DATs for this system/category selection"
		return res, nil
	}
	if err := pruneOthers(s.DB, systemID, newIDs); err != nil {
		return res, err
	}
	res.Updated = true
	res.Reason = fmt.Sprintf("imported %d DAT(s) from release %s", res.Dats, rel.Date)
	return res, nil
}

func pruneOthers(db *store.DB, systemID string, keep []int64) error {
	rows, err := db.Query(`SELECT id FROM dats WHERE remote_source = ? AND remote_slug = ?`, remoteSource, systemID)
	if err != nil {
		return err
	}
	keepSet := map[int64]bool{}
	for _, k := range keep {
		keepSet[k] = true
	}
	var doomed []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		if !keepSet[id] {
			doomed = append(doomed, id)
		}
	}
	rows.Close()
	for _, id := range doomed {
		if _, err := db.Exec(`DELETE FROM dats WHERE id = ?`, id); err != nil {
			return err
		}
	}
	return nil
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if strings.EqualFold(x, s) {
			return true
		}
	}
	return false
}

func extract(f *zip.File, dest string) error {
	rc, err := f.Open()
	if err != nil {
		return err
	}
	defer rc.Close()
	tmp := dest + ".tmp"
	out, err := os.Create(tmp)
	if err != nil {
		return err
	}
	n, err := io.Copy(out, io.LimitReader(rc, maxDatBytes+1))
	if cerr := out.Close(); err == nil {
		err = cerr
	}
	if err == nil && n > maxDatBytes {
		err = errors.New("dat exceeds size limit")
	}
	if err != nil {
		os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, dest)
}
