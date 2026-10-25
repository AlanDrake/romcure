package redump

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"romcure/internal/dat"
	"romcure/internal/store"
)

const remoteSource = "redump"

// SyncResult reports what SyncSystem did, so the UI can say why nothing
// happened instead of failing silently.
type SyncResult struct {
	Slug     string
	SystemID string
	Version  string
	Updated  bool   // a new DAT was downloaded and imported
	Reason   string // human-readable outcome
	SBIFiles int
	DatPath  string
	DatGames int
}

// storedVersion returns the remote version of the currently imported
// Redump DAT for slug, or "" if none.
func storedVersion(db *store.DB, slug string) (string, error) {
	var v sql.NullString
	err := db.QueryRow(`SELECT remote_version FROM dats WHERE remote_source = ? AND remote_slug = ?
		ORDER BY id DESC LIMIT 1`, remoteSource, slug).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return v.String, err
}

// SyncSystem brings the local Redump DAT for one system up to date: it
// skips the download when the catalog version matches what's imported,
// otherwise downloads, imports, and prunes the superseded import. When the
// catalog row advertises SBI files and withSBI is set, it fetches those too
// (only on an update, so an unchanged system costs no traffic).
func SyncSystem(ctx context.Context, db *store.DB, dl *Downloader, sys System, withSBI bool) (SyncResult, error) {
	res := SyncResult{Slug: sys.Slug, Version: sys.Version}
	systemID, ok := SystemID(sys.Slug)
	if !ok {
		res.Reason = "no app system is mapped to this Redump slug"
		return res, nil
	}
	res.SystemID = systemID

	have, err := storedVersion(db, sys.Slug)
	if err != nil {
		return res, fmt.Errorf("check stored version for %s: %w", sys.Slug, err)
	}
	if have != "" && have == sys.Version {
		res.Reason = "already up to date (" + have + ")"
		return res, nil
	}

	path, err := dl.FetchDat(ctx, sys.Slug)
	if err != nil {
		return res, err
	}
	df, err := dat.ParseFile(path)
	if err != nil {
		return res, fmt.Errorf("parse %s: %w", path, err)
	}
	newID, err := dat.Import(db, df, systemID, path)
	if err != nil {
		return res, err
	}
	if _, err := db.Exec(`UPDATE dats SET remote_source = ?, remote_slug = ?, remote_version = ? WHERE id = ?`,
		remoteSource, sys.Slug, sys.Version, newID); err != nil {
		return res, fmt.Errorf("record remote version: %w", err)
	}
	// Prune superseded imports only after the new one fully succeeded.
	if _, err := db.Exec(`DELETE FROM dats WHERE remote_source = ? AND remote_slug = ? AND id <> ?`,
		remoteSource, sys.Slug, newID); err != nil {
		return res, fmt.Errorf("prune old imports: %w", err)
	}
	res.Updated, res.DatPath, res.DatGames = true, path, len(df.Games)
	res.Reason = "imported " + sys.Version
	if have != "" {
		res.Reason = "updated " + have + " to " + sys.Version
	}

	if withSBI && sys.HasSBI {
		n, err := dl.FetchSBI(ctx, sys.Slug)
		res.SBIFiles = n
		if err != nil {
			return res, fmt.Errorf("dat imported, but SBI download failed: %w", err)
		}
	}
	return res, nil
}
